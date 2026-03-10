// Copyright 2025-2026 coRAN LABS Private Limited
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package r1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	rappsdk "github.com/coranlabs/OTAF/otaf-rapp-sdk"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
)

const (
	consumerBase   = "/data-consumer/v1"
	reconcileEvery = 30 * time.Second

	// What the coordinator reports once at least one producer stands behind a
	// type or a job.
	statusEnabled = "ENABLED"
)

type ConsumerConfig struct {
	// Endpoint is the information coordinator.
	Endpoint string `yaml:"endpoint" env:"ICS_ADDR"`

	// Owner identifies this rApp's jobs. Reuse it across restarts so the rApp
	// takes ownership of jobs it created before rather than duplicating them.
	Owner string `yaml:"owner" env:"ICS_JOB_OWNER"`

	// SelfURL is where producers can reach this rApp, normally the rApp's own
	// service address. Delivery targets are built from it.
	SelfURL string `yaml:"self_url" env:"ICS_SELF_URL"`

	Timeout time.Duration `yaml:"timeout" env:"ICS_TIMEOUT"`
}

func (c ConsumerConfig) Enabled() bool { return strings.TrimSpace(c.Endpoint) != "" }

func (c ConsumerConfig) Validate() error {
	if !c.Enabled() {
		return nil
	}
	if strings.TrimSpace(c.Owner) == "" {
		return errs.New(errs.CategoryConfig, "R1_NO_OWNER",
			"r1: consumer owner is required; it is how the platform attributes jobs to this rApp")
	}
	if strings.TrimSpace(c.SelfURL) == "" {
		return errs.New(errs.CategoryConfig, "R1_NO_SELF_URL",
			"r1: consumer self_url is required; a producer has to be told where to deliver")
	}
	if _, err := url.Parse(c.SelfURL); err != nil {
		return errs.Wrap(err, errs.CategoryConfig, "R1_BAD_SELF_URL",
			"r1: consumer self_url is not a URL").WithField("self_url", c.SelfURL)
	}
	return nil
}

// Subscription is a standing request for data of one information type.
type Subscription struct {
	// JobID must be stable across restarts so the job is replaced rather than
	// duplicated.
	JobID string

	InfoTypeID string

	// Definition is type-specific and validated by the platform against the
	// information type's schema.
	Definition any

	// DeliverTo is the path on this rApp where data should arrive, matching
	// the path an ingest source listens on.
	DeliverTo string
}

// A subscription missing a field is the caller's mistake, not the platform's.
func (s Subscription) validate() error {
	switch {
	case s.JobID == "":
		return errs.New(errs.CategoryInternal, "R1_INVALID_SUBSCRIPTION",
			"r1: subscription job id is required")
	case s.InfoTypeID == "":
		return errs.New(errs.CategoryInternal, "R1_INVALID_SUBSCRIPTION",
			"r1: subscription info type id is required")
	case s.DeliverTo == "":
		return errs.New(errs.CategoryInternal, "R1_INVALID_SUBSCRIPTION",
			"r1: subscription needs a delivery path")
	}
	return nil
}

// InfoType is the consumer's view of an information type: the schema a job
// definition must satisfy, plus whether anyone is currently producing it.
// Note the field name differs from the producer's view of the same type.
type InfoType struct {
	ID        string          `json:"-"`
	Schema    json.RawMessage `json:"job_data_schema"`
	Status    string          `json:"type_status"`
	Producers int             `json:"no_of_producers"`
}

// Available reports whether subscribing to this type would actually yield
// data, rather than a job that sits idle waiting for a producer.
func (t InfoType) Available() bool { return t.Status == statusEnabled && t.Producers > 0 }

type JobStatus struct {
	State     string   `json:"info_job_status"`
	Producers []string `json:"producers"`
}

// Delivering reports whether at least one producer is serving this job.
func (s JobStatus) Delivering() bool { return s.State == statusEnabled && len(s.Producers) > 0 }

type consumerJob struct {
	InfoTypeID string `json:"info_type_id"`
	JobOwner   string `json:"job_owner"`
	ResultURI  string `json:"job_result_uri"`
	Definition any    `json:"job_definition"`
}

// Consumer subscribes this rApp to information types other producers publish.
type Consumer struct {
	cfg    ConsumerConfig
	http   *http.Client
	logger *logrus.Logger

	mu      sync.Mutex
	wanted  []Subscription
	placed  map[string]bool
	lastErr map[string]string
}

func (c *Consumer) InfoType(ctx context.Context, id string) (*InfoType, error) {
	body, err := c.do(ctx, http.MethodGet,
		consumerBase+"/info-types/"+url.PathEscape(id), nil, "get info type")
	if err != nil {
		return nil, err
	}
	t := &InfoType{ID: id}
	if err := json.Unmarshal(body, t); err != nil {
		return nil, fmt.Errorf("r1 get info type: %w", err)
	}
	return t, nil
}

func (c *Consumer) JobStatus(ctx context.Context, jobID string) (*JobStatus, error) {
	body, err := c.do(ctx, http.MethodGet,
		consumerBase+"/info-jobs/"+url.PathEscape(jobID)+"/status", nil, "get info job status")
	if err != nil {
		return nil, err
	}
	var s JobStatus
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("r1 get info job status: %w", err)
	}
	return &s, nil
}

type ConsumerError struct {
	Op     string
	Status int
	Detail string
}
