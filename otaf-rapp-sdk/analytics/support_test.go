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
