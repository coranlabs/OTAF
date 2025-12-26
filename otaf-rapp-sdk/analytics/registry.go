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

package analytics

import (
	"sort"
	"sync"
	"time"
)

const (
	defaultHistory    = 16
	defaultStaleAfter = 5 * time.Minute
)

// Entity is what the registry knows about one thing it is watching.
type Entity[K any] struct {
	ID      string
	History *History[K]
	Verdict Verdict

	// Reported is the timestamp on the last accepted sample; Observed is when
	// it reached the rApp. They differ, and only the second says whether the
	// entity is still being heard from.
	Reported time.Time
	Observed time.Time
}

// Result is what one call to Observe concluded.
type Result struct {
	Entity string `json:"entity"`

	// Accepted is false when the sample was not newer than the last one, in
	// which case nothing was recorded and the verdict is unchanged.
	Accepted bool `json:"accepted"`

	Verdict  Verdict `json:"verdict"`
	Previous State   `json:"previous"`

	// Changed marks a verdict that differs from the one before it, which is
	// usually what is worth logging or acting on.
	Changed bool `json:"changed"`

	Samples int       `json:"samples"`
	At      time.Time `json:"at"`
}

// Registry holds per-entity state and applies a classifier to it. It is safe
// for concurrent use.
type Registry[K any] struct {
	mu       sync.RWMutex
	entities map[string]*Entity[K]

	classifier  Classifier[K]
	historySize int
	staleAfter  time.Duration
	now         func() time.Time
}

type RegistryOption[K any] func(*Registry[K])

// WithClassifier sets what turns samples into a verdict. Without one, the
// registry still keeps history but every verdict stays unknown.
func WithClassifier[K any](c Classifier[K]) RegistryOption[K] {
	return func(r *Registry[K]) { r.classifier = c }
}

// WithHistorySize bounds how many samples are kept per entity.
func WithHistorySize[K any](n int) RegistryOption[K] {
	return func(r *Registry[K]) {
		if n > 0 {
			r.historySize = n
		}
	}
}

// WithStaleAfter sets how long an entity may go unheard before Stale reports
// it. It is measured against arrival, not the timestamp inside the report, so
// a stopped feed is detected even if it was replaying old data.
func WithStaleAfter[K any](d time.Duration) RegistryOption[K] {
	return func(r *Registry[K]) {
		if d > 0 {
			r.staleAfter = d
		}
	}
}

// WithClock replaces the source of time, for tests.
func WithClock[K any](now func() time.Time) RegistryOption[K] {
	return func(r *Registry[K]) {
		if now != nil {
			r.now = now
		}
	}
}

func NewRegistry[K any](opts ...RegistryOption[K]) *Registry[K] {
	r := &Registry[K]{
		entities:    map[string]*Entity[K]{},
		historySize: defaultHistory,
		staleAfter:  defaultStaleAfter,
		now:         time.Now,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Observe records a sample and re-evaluates the entity. This is the one call
// an rApp makes per incoming report.
func (r *Registry[K]) Observe(id string, at time.Time, kpi K) Result {
	r.mu.Lock()
	defer r.mu.Unlock()

	if at.IsZero() {
		at = r.now()
	}

	entity, known := r.entities[id]
	if !known {
		entity = &Entity[K]{
			ID:      id,
			History: NewHistory[K](r.historySize),
			Verdict: Verdict{State: StateUnknown},
		}
		r.entities[id] = entity
	}

	previous := entity.Verdict.State

	if !entity.History.Append(Sample[K]{At: at, KPI: kpi}) {
		return Result{
			Entity:   id,
			Accepted: false,
			Verdict:  entity.Verdict,
			Previous: previous,
			Samples:  entity.History.Len(),
			At:       at,
		}
	}

	entity.Reported = at
	entity.Observed = r.now()

	if r.classifier != nil {
		entity.Verdict = r.classifier.Classify(entity.History.Samples())
	}

	return Result{
		Entity:   id,
		Accepted: true,
		Verdict:  entity.Verdict,
		Previous: previous,
		Changed:  entity.Verdict.State != previous,
		Samples:  entity.History.Len(),
		At:       at,
	}
}

// With runs fn against one entity while the registry is locked, for the cases
// a snapshot cannot serve. Do not retain the pointer beyond fn.
func (r *Registry[K]) With(id string, fn func(*Entity[K])) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	entity, ok := r.entities[id]
	if !ok {
		return false
	}
	fn(entity)
	return true
}

// Each visits every entity while the registry is locked, in id order.
func (r *Registry[K]) Each(fn func(*Entity[K])) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, id := range r.sortedIDs() {
		fn(r.entities[id])
	}
}

func (r *Registry[K]) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entities)
}
