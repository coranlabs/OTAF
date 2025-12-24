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

import "sync"

type Journal[T any] struct {
	mu      sync.RWMutex
	entries []T
	keep    int
	total   uint64
}

// NewJournal keeps at most the last keep entries. A keep below one is treated
// as one.
func NewJournal[T any](keep int) *Journal[T] {
	if keep < 1 {
		keep = 1
	}
	return &Journal[T]{keep: keep, entries: make([]T, 0, keep)}
}

func (j *Journal[T]) Append(entry T) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.entries = append(j.entries, entry)
	if len(j.entries) > j.keep {
		j.entries = j.entries[len(j.entries)-j.keep:]
	}
	j.total++
}

// Entries returns a copy, oldest first.
func (j *Journal[T]) Entries() []T {
	j.mu.RLock()
	defer j.mu.RUnlock()

	out := make([]T, len(j.entries))
	copy(out, j.entries)
	return out
}

// Recent returns a copy of the last n entries, newest last.
func (j *Journal[T]) Recent(n int) []T {
	j.mu.RLock()
	defer j.mu.RUnlock()

	if n <= 0 || n > len(j.entries) {
		n = len(j.entries)
	}
	out := make([]T, n)
	copy(out, j.entries[len(j.entries)-n:])
	return out
}

// Latest returns the most recent entry.
func (j *Journal[T]) Latest() (T, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	if len(j.entries) == 0 {
		var zero T
		return zero, false
	}
	return j.entries[len(j.entries)-1], true
}

func (j *Journal[T]) Len() int {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return len(j.entries)
}

// Total counts everything ever appended, including entries since dropped, so a
// status endpoint can distinguish a quiet rApp from one whose journal has
// simply rolled over.
func (j *Journal[T]) Total() uint64 {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.total
}
