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

import "time"

// History is a bounded window of samples, oldest first. It exists so a
// classifier can look at a trend without every rApp reimplementing the ring.
type History[K any] struct {
	samples []Sample[K]
	limit   int
}

// NewHistory keeps at most limit samples. A limit below one is treated as one.
func NewHistory[K any](limit int) *History[K] {
	if limit < 1 {
		limit = 1
	}
	return &History[K]{limit: limit, samples: make([]Sample[K], 0, limit)}
}

// Append records a sample and drops the oldest once the window is full. It
// reports false for a sample that is not newer than the last one, which is how
// a repeated or out-of-order report is rejected rather than silently
// corrupting the trend.
func (h *History[K]) Append(s Sample[K]) bool {
	if last, ok := h.Latest(); ok && !s.At.After(last.At) {
		return false
	}

	h.samples = append(h.samples, s)
	if len(h.samples) > h.limit {
		h.samples = h.samples[len(h.samples)-h.limit:]
	}
	return true
}

// Samples returns the window, oldest first. The slice is only valid until the
// next Append; copy it to keep it.
func (h *History[K]) Samples() []Sample[K] { return h.samples }

func (h *History[K]) Latest() (Sample[K], bool) {
	if len(h.samples) == 0 {
		var zero Sample[K]
		return zero, false
	}
	return h.samples[len(h.samples)-1], true
}

func (h *History[K]) Oldest() (Sample[K], bool) {
	if len(h.samples) == 0 {
		var zero Sample[K]
		return zero, false
	}
	return h.samples[0], true
}

func (h *History[K]) Len() int { return len(h.samples) }

func (h *History[K]) Limit() int { return h.limit }

// Span is the time between the oldest and newest sample. A classifier that
// needs a minimum period of evidence can check this rather than counting
// samples, which say nothing about how long they took to arrive.
func (h *History[K]) Span() time.Duration {
	if len(h.samples) < 2 {
		return 0
	}
	return h.samples[len(h.samples)-1].At.Sub(h.samples[0].At)
}

// Values pulls one number out of every sample, for the statistics helpers.
func (h *History[K]) Values(pick func(K) float64) []float64 {
	out := make([]float64, len(h.samples))
	for i, s := range h.samples {
		out[i] = pick(s.KPI)
	}
	return out
}

// Reset empties the window, keeping the limit.
func (h *History[K]) Reset() { h.samples = h.samples[:0] }
