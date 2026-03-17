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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeICS struct {
	mu       sync.Mutex
	jobs     map[string]consumerJob
	types    map[string]bool
	producer int
}

func newFakeICS(types ...string) *fakeICS {
	f := &fakeICS{jobs: map[string]consumerJob{}, types: map[string]bool{}}
	for _, t := range types {
		f.types[t] = true
	}
	return f
}

func (f *fakeICS) addType(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.types[id] = true
}

func (f *fakeICS) jobCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.jobs)
}

func (f *fakeICS) job(id string) (consumerJob, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	return j, ok
}

func (f *fakeICS) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/data-consumer/v1/info-types", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		ids := make([]string, 0, len(f.types))
		for id := range f.types {
			ids = append(ids, id)
		}
		f.mu.Unlock()
		body, _ := json.Marshal(ids)
		_, _ = w.Write(body)
	})

	mux.HandleFunc("/data-consumer/v1/info-types/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/data-consumer/v1/info-types/")
		f.mu.Lock()
		known, producers := f.types[id], f.producer
		f.mu.Unlock()

		if !known {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"Information type not found: ` + id + `"}`))
			return
		}
		status := "DISABLED"
		if producers > 0 {
			status = "ENABLED"
		}
		body, _ := json.Marshal(map[string]any{
			"job_data_schema": map[string]any{"type": "object"},
			"type_status":     status,
			"no_of_producers": producers,
		})
		_, _ = w.Write(body)
	})

	mux.HandleFunc("/data-consumer/v1/info-jobs", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		ids := make([]string, 0, len(f.jobs))
		for id := range f.jobs {
			ids = append(ids, id)
		}
		f.mu.Unlock()
		body, _ := json.Marshal(ids)
		_, _ = w.Write(body)
	})

	mux.HandleFunc("/data-consumer/v1/info-jobs/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/data-consumer/v1/info-jobs/")

		if strings.HasSuffix(rest, "/status") {
			id := strings.TrimSuffix(rest, "/status")
			f.mu.Lock()
			_, known := f.jobs[id]
			producers := f.producer
			f.mu.Unlock()
			if !known {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			state, list := "DISABLED", []string{}
			if producers > 0 {
				state, list = "ENABLED", []string{"producer-1"}
			}
			body, _ := json.Marshal(map[string]any{"info_job_status": state, "producers": list})
			_, _ = w.Write(body)
			return
		}

		switch r.Method {
		case http.MethodPut:
			var job consumerJob
			if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.mu.Lock()
			known := f.types[job.InfoTypeID]
			if known {
				f.jobs[rest] = job
			}
			f.mu.Unlock()
			if !known {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"detail":"Information type not found: ` + job.InfoTypeID + `"}`))
				return
			}
			w.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			f.mu.Lock()
			_, known := f.jobs[rest]
			delete(f.jobs, rest)
			f.mu.Unlock()
			if !known {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestConsumer(t *testing.T, endpoint string) *Consumer {
	t.Helper()
	c, err := NewConsumer(ConsumerConfig{
		Endpoint: endpoint,
		Owner:    "test-rapp",
		SelfURL:  "http://test-rapp.nonrtric:8080",
	}, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestConsumerDisabledWithoutEndpoint(t *testing.T) {
	c, err := NewConsumer(ConsumerConfig{}, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if c != nil {
		t.Fatal("an unconfigured endpoint should yield a nil consumer")
	}
}

func TestConsumerRequiresOwnerAndSelfURL(t *testing.T) {
	if _, err := NewConsumer(ConsumerConfig{Endpoint: "http://ics", SelfURL: "http://me"}, quietLogger()); err == nil {
		t.Error("expected an error when no owner is configured")
	}
	if _, err := NewConsumer(ConsumerConfig{Endpoint: "http://ics", Owner: "me"}, quietLogger()); err == nil {
		t.Error("expected an error when no self URL is configured: a producer must be told where to deliver")
	}
}

func TestSubscribeBuildsTheDeliveryTarget(t *testing.T) {
	fake := newFakeICS("cell-state")
	c := newTestConsumer(t, fake.server(t).URL)

	err := c.Subscribe(context.Background(), Subscription{
		JobID: "job-1", InfoTypeID: "cell-state", DeliverTo: "/data",
	})
	if err != nil {
		t.Fatal(err)
	}

	job, ok := fake.job("job-1")
	if !ok {
		t.Fatal("job was not created")
	}
	if job.ResultURI != "http://test-rapp.nonrtric:8080/data" {
		t.Errorf("result uri = %q, want the rApp's own address plus the delivery path", job.ResultURI)
	}
	if job.JobOwner != "test-rapp" {
		t.Errorf("owner = %q, want test-rapp", job.JobOwner)
	}
}

func TestSubscribeValidatesInput(t *testing.T) {
	c := newTestConsumer(t, newFakeICS().server(t).URL)
	ctx := context.Background()

	cases := map[string]Subscription{
		"no job id":        {InfoTypeID: "t", DeliverTo: "/data"},
		"no type":          {JobID: "j", DeliverTo: "/data"},
		"no delivery path": {JobID: "j", InfoTypeID: "t"},
	}
	for name, s := range cases {
		if err := c.Subscribe(ctx, s); err == nil {
			t.Errorf("%s should be rejected before anything is sent", name)
		}
	}
}

// A consumer commonly starts before the rApp producing what it wants, so an
// unknown type must be a retryable condition rather than a fatal one.
func TestUnknownTypeIsNotFound(t *testing.T) {
	c := newTestConsumer(t, newFakeICS().server(t).URL)

	err := c.Subscribe(context.Background(), Subscription{
		JobID: "job-1", InfoTypeID: "absent", DeliverTo: "/data",
	})
	if !IsNotFound(err) {
		t.Errorf("expected a not-found classification, got %v", err)
	}
}

func TestReconcileRetriesUntilTheTypeAppears(t *testing.T) {
	fake := newFakeICS()
	c := newTestConsumer(t, fake.server(t).URL)

	if err := c.Want(Subscription{JobID: "job-1", InfoTypeID: "cell-state", DeliverTo: "/data"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	c.reconcile(ctx)
	if fake.jobCount() != 0 {
		t.Fatal("no job should exist while the type is unknown")
	}
	pending := c.Pending()
	if len(pending) != 1 || pending["job-1"] == "" {
		t.Errorf("pending = %#v, want job-1 with a reason", pending)
	}

	fake.addType("cell-state")
	c.reconcile(ctx)

	if fake.jobCount() != 1 {
		t.Error("the job should be placed once the type appears")
	}
	if len(c.Pending()) != 0 {
		t.Errorf("nothing should stay pending after success, got %#v", c.Pending())
	}
}

func TestReconcileDoesNotResubmitPlacedJobs(t *testing.T) {
	fake := newFakeICS("cell-state")
	c := newTestConsumer(t, fake.server(t).URL)

	if err := c.Want(Subscription{JobID: "job-1", InfoTypeID: "cell-state", DeliverTo: "/data"}); err != nil {
		t.Fatal(err)
	}

	c.reconcile(context.Background())
	c.reconcile(context.Background())

	if len(c.Pending()) != 0 {
		t.Error("a placed job should not be retried")
	}
}

func TestJobStatusReflectsProducerPresence(t *testing.T) {
	fake := newFakeICS("cell-state")
	c := newTestConsumer(t, fake.server(t).URL)
	ctx := context.Background()

	if err := c.Subscribe(ctx, Subscription{JobID: "job-1", InfoTypeID: "cell-state", DeliverTo: "/data"}); err != nil {
		t.Fatal(err)
	}

	st, err := c.JobStatus(ctx, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if st.Delivering() {
		t.Error("a job with no producer must not report as delivering")
	}

	fake.mu.Lock()
	fake.producer = 1
	fake.mu.Unlock()

	st, err = c.JobStatus(ctx, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Delivering() {
		t.Error("a job with a producer should report as delivering")
	}
}

// The consumer view of a type names its schema differently from the producer
// view, and carries producer availability the producer view does not.
func TestInfoTypeUsesTheConsumerShape(t *testing.T) {
	fake := newFakeICS("cell-state")
	c := newTestConsumer(t, fake.server(t).URL)
	ctx := context.Background()

	it, err := c.InfoType(ctx, "cell-state")
	if err != nil {
		t.Fatal(err)
	}
	if len(it.Schema) == 0 {
		t.Error("schema should be populated from job_data_schema")
	}
	if it.Available() {
		t.Error("a type with no producers is not available")
	}

	fake.mu.Lock()
	fake.producer = 2
	fake.mu.Unlock()

	it, err = c.InfoType(ctx, "cell-state")
	if err != nil {
		t.Fatal(err)
	}
	if !it.Available() {
		t.Error("a type with producers should be available")
	}
}

func TestUnsubscribeIsIdempotent(t *testing.T) {
	fake := newFakeICS("cell-state")
	c := newTestConsumer(t, fake.server(t).URL)
	ctx := context.Background()

	if err := c.Subscribe(ctx, Subscription{JobID: "job-1", InfoTypeID: "cell-state", DeliverTo: "/data"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Unsubscribe(ctx, "job-1"); err != nil {
		t.Fatal(err)
	}
	if err := c.Unsubscribe(ctx, "job-1"); err != nil {
		t.Errorf("unsubscribing twice should be a no-op, got %v", err)
	}
}
