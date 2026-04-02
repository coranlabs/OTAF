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

type Write struct {
	Method string
	Path   string
	Body   string
}
