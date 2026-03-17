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

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

func quietLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(discard{})
	return l
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func newTestProducer(t *testing.T, snap Snapshot) (*Producer, http.Handler) {
	t.Helper()
	p := NewProducer("demo-producer", snap, quietLogger())
	r := mux.NewRouter()
	p.Register(r)
	return p, r
}

func startJob(t *testing.T, h http.Handler, body string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, SubscriptionPath+"/job-1", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestSupervisionReportsHealthy(t *testing.T) {
	_, h := newTestProducer(t, nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, HealthPath, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("supervision status = %d, want 200", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "healthy" {
		t.Errorf("status = %v, want healthy", body["status"])
	}
}

func TestJobLifecycle(t *testing.T) {
	p, h := newTestProducer(t, nil)

	if code := startJob(t, h, `{"info_type_identity":"demo-state","target_uri":"http://consumer/data"}`); code != http.StatusOK {
		t.Fatalf("job start status = %d, want 200", code)
	}
	jobs := p.Jobs()
	if len(jobs) != 1 || jobs[0].ID != "job-1" {
		t.Fatalf("jobs = %#v, want one job called job-1", jobs)
	}

	req := httptest.NewRequest(http.MethodDelete, SubscriptionPath+"/job-1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("job stop status = %d, want 204", rec.Code)
	}
	if len(p.Jobs()) != 0 {
		t.Error("job should be gone after it is stopped")
	}
}

func TestJobWithoutTargetIsRejected(t *testing.T) {
	_, h := newTestProducer(t, nil)

	if code := startJob(t, h, `{"info_type_identity":"demo-state"}`); code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when the job has no target", code)
	}
}

func TestConsumerIntervalIsHonoured(t *testing.T) {
	p, h := newTestProducer(t, nil)

	startJob(t, h, `{"target_uri":"http://consumer/data","info_job_data":{"delivery_interval_seconds":90}}`)

	jobs := p.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("expected one job, got %d", len(jobs))
	}
	if jobs[0].Interval() != 90*time.Second {
		t.Errorf("interval = %v, want 90s", jobs[0].Interval())
	}
}
