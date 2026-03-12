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

// NewConsumer returns nil when no endpoint is configured, so an rApp that only
// produces can hold a nil *Consumer safely.
func NewConsumer(cfg ConsumerConfig, logger *logrus.Logger) (*Consumer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.Enabled() {
		return nil, nil
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &Consumer{
		cfg:     cfg,
		http:    &http.Client{Timeout: cfg.Timeout},
		logger:  logger,
		placed:  map[string]bool{},
		lastErr: map[string]string{},
	}, nil
}

func (c *Consumer) Name() string { return "r1-consumer:" + c.cfg.Owner }

// Want declares a subscription. Nothing is sent until Start runs, and the
// subscription is retried until the platform accepts it, because a consumer
// commonly starts before the producer it depends on exists.
func (c *Consumer) Want(subs ...Subscription) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range subs {
		if err := s.validate(); err != nil {
			return err
		}
		c.wanted = append(c.wanted, s)
	}
	return nil
}

func (c *Consumer) Ping(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, consumerBase+"/info-types", nil, "list info types")
	return err
}

func (c *Consumer) InfoTypes(ctx context.Context) ([]string, error) {
	body, err := c.do(ctx, http.MethodGet, consumerBase+"/info-types", nil, "list info types")
	if err != nil {
		return nil, err
	}
	var ids []string
	if err := json.Unmarshal(body, &ids); err != nil {
		return nil, fmt.Errorf("r1 list info types: %w", err)
	}
	return ids, nil
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

func (c *Consumer) Jobs(ctx context.Context) ([]string, error) {
	body, err := c.do(ctx, http.MethodGet,
		consumerBase+"/info-jobs?owner="+url.QueryEscape(c.cfg.Owner), nil, "list info jobs")
	if err != nil {
		return nil, err
	}
	var ids []string
	if err := json.Unmarshal(body, &ids); err != nil {
		return nil, fmt.Errorf("r1 list info jobs: %w", err)
	}
	return ids, nil
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

// Subscribe places one job immediately. Start does this for every declared
// subscription, so call it directly only for jobs decided at runtime.
func (c *Consumer) Subscribe(ctx context.Context, s Subscription) error {
	if err := s.validate(); err != nil {
		return err
	}

	definition := s.Definition
	if definition == nil {
		definition = map[string]any{}
	}
	body, err := json.Marshal(consumerJob{
		InfoTypeID: s.InfoTypeID,
		JobOwner:   c.cfg.Owner,
		ResultURI:  c.resultURI(s.DeliverTo),
		Definition: definition,
	})
	if err != nil {
		return fmt.Errorf("r1 subscribe: %w", err)
	}

	_, err = c.do(ctx, http.MethodPut,
		consumerBase+"/info-jobs/"+url.PathEscape(s.JobID), body, "create info job")
	return err
}

func (c *Consumer) Unsubscribe(ctx context.Context, jobID string) error {
	_, err := c.do(ctx, http.MethodDelete,
		consumerBase+"/info-jobs/"+url.PathEscape(jobID), nil, "delete info job")
	if err != nil && !IsNotFound(err) {
		return err
	}
	c.mu.Lock()
	delete(c.placed, jobID)
	c.mu.Unlock()
	return nil
}

func (c *Consumer) resultURI(path string) string {
	return strings.TrimRight(c.cfg.SelfURL, "/") + "/" + strings.TrimPrefix(path, "/")
}

// Pending lists subscriptions the platform has not accepted yet, with the
// reason, so an rApp can surface why data is not arriving.
func (c *Consumer) Pending() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]string{}
	for _, s := range c.wanted {
		if !c.placed[s.JobID] {
			out[s.JobID] = c.lastErr[s.JobID]
		}
	}
	return out
}

// Start places every declared subscription and keeps trying for the ones the
// platform has not accepted, until ctx ends.
func (c *Consumer) Start(ctx context.Context) error {
	if c == nil {
		return nil
	}

	c.reconcile(ctx)

	ticker := time.NewTicker(reconcileEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.reconcile(ctx)
		}
	}
}

func (c *Consumer) reconcile(ctx context.Context) {
	c.mu.Lock()
	pending := make([]Subscription, 0, len(c.wanted))
	for _, s := range c.wanted {
		if !c.placed[s.JobID] {
			pending = append(pending, s)
		}
	}
	c.mu.Unlock()

	for _, s := range pending {
		err := c.Subscribe(ctx, s)

		c.mu.Lock()
		if err == nil {
			c.placed[s.JobID] = true
			delete(c.lastErr, s.JobID)
		} else {
			c.lastErr[s.JobID] = err.Error()
		}
		c.mu.Unlock()

		entry := c.logger.WithFields(logrus.Fields{"job": s.JobID, "type": s.InfoTypeID})
		if err != nil {
			// A type that does not exist yet is the normal case when this
			// rApp starts before the one producing the data.
			entry.WithError(err).Info("information job not accepted yet, will retry")
			continue
		}
		entry.WithField("target", c.resultURI(s.DeliverTo)).Info("information job created")
	}
}

func (c *Consumer) do(ctx context.Context, method, path string, body []byte, op string) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method,
		strings.TrimRight(c.cfg.Endpoint, "/")+path, reader)
	if err != nil {
		return nil, fmt.Errorf("r1 %s: %w", op, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", rappsdk.UserAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("r1 %s: %w", op, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("r1 %s: %w", op, err)
	}
	if resp.StatusCode >= 300 {
		return nil, &ConsumerError{Op: op, Status: resp.StatusCode, Detail: problemDetail(payload)}
	}
	return payload, nil
}

type ConsumerError struct {
	Op     string
	Status int
	Detail string
}

func (e *ConsumerError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("r1 %s: status %d", e.Op, e.Status)
	}
	return fmt.Sprintf("r1 %s: status %d: %s", e.Op, e.Status, e.Detail)
}

// Retryable reports whether trying the same call again could succeed. An
// information type that does not exist yet is deliberately not retryable here:
// the reconcile loop handles that on a much longer cycle.
func (e *ConsumerError) Retryable() bool {
	switch e.Status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	}
	return e.Status >= 500
}

// The three methods below let errs classify this failure without either
// package importing the other.

func (e *ConsumerError) ErrorCategory() string { return "platform" }
