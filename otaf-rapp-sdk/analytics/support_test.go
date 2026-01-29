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
	"math"
	"sync"
	"testing"
	"time"
)

func TestHistoryRejectsOutOfOrderSamples(t *testing.T) {
	h := NewHistory[kpi](4)
	base := time.Now()

	if !h.Append(Sample[kpi]{At: base, KPI: kpi{Value: 1}}) {
		t.Fatal("the first sample should be accepted")
	}
	if h.Append(Sample[kpi]{At: base, KPI: kpi{Value: 2}}) {
		t.Error("a sample with the same timestamp should be rejected")
	}
	if h.Append(Sample[kpi]{At: base.Add(-time.Second), KPI: kpi{Value: 3}}) {
		t.Error("an older sample should be rejected")
	}
	if h.Len() != 1 {
		t.Errorf("len = %d, want 1", h.Len())
	}
}

func TestHistoryDropsOldestWhenFull(t *testing.T) {
	h := NewHistory[kpi](3)
	base := time.Now()

	for i := 0; i < 5; i++ {
		h.Append(Sample[kpi]{At: base.Add(time.Duration(i) * time.Second), KPI: kpi{Value: float64(i)}})
	}

	if h.Len() != 3 {
		t.Fatalf("len = %d, want 3", h.Len())
	}
	oldest, _ := h.Oldest()
	latest, _ := h.Latest()
	if oldest.KPI.Value != 2 || latest.KPI.Value != 4 {
		t.Errorf("window = %v..%v, want 2..4", oldest.KPI.Value, latest.KPI.Value)
	}
}

// Samples say nothing about how long they took to arrive, so a classifier
// needing a minimum period of evidence asks for the span.
func TestHistorySpan(t *testing.T) {
	h := NewHistory[kpi](8)
	base := time.Now()

	if h.Span() != 0 {
		t.Error("an empty window spans nothing")
	}
	h.Append(Sample[kpi]{At: base, KPI: kpi{}})
	if h.Span() != 0 {
		t.Error("a single sample spans nothing")
	}
	h.Append(Sample[kpi]{At: base.Add(90 * time.Second), KPI: kpi{}})
	if h.Span() != 90*time.Second {
		t.Errorf("span = %v, want 90s", h.Span())
	}
}

func TestHistoryValuesExtractsOneField(t *testing.T) {
	h := NewHistory[kpi](4)
	base := time.Now()
	for i := 1; i <= 3; i++ {
		h.Append(Sample[kpi]{At: base.Add(time.Duration(i) * time.Second), KPI: kpi{Value: float64(i * 10)}})
	}

	values := h.Values(func(k kpi) float64 { return k.Value })
	if len(values) != 3 || values[0] != 10 || values[2] != 30 {
		t.Errorf("values = %v, want [10 20 30]", values)
	}
}

func TestHistoryLimitIsAtLeastOne(t *testing.T) {
	if got := NewHistory[kpi](0).Limit(); got != 1 {
		t.Errorf("limit = %d, want 1", got)
	}
}

func TestJournalKeepsTheMostRecent(t *testing.T) {
	j := NewJournal[int](3)
	for i := 1; i <= 5; i++ {
		j.Append(i)
	}

	entries := j.Entries()
	if len(entries) != 3 || entries[0] != 3 || entries[2] != 5 {
		t.Errorf("entries = %v, want [3 4 5]", entries)
	}
	// Total distinguishes a quiet rApp from one whose journal has rolled.
	if j.Total() != 5 {
		t.Errorf("total = %d, want 5", j.Total())
	}
	if latest, ok := j.Latest(); !ok || latest != 5 {
		t.Errorf("latest = %v, want 5", latest)
	}
}

func TestJournalRecent(t *testing.T) {
	j := NewJournal[int](10)
	for i := 1; i <= 5; i++ {
		j.Append(i)
	}

	if got := j.Recent(2); len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Errorf("recent = %v, want [4 5]", got)
	}
	if got := j.Recent(99); len(got) != 5 {
		t.Errorf("asking for more than exists should return everything, got %d", len(got))
	}
}

func TestJournalEntriesAreCopied(t *testing.T) {
	j := NewJournal[int](3)
	j.Append(1)

	entries := j.Entries()
	entries[0] = 99

	if again := j.Entries(); again[0] != 1 {
		t.Error("mutating the returned slice must not affect the journal")
	}
}

func TestJournalIsConcurrencySafe(t *testing.T) {
	j := NewJournal[int](32)

	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				j.Append(i)
				j.Entries()
				j.Len()
			}
		}()
	}
	wg.Wait()

	if j.Total() != 800 {
		t.Errorf("total = %d, want 800", j.Total())
	}
}

// Without a guard, an rApp reacting to a KPI it influences acts on every
// report until the KPI catches up.
func TestCooldownBlocksRepeatActions(t *testing.T) {
	guard := NewCooldown(time.Minute)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if !guard.Take("cell-1", at) {
		t.Fatal("the first action should be allowed")
	}
	if guard.Take("cell-1", at.Add(30*time.Second)) {
		t.Error("a second action within the period should be blocked")
	}
	if !guard.Take("cell-2", at) {
		t.Error("another entity should be unaffected")
	}
	if !guard.Take("cell-1", at.Add(61*time.Second)) {
		t.Error("the guard should lift once the period has passed")
	}
}

// Time is the caller's, so logic driven by sample timestamps behaves the same
// as logic driven by the wall clock.
func TestCooldownFollowsTheCallersClock(t *testing.T) {
	guard := NewCooldown(2 * time.Minute)
	replay := time.Date(2020, 6, 1, 12, 0, 0, 0, time.UTC)

	if !guard.Take("cell-1", replay) {
		t.Fatal("the first action should be allowed")
	}
	if !guard.Take("cell-1", replay.Add(3*time.Minute)) {
		t.Error("advancing the data's timeline should lift the guard, whatever the wall clock says")
	}
}

// Allow and Mark are separate so a failed attempt does not consume the guard.
func TestAllowDoesNotClaimTheGuard(t *testing.T) {
	guard := NewCooldown(time.Minute)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if !guard.Allow("cell-1", at) {
		t.Fatal("a fresh key should be allowed")
	}
	if !guard.Allow("cell-1", at) {
		t.Error("Allow must not claim the guard, so a failed action can retry")
	}

	guard.Mark("cell-1", at)
	if guard.Allow("cell-1", at) {
		t.Error("the guard should hold once marked")
	}
}

func TestCooldownRemainingCountsDown(t *testing.T) {
	guard := NewCooldown(time.Minute)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if guard.Remaining("cell-1", at) != 0 {
		t.Error("an unclaimed key has nothing remaining")
	}

	guard.Mark("cell-1", at)

	if got := guard.Remaining("cell-1", at.Add(20*time.Second)); got != 40*time.Second {
		t.Errorf("remaining = %v, want 40s", got)
	}
	if got := guard.Remaining("cell-1", at.Add(80*time.Second)); got != 0 {
		t.Errorf("remaining = %v, want 0 once elapsed", got)
	}
}

func TestZeroCooldownAllowsEverything(t *testing.T) {
	guard := NewCooldown(0)
	at := time.Now()

	for i := 0; i < 3; i++ {
		if !guard.Take("cell-1", at) {
			t.Fatal("a zero period should never block")
		}
	}
}

func TestCooldownClearAndEvict(t *testing.T) {
	guard := NewCooldown(time.Minute)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	guard.Mark("cell-1", at)
	guard.Clear("cell-1")
	if !guard.Allow("cell-1", at) {
		t.Error("clearing should free the key immediately")
	}

	guard.Mark("cell-2", at)
	if dropped := guard.Evict(at.Add(5 * time.Minute)); dropped != 1 {
		t.Errorf("evicted %d, want 1", dropped)
	}
	if guard.Len() != 0 {
		t.Errorf("tracked keys = %d, want 0", guard.Len())
	}
}

func TestBucketsAccumulateIntoSlots(t *testing.T) {
	b := NewBuckets(time.Hour, 4)
	base := time.Date(2026, 1, 1, 10, 30, 0, 0, time.UTC)

	b.Incr(base, "acted")
	b.Incr(base.Add(10*time.Minute), "acted")
	b.Add(base, "moved", 2.5)
	b.Incr(base.Add(time.Hour), "acted")

	window := b.Window(base.Add(time.Hour))
	if len(window) != 4 {
		t.Fatalf("window has %d slots, want 4", len(window))
	}

	last := window[len(window)-1]
	if last.Values["acted"] != 1 {
		t.Errorf("newest slot acted = %v, want 1", last.Values["acted"])
	}

	previous := window[len(window)-2]
	if previous.Values["acted"] != 2 {
		t.Errorf("previous slot acted = %v, want 2", previous.Values["acted"])
	}
	if previous.Values["moved"] != 2.5 {
		t.Errorf("previous slot moved = %v, want 2.5", previous.Values["moved"])
	}

	if total := b.Total(base.Add(time.Hour), "acted"); total != 3 {
		t.Errorf("total = %v, want 3", total)
	}
}

// A chart needs a point for every interval, including the quiet ones.
func TestEmptySlotsAreReturnedNotOmitted(t *testing.T) {
	b := NewBuckets(time.Hour, 3)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	b.Incr(base, "acted")

	window := b.Window(base)
	if len(window) != 3 {
		t.Fatalf("window has %d slots, want 3", len(window))
	}
	for i, slot := range window[:2] {
		if len(slot.Values) != 0 {
			t.Errorf("slot %d should be empty, got %v", i, slot.Values)
		}
		if slot.Start.IsZero() {
			t.Errorf("slot %d should still carry its start time", i)
		}
	}
}

// Slots are wall-clock aligned so a restart does not shift the boundaries.
func TestSlotsAreAlignedToTheClock(t *testing.T) {
	b := NewBuckets(2*time.Hour, 12)
	at := time.Date(2026, 1, 1, 9, 47, 13, 0, time.UTC)

	b.Incr(at, "acted")
	window := b.Window(at)
	newest := window[len(window)-1]

	want := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	if !newest.Start.Equal(want) {
		t.Errorf("slot start = %v, want %v", newest.Start, want)
	}
}

// A slot reused after a full rotation must not carry the old period's counts.
func TestSlotsResetOnRotation(t *testing.T) {
	b := NewBuckets(time.Hour, 2)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	b.Incr(base, "acted")
	later := base.Add(2 * time.Hour)
	b.Incr(later, "acted")

	window := b.Window(later)
	newest := window[len(window)-1]
	if newest.Values["acted"] != 1 {
		t.Errorf("reused slot = %v, want it reset to 1", newest.Values["acted"])
	}
}

func TestBucketWindowIsCopied(t *testing.T) {
	b := NewBuckets(time.Hour, 2)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b.Incr(at, "acted")

	window := b.Window(at)
	window[len(window)-1].Values["acted"] = 99

	again := b.Window(at)
	if again[len(again)-1].Values["acted"] != 1 {
		t.Error("mutating a returned window must not affect the buckets")
	}
}

func TestStatsOverAWindow(t *testing.T) {
	values := []float64{10, 20, 30, 40}

	if got := Mean(values); got != 25 {
		t.Errorf("mean = %v, want 25", got)
	}
	if got := Min(values); got != 10 {
		t.Errorf("min = %v, want 10", got)
	}
	if got := Max(values); got != 40 {
		t.Errorf("max = %v, want 40", got)
	}
	if got := Last(values); got != 40 {
		t.Errorf("last = %v, want 40", got)
	}
	if got := Slope(values); math.Abs(got-10) > 1e-9 {
		t.Errorf("slope = %v, want 10 per sample", got)
	}
	if got := ChangePct(values); math.Abs(got-300) > 1e-9 {
		t.Errorf("change = %v%%, want 300", got)
	}
	if got := Percentile(values, 0.5); math.Abs(got-25) > 1e-9 {
		t.Errorf("median = %v, want 25", got)
	}
	if got := StdDev([]float64{2, 2, 2}); got != 0 {
		t.Errorf("stddev of constants = %v, want 0", got)
	}
}

// A short or gappy window must not need guarding at every call site.
func TestStatsToleratesEmptyAndNaN(t *testing.T) {
	empty := []float64{}
	for name, got := range map[string]float64{
		"mean":   Mean(empty),
		"min":    Min(empty),
		"max":    Max(empty),
		"last":   Last(empty),
		"slope":  Slope(empty),
		"change": ChangePct(empty),
		"pct":    Percentile(empty, 0.9),
		"stddev": StdDev(empty),
	} {
		if got != 0 {
			t.Errorf("%s over an empty window = %v, want 0", name, got)
		}
	}

	gappy := []float64{10, math.NaN(), 30}
	if got := Mean(gappy); got != 20 {
		t.Errorf("mean ignoring NaN = %v, want 20", got)
	}
	if got := Max(gappy); got != 30 {
		t.Errorf("max ignoring NaN = %v, want 30", got)
	}
	if got := Last(gappy); got != 30 {
		t.Errorf("last ignoring NaN = %v, want 30", got)
	}
}

func TestChangePctFromZeroIsZero(t *testing.T) {
	if got := ChangePct([]float64{0, 50}); got != 0 {
		t.Errorf("change from zero = %v, want 0: a proportion of nothing is meaningless", got)
	}
}
