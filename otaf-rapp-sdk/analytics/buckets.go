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
	"time"
)

// Bucket is one slot of a rolling window.
type Bucket struct {
	Start  time.Time          `json:"start"`
	Values map[string]float64 `json:"values"`
}

// Buckets counts things into fixed time slots and keeps a fixed number of
// them, so an rApp can answer "how much did this happen over the last day"
// without a database.
//
// Slots are aligned to the wall clock rather than to when the rApp started, so
// two rApps agree on which slot an event falls in, and a restart does not
// shift the boundaries.
type Buckets struct {
	mu     sync.Mutex
	width  time.Duration
	slots  []Bucket
	starts []time.Time
}

// NewBuckets keeps count slots of the given width. A day of two-hour slots is
// twelve of them.
func NewBuckets(width time.Duration, count int) *Buckets {
	if width <= 0 {
		width = time.Hour
	}
	if count < 1 {
		count = 1
	}
	return &Buckets{
		width:  width,
		slots:  make([]Bucket, count),
		starts: make([]time.Time, count),
	}
}

// Add accumulates a value into the slot the timestamp falls in.
func (b *Buckets) Add(at time.Time, field string, v float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	slot := b.slotFor(at)
	slot.Values[field] += v
}

// Incr adds one, for counting events.
func (b *Buckets) Incr(at time.Time, field string) { b.Add(at, field, 1) }

// Window returns the slots covering the period ending at now, oldest first.
// Slots with nothing in them are returned empty rather than omitted, so a
// chart has a point for every interval.
func (b *Buckets) Window(now time.Time) []Bucket {
	b.mu.Lock()
	defer b.mu.Unlock()

	count := len(b.slots)
	out := make([]Bucket, 0, count)

	for i := count - 1; i >= 0; i-- {
		start := now.Truncate(b.width).Add(-time.Duration(i) * b.width)
		index := b.indexOf(start)

		if b.starts[index].Equal(start) {
			out = append(out, Bucket{Start: start, Values: copyValues(b.slots[index].Values)})
			continue
		}
		out = append(out, Bucket{Start: start, Values: map[string]float64{}})
	}
	return out
}

// Total sums one field across the whole window.
func (b *Buckets) Total(now time.Time, field string) float64 {
	var sum float64
	for _, bucket := range b.Window(now) {
		sum += bucket.Values[field]
	}
	return sum
}

// slotFor returns the slot a timestamp belongs in, resetting it first if it
// still holds an older period. Callers hold the lock.
func (b *Buckets) slotFor(at time.Time) *Bucket {
	start := at.Truncate(b.width)
	index := b.indexOf(start)

	if !b.starts[index].Equal(start) {
		b.starts[index] = start
		b.slots[index] = Bucket{Start: start, Values: map[string]float64{}}
	}
	return &b.slots[index]
}

func (b *Buckets) indexOf(start time.Time) int {
	slot := start.UnixNano() / int64(b.width)
	index := int(slot % int64(len(b.slots)))
	if index < 0 {
		index += len(b.slots)
	}
	return index
}

func copyValues(in map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
