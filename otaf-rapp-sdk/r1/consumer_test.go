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
