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
