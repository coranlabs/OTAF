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

package rapptest_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/a1"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/influx"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/ingest"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/r1"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/rapptest"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/sdnr"
)

type engine struct {
	store      *influx.Writer
	controller *sdnr.Client
	policies   *a1.Client

	mu   sync.Mutex
	seen []reading
}

type reading struct {
	Cell string  `json:"cell"`
	Load float64 `json:"load"`
}

func (e *engine) Handle(ctx context.Context, m ingest.Message) error {
	var r reading
	if err := json.Unmarshal(m.Payload, &r); err != nil {
		return err
	}
	if r.Cell == "" {
		return errors.New("reading has no cell")
	}

	e.mu.Lock()
	e.seen = append(e.seen, r)
	e.mu.Unlock()

	e.store.Point("cell_kpis",
		map[string]string{"cell": r.Cell},
		map[string]any{"load": r.Load},
		m.Received)

	if r.Load > 80 {
		ric, err := e.policies.RicFor(ctx, "20100", "me1")
		if err != nil {
			return err
		}
		data, _ := json.Marshal(map[string]any{"cell": r.Cell})
		return e.policies.PutPolicy(ctx, a1.Policy{
			ID: "relief-" + r.Cell, RicID: ric.ID, PolicyTypeID: "20100",
			Transient: true, Data: data,
		})
	}
	return nil
}

func (e *engine) Snapshot(ctx context.Context, job r1.Job) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.seen) == 0 {
		return nil, nil
	}
	return json.Marshal(map[string]any{"readings": e.seen})
}

func newEngine(t *testing.T) (*engine, *rapptest.PolicyManagement, *rapptest.TimeSeries, *rapptest.Controller) {
	t.Helper()

	policies, pms := rapptest.NewPolicyManagement(t)
	store, series := rapptest.NewTimeSeries(t)
	controller, ctrl := rapptest.NewController(t)

	return &engine{store: store, controller: controller, policies: policies}, pms, series, ctrl
}

func TestHarnessDrivesTheHandler(t *testing.T) {
	e, _, _, _ := newEngine(t)
	h := rapptest.NewHarness(t, e)

	h.SendAll("test", reading{Cell: "c1", Load: 10}, reading{Cell: "c2", Load: 20})

	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.seen) != 2 {
		t.Errorf("handler saw %d readings, want 2", len(e.seen))
	}
}

func TestHarnessSurfacesRejections(t *testing.T) {
	e, _, _, _ := newEngine(t)
	h := rapptest.NewHarness(t, e)

	err := h.SendExpectingError("test", reading{Load: 5})
	if !strings.Contains(err.Error(), "no cell") {
		t.Errorf("error = %v, want the handler's own message", err)
	}
}

// The point of the fake is asserting on the decision, not the transport.
func TestPolicyIsPlacedOnceLoadIsHigh(t *testing.T) {
	e, pms, _, _ := newEngine(t)
	h := rapptest.NewHarness(t, e)

	h.Send("test", reading{Cell: "c1", Load: 20})
	if pms.Count() != 0 {
		t.Fatal("a quiet cell should not produce a policy")
	}

	h.Send("test", reading{Cell: "c1", Load: 91})

	var data struct {
		Cell string `json:"cell"`
	}
	policy := pms.Policy(t, "relief-c1", &data)

	if data.Cell != "c1" {
		t.Errorf("policy names cell %q, want c1", data.Cell)
	}
	if policy.RicID != "test-ric" {
		t.Errorf("policy went to %q, want test-ric", policy.RicID)
	}
	if !policy.Transient {
		t.Error("a relief decision should be transient")
	}
}

func TestRejectedPolicySurfacesToTheHandler(t *testing.T) {
	e, pms, _, _ := newEngine(t)
	pms.Reject = true

	h := rapptest.NewHarness(t, e)
	err := h.SendExpectingError("test", reading{Cell: "c1", Load: 95})

	if !a1.IsRejected(err) {
		t.Errorf("error should classify as a rejection, got %v", err)
	}
}
