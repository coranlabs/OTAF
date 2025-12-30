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
	"sync"
	"testing"
	"time"
)

type kpi struct {
	Value float64
}

// A deliberately naive classifier: the SDK ships none, so tests bring their
// own, exactly as an rApp does.
func threshold(limit float64) Classifier[kpi] {
	return ClassifierFunc[kpi]{
		Label: "threshold",
		Fn: func(samples []Sample[kpi]) Verdict {
			latest := samples[len(samples)-1].KPI.Value
			state := State("LOW")
			if latest > limit {
				state = State("HIGH")
			}
			return Verdict{State: state, Score: latest, Signals: map[string]float64{"value": latest}}
		},
	}
}

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *clock {
	return &clock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestObserveRecordsAndClassifies(t *testing.T) {
	c := newClock()
	r := NewRegistry(
		WithClassifier[kpi](threshold(80)),
		WithClock[kpi](c.Now),
	)

	got := r.Observe("cell-1", c.Now(), kpi{Value: 20})

	if !got.Accepted {
		t.Fatal("a first sample should be accepted")
	}
	if got.Verdict.State != "LOW" {
		t.Errorf("state = %s, want LOW", got.Verdict.State)
	}
	if got.Previous != StateUnknown {
		t.Errorf("previous = %s, want UNKNOWN for a new entity", got.Previous)
	}
	if !got.Changed {
		t.Error("moving off unknown is a change")
	}
	if got.Samples != 1 {
		t.Errorf("samples = %d, want 1", got.Samples)
	}
}

// Acting only on a change is the usual pattern; the registry has to report it
// so every rApp does not track the previous state itself.
func TestChangedMarksVerdictTransitions(t *testing.T) {
	c := newClock()
	r := NewRegistry(WithClassifier[kpi](threshold(80)), WithClock[kpi](c.Now))

	r.Observe("cell-1", c.Now(), kpi{Value: 10})

	c.advance(time.Minute)
	steady := r.Observe("cell-1", c.Now(), kpi{Value: 20})
	if steady.Changed {
		t.Error("staying LOW is not a change")
	}

	c.advance(time.Minute)
	crossed := r.Observe("cell-1", c.Now(), kpi{Value: 95})
	if !crossed.Changed {
		t.Error("crossing the threshold is a change")
	}
	if crossed.Previous != "LOW" || crossed.Verdict.State != "HIGH" {
		t.Errorf("transition = %s -> %s, want LOW -> HIGH", crossed.Previous, crossed.Verdict.State)
	}
}

// A repeated report must not enter the window twice, or a trend computed from
// it is wrong in a way nothing surfaces.
func TestRepeatedOrOlderSamplesAreRejected(t *testing.T) {
	c := newClock()
	r := NewRegistry(WithClassifier[kpi](threshold(80)), WithClock[kpi](c.Now))

	at := c.Now()
	r.Observe("cell-1", at, kpi{Value: 10})

	same := r.Observe("cell-1", at, kpi{Value: 99})
	if same.Accepted {
		t.Error("a sample with the same timestamp should be rejected")
	}
	if same.Verdict.State != "LOW" {
		t.Errorf("state = %s, want the rejected sample to have changed nothing", same.Verdict.State)
	}

	older := r.Observe("cell-1", at.Add(-time.Minute), kpi{Value: 99})
	if older.Accepted {
		t.Error("an older sample should be rejected")
	}
	if r.Len() != 1 {
		t.Errorf("entities = %d, want 1", r.Len())
	}
}

func TestHistoryIsBounded(t *testing.T) {
	c := newClock()
	r := NewRegistry(WithHistorySize[kpi](3), WithClock[kpi](c.Now))

	for i := 0; i < 10; i++ {
		c.advance(time.Second)
		r.Observe("cell-1", c.Now(), kpi{Value: float64(i)})
	}

	latest, _ := r.Latest("cell-1")
	if latest.KPI.Value != 9 {
		t.Errorf("latest = %v, want the newest sample", latest.KPI.Value)
	}

	var held int
	r.With("cell-1", func(e *Entity[kpi]) { held = e.History.Len() })
	if held != 3 {
		t.Errorf("history = %d, want it capped at 3", held)
	}
}

func TestWithoutClassifierStateStaysUnknown(t *testing.T) {
	c := newClock()
	r := NewRegistry(WithClock[kpi](c.Now))

	got := r.Observe("cell-1", c.Now(), kpi{Value: 99})

	if !got.Accepted {
		t.Fatal("samples should still be recorded without a classifier")
	}
	if got.Verdict.State != StateUnknown {
		t.Errorf("state = %s, want UNKNOWN", got.Verdict.State)
	}
}

// Freshness is measured against arrival, so a feed that stopped is detected
// even when it was replaying old timestamps.
func TestStalenessFollowsArrivalNotReportTime(t *testing.T) {
	c := newClock()
	r := NewRegistry(WithStaleAfter[kpi](time.Minute), WithClock[kpi](c.Now))

	r.Observe("cell-1", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), kpi{Value: 1})

	if !r.Fresh("cell-1") {
		t.Error("an old timestamp that just arrived is still fresh")
	}

	c.advance(2 * time.Minute)
	if r.Fresh("cell-1") {
		t.Error("an entity unheard from beyond the window is stale")
	}
	if stale := r.Stale(); len(stale) != 1 || stale[0] != "cell-1" {
		t.Errorf("stale = %v, want [cell-1]", stale)
	}
}

func TestUnknownEntityIsNotFresh(t *testing.T) {
	r := NewRegistry[kpi]()
	if r.Fresh("nobody") {
		t.Error("an entity that was never seen cannot be fresh")
	}
}

func TestEvictForgetsQuietEntities(t *testing.T) {
	c := newClock()
	r := NewRegistry(WithClock[kpi](c.Now))

	r.Observe("cell-1", c.Now(), kpi{Value: 1})
	c.advance(10 * time.Minute)
	r.Observe("cell-2", c.Now(), kpi{Value: 1})

	dropped := r.Evict(5 * time.Minute)

	if len(dropped) != 1 || dropped[0] != "cell-1" {
		t.Errorf("dropped = %v, want [cell-1]", dropped)
	}
	if r.Len() != 1 {
		t.Errorf("entities = %d, want 1", r.Len())
	}
}

// Forgetting everything is never what a caller means.
func TestEvictIgnoresNonPositivePeriods(t *testing.T) {
	c := newClock()
	r := NewRegistry(WithClock[kpi](c.Now))
	r.Observe("cell-1", c.Now(), kpi{Value: 1})

	if dropped := r.Evict(0); dropped != nil {
		t.Errorf("dropped = %v, want nothing", dropped)
	}
	if r.Len() != 1 {
		t.Error("a zero period should evict nothing")
	}
}

func TestSnapshotIsOrderedAndCopied(t *testing.T) {
	c := newClock()
	r := NewRegistry(WithClassifier[kpi](threshold(80)), WithClock[kpi](c.Now))

	for _, id := range []string{"cell-3", "cell-1", "cell-2"} {
		r.Observe(id, c.Now(), kpi{Value: 10})
		c.advance(time.Second)
	}

	snap := r.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("snapshot has %d entries, want 3", len(snap))
	}
	for i, want := range []string{"cell-1", "cell-2", "cell-3"} {
		if snap[i].ID != want {
			t.Errorf("snapshot[%d] = %s, want %s", i, snap[i].ID, want)
		}
	}

	snap[0].State = "TAMPERED"
	if again := r.Snapshot(); again[0].State == "TAMPERED" {
		t.Error("mutating a snapshot must not affect the registry")
	}
}

func TestStatesCountsByVerdict(t *testing.T) {
	c := newClock()
	r := NewRegistry(WithClassifier[kpi](threshold(80)), WithClock[kpi](c.Now))

	r.Observe("cell-1", c.Now(), kpi{Value: 10})
	r.Observe("cell-2", c.Now(), kpi{Value: 90})
	r.Observe("cell-3", c.Now(), kpi{Value: 95})

	states := r.States()
	if states["LOW"] != 1 || states["HIGH"] != 2 {
		t.Errorf("states = %v, want one LOW and two HIGH", states)
	}
}

func TestForgetRemovesAnEntity(t *testing.T) {
	c := newClock()
	r := NewRegistry(WithClock[kpi](c.Now))
	r.Observe("cell-1", c.Now(), kpi{Value: 1})

	if !r.Forget("cell-1") {
		t.Error("forgetting a known entity should report true")
	}
	if r.Forget("cell-1") {
		t.Error("forgetting an unknown entity should report false")
	}
	if r.Len() != 0 {
		t.Error("the entity should be gone")
	}
}
