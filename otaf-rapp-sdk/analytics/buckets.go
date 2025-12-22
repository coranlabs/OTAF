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
