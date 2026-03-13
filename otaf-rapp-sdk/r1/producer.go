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

func (p *Producer) Register(r *mux.Router) {
	r.HandleFunc(HealthPath, p.handleHealth).Methods(http.MethodGet)
	r.HandleFunc(SubscriptionPath, p.handleJobStart).Methods(http.MethodPost, http.MethodPut)
	r.HandleFunc(SubscriptionPath+"/{jobId}", p.handleJobStart).Methods(http.MethodPost, http.MethodPut)
	r.HandleFunc(SubscriptionPath+"/{jobId}", p.handleJobStop).Methods(http.MethodDelete)
}

func (p *Producer) Open() []string { return []string{HealthPath, SubscriptionPath} }

func (p *Producer) Jobs() []Job {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]Job, 0, len(p.jobs))
	for _, j := range p.jobs {
		out = append(out, *j)
	}
	return out
}

func (p *Producer) handleHealth(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	active := len(p.jobs)
	p.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      "healthy",
		"producer_id": p.id,
		"jobs":        active,
	})
}

func (p *Producer) handleJobStart(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "unreadable body", http.StatusBadRequest)
		return
	}

	var job Job
	if err := json.Unmarshal(body, &job); err != nil {
		p.logger.WithError(err).Warn("information job callback carried invalid JSON")
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if id := mux.Vars(r)["jobId"]; id != "" {
		job.ID = id
	}
	if job.ID == "" {
		http.Error(w, "missing info_job_identity", http.StatusBadRequest)
		return
	}
	if job.TargetURI == "" {
		http.Error(w, "missing target_uri", http.StatusBadRequest)
		return
	}

	job.interval = p.intervalFor(job.Data)
	job.nextDue = time.Now()

	p.mu.Lock()
	p.jobs[job.ID] = &job
	active := len(p.jobs)
	p.mu.Unlock()

	p.logger.WithFields(logrus.Fields{
		"job":      job.ID,
		"type":     job.TypeID,
		"target":   job.TargetURI,
		"owner":    job.Owner,
		"interval": job.interval,
		"active":   active,
	}).Info("information job started")
	w.WriteHeader(http.StatusOK)
}

func (p *Producer) handleJobStop(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["jobId"]

	p.mu.Lock()
	_, known := p.jobs[id]
	delete(p.jobs, id)
	active := len(p.jobs)
	p.mu.Unlock()

	p.logger.WithFields(logrus.Fields{"job": id, "known": known, "active": active}).Info("information job stopped")
	w.WriteHeader(http.StatusNoContent)
}
