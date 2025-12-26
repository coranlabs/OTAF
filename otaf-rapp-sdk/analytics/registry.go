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

// Verdict returns one entity's current verdict.
func (r *Registry[K]) Verdict(id string) (Verdict, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entity, ok := r.entities[id]
	if !ok {
		return Verdict{}, false
	}
	return entity.Verdict, true
}

// Latest returns the most recent sample recorded for an entity.
func (r *Registry[K]) Latest(id string) (Sample[K], bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entity, ok := r.entities[id]
	if !ok {
		var zero Sample[K]
		return zero, false
	}
	return entity.History.Latest()
}

// View is a copy of one entity, safe to hold and to serialise.
type View struct {
	ID       string    `json:"id"`
	State    State     `json:"state"`
	Verdict  Verdict   `json:"verdict"`
	Samples  int       `json:"samples"`
	Reported time.Time `json:"reported_at"`
	Observed time.Time `json:"observed_at"`
	Stale    bool      `json:"stale"`
}

// Snapshot copies every entity, in id order, for a status endpoint or a
// delivered payload.
func (r *Registry[K]) Snapshot() []View {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := r.now()
	out := make([]View, 0, len(r.entities))
	for _, id := range r.sortedIDs() {
		entity := r.entities[id]
		out = append(out, View{
			ID:       entity.ID,
			State:    entity.Verdict.State,
			Verdict:  entity.Verdict,
			Samples:  entity.History.Len(),
			Reported: entity.Reported,
			Observed: entity.Observed,
			Stale:    r.isStale(entity, now),
		})
	}
	return out
}

// States counts entities by verdict, which is the usual thing to publish.
func (r *Registry[K]) States() map[State]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := map[State]int{}
	for _, entity := range r.entities {
		out[entity.Verdict.State]++
	}
	return out
}

// Fresh reports whether an entity has been heard from recently enough to act
// on. An entity nobody has heard from is not evidence of anything.
func (r *Registry[K]) Fresh(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entity, ok := r.entities[id]
	return ok && !r.isStale(entity, r.now())
}

// Stale lists entities that have gone quiet, in id order.
func (r *Registry[K]) Stale() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := r.now()
	var out []string
	for _, id := range r.sortedIDs() {
		if r.isStale(r.entities[id], now) {
			out = append(out, id)
		}
	}
	return out
}

// Evict forgets entities unheard from for the given period, so a registry
// tracking a changing population does not grow without bound. A zero or
// negative period is ignored, since forgetting everything is never intended.
func (r *Registry[K]) Evict(after time.Duration) []string {
	if after <= 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	var dropped []string
	for _, id := range r.sortedIDs() {
		if now.Sub(r.entities[id].Observed) > after {
			delete(r.entities, id)
			dropped = append(dropped, id)
		}
	}
	return dropped
}

// Forget removes one entity.
func (r *Registry[K]) Forget(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, ok := r.entities[id]
	delete(r.entities, id)
	return ok
}

func (r *Registry[K]) isStale(e *Entity[K], now time.Time) bool {
	if e.Observed.IsZero() {
		return true
	}
	return now.Sub(e.Observed) > r.staleAfter
}

// Callers hold the lock.
func (r *Registry[K]) sortedIDs() []string {
	ids := make([]string, 0, len(r.entities))
	for id := range r.entities {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
