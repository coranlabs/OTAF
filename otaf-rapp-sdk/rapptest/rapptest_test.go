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
