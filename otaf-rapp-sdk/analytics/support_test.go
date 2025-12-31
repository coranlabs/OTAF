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
