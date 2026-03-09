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

package a1

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/retry"
)

func quietLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(discard{})
	return l
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// The responses below mirror what A1 policy management actually returns.
type fakePMS struct {
	mu       sync.Mutex
	policies map[string]json.RawMessage
	services map[string]bool
	requests []string

	rejectPolicy bool
	dropService  bool
}

func newFakePMS() *fakePMS {
	return &fakePMS{policies: map[string]json.RawMessage{}, services: map[string]bool{}}
}

func (f *fakePMS) record(r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
}

func (f *fakePMS) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.requests))
	copy(out, f.requests)
	return out
}

func (f *fakePMS) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/a1-policy/v2/status", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	})

	mux.HandleFunc("/a1-policy/v2/rics", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		if r.URL.Query().Get("policytype_id") == "unknown" {
			_, _ = w.Write([]byte(`{"rics":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"rics":[
			{"ric_id":"ric-down","managed_element_ids":["me1"],"state":"UNAVAILABLE","policytype_ids":["20100"]},
			{"ric_id":"ric-a","managed_element_ids":["me9"],"state":"AVAILABLE","policytype_ids":["20100"]},
			{"ric_id":"ric-b","managed_element_ids":["me1","me2"],"state":"AVAILABLE","policytype_ids":["20100"]}
		]}`))
	})

	mux.HandleFunc("/a1-policy/v2/policy-types/20100", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		_, _ = w.Write([]byte(`{"policy_schema":{"type":"object"}}`))
	})

	mux.HandleFunc("/a1-policy/v2/policies", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		switch r.Method {
		case http.MethodPut:
			if f.rejectPolicy {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"about:blank","status":400,"detail":""}`))
				return
			}
			var p Policy
			_ = json.NewDecoder(r.Body).Decode(&p)
			f.mu.Lock()
			f.policies[p.ID] = p.Data
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
		f.record(r)
		id := r.URL.Path[len("/a1-policy/v2/policies/"):]

		f.mu.Lock()
		_, known := f.policies[id]
		f.mu.Unlock()

		if !known {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"type":"about:blank","status":404,"detail":"Could not find policy: ` + id + `"}`))
			return
		}
		if r.Method == http.MethodDelete {
			f.mu.Lock()
			delete(f.policies, id)
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		}
	})

	mux.HandleFunc("/a1-policy/v2/services", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		switch r.Method {
		case http.MethodPut:
			var reg Registration
			_ = json.NewDecoder(r.Body).Decode(&reg)
			f.mu.Lock()
			f.services[reg.ServiceID] = true
			f.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			id := r.URL.Query().Get("service_id")
			f.mu.Lock()
			known := f.services[id]
			f.mu.Unlock()
			if !known {
				_, _ = w.Write([]byte(`{"service_list":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"service_list":[{"service_id":"` + id + `","keep_alive_interval_seconds":300}]}`))
		}
	})

	mux.HandleFunc("/a1-policy/v2/services/", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		if f.dropService {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"type":"about:blank","status":404,"detail":"service not found"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(t *testing.T, endpoint string, opts ...Option) *Client {
	t.Helper()
	c, err := New(Config{
		Endpoint:  endpoint,
		ServiceID: "test-rapp",
		KeepAlive: 90 * time.Second,
	}, quietLogger(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDisabledWhenNoEndpoint(t *testing.T) {
	c, err := New(Config{}, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if c != nil {
		t.Fatal("an unconfigured endpoint should yield a nil client")
	}
}

func TestServiceIDIsRequired(t *testing.T) {
	if _, err := New(Config{Endpoint: "http://pms"}, quietLogger()); err == nil {
		t.Fatal("expected an error when no service id is configured")
	}
}

func TestRicForPrefersTheManagedElement(t *testing.T) {
	srv := newFakePMS().server(t)
	c := newTestClient(t, srv.URL)

	ric, err := c.RicFor(context.Background(), "20100", "me2")
	if err != nil {
		t.Fatal(err)
	}
	if ric.ID != "ric-b" {
		t.Errorf("selected %s, want ric-b which manages me2", ric.ID)
	}
}

func TestRicForSkipsUnavailableRics(t *testing.T) {
	srv := newFakePMS().server(t)
	c := newTestClient(t, srv.URL)

	ric, err := c.RicFor(context.Background(), "20100", "me1")
	if err != nil {
		t.Fatal(err)
	}
	if ric.ID == "ric-down" {
		t.Error("an unavailable RIC must never be selected, even when it manages the element")
	}
}

func TestRicForFailsWhenNothingSupportsTheType(t *testing.T) {
	srv := newFakePMS().server(t)
	c := newTestClient(t, srv.URL)

	if _, err := c.RicFor(context.Background(), "unknown", ""); err == nil {
		t.Fatal("expected an error when no RIC supports the policy type")
	}
}

func TestPutPolicyFillsInTheServiceID(t *testing.T) {
	fake := newFakePMS()
	srv := fake.server(t)
	c := newTestClient(t, srv.URL)

	err := c.PutPolicy(context.Background(), Policy{
		ID:           "p1",
		RicID:        "ric-b",
		PolicyTypeID: "20100",
		Data:         json.RawMessage(`{"plmn":"00101"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	ids, err := c.Policies(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "p1" {
		t.Errorf("policies = %v, want [p1]", ids)
	}
}

func TestPutPolicyRequiresIdentifiers(t *testing.T) {
	srv := newFakePMS().server(t)
	c := newTestClient(t, srv.URL)
	ctx := context.Background()

	if err := c.PutPolicy(ctx, Policy{RicID: "ric-b"}); err == nil {
		t.Error("a policy without an id should be rejected before it is sent")
	}
	if err := c.PutPolicy(ctx, Policy{ID: "p1"}); err == nil {
		t.Error("a policy without a RIC should be rejected before it is sent")
	}
}

// A schema rejection arrives with an empty detail, so the client has to supply
// something the operator can act on.
func TestRejectedPolicyIsClassified(t *testing.T) {
	fake := newFakePMS()
	fake.rejectPolicy = true
	srv := fake.server(t)
	c := newTestClient(t, srv.URL)

	err := c.PutPolicy(context.Background(), Policy{ID: "p1", RicID: "ric-b", PolicyTypeID: "20100"})
	if err == nil {
		t.Fatal("expected the policy to be rejected")
	}
	if !IsRejected(err) {
		t.Errorf("error should classify as rejected, got %v", err)
	}
	if IsNotFound(err) {
		t.Error("a rejection must not classify as not-found")
	}
	if len(err.Error()) < 30 {
		t.Errorf("error should explain the likely cause, got %q", err.Error())
	}
}

func TestMissingPolicyIsNotFound(t *testing.T) {
	srv := newFakePMS().server(t)
	c := newTestClient(t, srv.URL)

	_, err := c.Policy(context.Background(), "absent")
	if !IsNotFound(err) {
		t.Errorf("expected a not-found classification, got %v", err)
	}
}

// Cleanup paths stay simple when withdrawing something already gone succeeds.
func TestDeletingAnAbsentPolicySucceeds(t *testing.T) {
	srv := newFakePMS().server(t)
	c := newTestClient(t, srv.URL)

	if err := c.DeletePolicy(context.Background(), "absent"); err != nil {
		t.Errorf("deleting an absent policy should be a no-op, got %v", err)
	}
}

func TestDeleteAllPoliciesWithdrawsOurOwn(t *testing.T) {
	srv := newFakePMS().server(t)
	c := newTestClient(t, srv.URL)
	ctx := context.Background()

	for _, id := range []string{"p1", "p2"} {
		if err := c.PutPolicy(ctx, Policy{ID: id, RicID: "ric-b", PolicyTypeID: "20100"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.DeleteAllPolicies(ctx); err != nil {
		t.Fatal(err)
	}

	ids, err := c.Policies(ctx, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Errorf("policies remaining = %v, want none", ids)
	}
}

func TestRegisterAndKeepAlive(t *testing.T) {
	fake := newFakePMS()
	srv := fake.server(t)
	c := newTestClient(t, srv.URL)
	ctx := context.Background()

	if err := c.Register(ctx); err != nil {
		t.Fatal(err)
	}
	ok, err := c.Registered(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("the service should be registered")
	}
	if err := c.KeepAlive(ctx); err != nil {
		t.Fatal(err)
	}
}

// If the platform forgets the service, the heartbeat has to register again or
// the rApp silently loses the ability to place policies.
func TestHeartbeatReregistersAfterTheServiceIsForgotten(t *testing.T) {
	fake := newFakePMS()
	fake.dropService = true
	srv := fake.server(t)
	c := newTestClient(t, srv.URL)

	c.heartbeat(context.Background())

	var registrations int
	for _, call := range fake.calls() {
		if call == "PUT /a1-policy/v2/services" {
			registrations++
		}
	}
	if registrations == 0 {
		t.Error("a lost registration should trigger a fresh register call")
	}
}

func TestStopDoesNotDeregisterByDefault(t *testing.T) {
	fake := newFakePMS()
	srv := fake.server(t)
	c := newTestClient(t, srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}

	for _, call := range fake.calls() {
		if call == "DELETE /a1-policy/v2/services/test-rapp" {
			t.Fatal("stopping must not withdraw the rApp's policies; a restart would revert the network")
		}
	}
}
