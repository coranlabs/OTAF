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

type JobStatus struct {
	State     string   `json:"info_job_status"`
	Producers []string `json:"producers"`
}

type consumerJob struct {
	InfoTypeID string `json:"info_type_id"`
	JobOwner   string `json:"job_owner"`
	ResultURI  string `json:"job_result_uri"`
	Definition any    `json:"job_definition"`
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
