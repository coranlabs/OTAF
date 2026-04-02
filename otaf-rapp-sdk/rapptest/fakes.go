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

package rapptest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/a1"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/influx"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/retry"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/sdnr"
)

type PolicyManagement struct {
	mu       sync.Mutex
	policies map[string]a1.Policy
	rics     []a1.Ric

	// Reject makes the next policy fail the way a schema violation does.
	Reject bool
}

// NewPolicyManagement returns a fake and a client wired to it. RICs default to
// one available RIC supporting every type the test asks for.
func NewPolicyManagement(t testing.TB, rics ...a1.Ric) (*a1.Client, *PolicyManagement) {
	t.Helper()

	if len(rics) == 0 {
		rics = []a1.Ric{{
			ID:                "test-ric",
			State:             "AVAILABLE",
			ManagedElementIDs: []string{"me1"},
		}}
	}

	f := &PolicyManagement{policies: map[string]a1.Policy{}, rics: rics}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	client, err := a1.New(a1.Config{
		Endpoint:  srv.URL,
		ServiceID: "test-rapp",
	}, Logger(), a1.WithRetry(retry.None()))
	if err != nil {
		t.Fatalf("rapptest: could not build the A1 client: %v", err)
	}
	return client, f
}

// Policies returns everything the rApp has placed.
func (f *PolicyManagement) Policies() map[string]a1.Policy {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]a1.Policy, len(f.policies))
	for k, v := range f.policies {
		out[k] = v
	}
	return out
}

// Policy returns one placed policy, with its data decoded into dst.
func (f *PolicyManagement) Policy(t testing.TB, id string, dst any) a1.Policy {
	t.Helper()

	f.mu.Lock()
	p, ok := f.policies[id]
	f.mu.Unlock()

	if !ok {
		t.Fatalf("rapptest: no policy %q was placed", id)
	}
	if dst != nil {
		if err := json.Unmarshal(p.Data, dst); err != nil {
			t.Fatalf("rapptest: policy %q has undecodable data: %v", id, err)
		}
	}
	return p
}

func (f *PolicyManagement) Count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.policies)
}

func (f *PolicyManagement) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/a1-policy/v2/status", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success"}`))
	})

	mux.HandleFunc("/a1-policy/v2/rics", func(w http.ResponseWriter, r *http.Request) {
		wanted := r.URL.Query().Get("policytype_id")

		f.mu.Lock()
		out := make([]a1.Ric, 0, len(f.rics))
		for _, ric := range f.rics {
			// A RIC with no declared types stands in for one that accepts
			// whatever the test asks about.
			if wanted == "" || len(ric.PolicyTypeIDs) == 0 || ric.Supports(wanted) {
				if len(ric.PolicyTypeIDs) == 0 && wanted != "" {
					ric.PolicyTypeIDs = []string{wanted}
				}
				out = append(out, ric)
			}
		}
		f.mu.Unlock()

		body, _ := json.Marshal(map[string][]a1.Ric{"rics": out})
		_, _ = w.Write(body)
	})

	mux.HandleFunc("/a1-policy/v2/policy-types/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"policy_schema":{"type":"object"}}`))
	})

	mux.HandleFunc("/a1-policy/v2/services", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"service_list":[{"service_id":"test-rapp"}]}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
	})

	mux.HandleFunc("/a1-policy/v2/services/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/a1-policy/v2/policies", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			f.mu.Lock()
			reject := f.Reject
			f.mu.Unlock()

			if reject {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"status":400,"detail":""}`))
				return
			}

			var p a1.Policy
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.mu.Lock()
			f.policies[p.ID] = p
			f.mu.Unlock()
			w.WriteHeader(http.StatusCreated)

		case http.MethodGet:
			f.mu.Lock()
			ids := make([]string, 0, len(f.policies))
			for id := range f.policies {
				ids = append(ids, id)
			}
			f.mu.Unlock()
			body, _ := json.Marshal(map[string][]string{"policy_ids": ids})
			_, _ = w.Write(body)
		}
	})

	mux.HandleFunc("/a1-policy/v2/policies/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/a1-policy/v2/policies/")
		id = strings.TrimSuffix(id, "/status")

		f.mu.Lock()
		p, known := f.policies[id]
		if known && r.Method == http.MethodDelete {
			delete(f.policies, id)
		}
		f.mu.Unlock()

		if !known {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":404,"detail":"Could not find policy: ` + id + `"}`))
			return
		}
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/status"):
			_, _ = w.Write([]byte(`{"last_modified":"2026-01-01T00:00:00Z","status":{"enforceStatus":"ENFORCED"}}`))
		default:
			body, _ := json.Marshal(p)
			_, _ = w.Write(body)
		}
	})

	return mux
}

// Controller is a stand-in for the SMO controller, recording the RESTCONF
// writes an rApp made against the network.
type Controller struct {
	mu      sync.Mutex
	writes  []Write
	rejects bool
}

type Write struct {
	Method string
	Path   string
	Body   string
}
