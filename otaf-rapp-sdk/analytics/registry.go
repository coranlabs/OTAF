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
