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

// Package r1 makes an rApp a data producer on the R1 data management and
// exposure interface: it answers supervision, accepts information jobs from
// the information coordinator, and delivers to each job's target.
package r1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	rappsdk "github.com/coranlabs/OTAF/otaf-rapp-sdk"
)

const (
	// Paths the producer registration in the rApp package points at.
	HealthPath       = "/r1/producer-health"
	SubscriptionPath = "/r1/callback/subscription"

	defaultInterval = 30 * time.Second
	minInterval     = 5 * time.Second
	tick            = time.Second
)

type Job struct {
	ID        string          `json:"info_job_identity"`
	TypeID    string          `json:"info_type_identity"`
	TargetURI string          `json:"target_uri"`
	Owner     string          `json:"owner"`
	Data      json.RawMessage `json:"info_job_data,omitempty"`

	interval time.Duration
	nextDue  time.Time
}

// Interval is how often this job wants delivery, taken from the job data when
// the consumer asked for a specific cadence.
func (j Job) Interval() time.Duration { return j.interval }

// Snapshot produces the payload delivered to one job. Returning a nil payload
// skips that delivery without logging an error, which is how a producer says
// "nothing matched this job's filter right now".
type Snapshot func(ctx context.Context, job Job) ([]byte, error)

type Producer struct {
	id       string
	snapshot Snapshot
	logger   *logrus.Logger
	client   *http.Client
	interval time.Duration

	mu   sync.RWMutex
	jobs map[string]*Job
}

type Option func(*Producer)

// WithInterval sets the delivery cadence for jobs that do not request one.
func WithInterval(d time.Duration) Option {
	return func(p *Producer) {
		if d >= minInterval {
			p.interval = d
		}
	}
}

func WithHTTPClient(c *http.Client) Option {
	return func(p *Producer) {
		if c != nil {
			p.client = c
		}
	}
}

func NewProducer(id string, snapshot Snapshot, logger *logrus.Logger, opts ...Option) *Producer {
	p := &Producer{
		id:       id,
		snapshot: snapshot,
		logger:   logger,
		client:   &http.Client{Timeout: 15 * time.Second},
		interval: defaultInterval,
		jobs:     map[string]*Job{},
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

func (p *Producer) Name() string { return "r1-producer:" + p.id }
