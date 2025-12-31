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
