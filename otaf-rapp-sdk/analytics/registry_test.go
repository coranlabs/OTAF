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
	"testing"
	"time"
)

type kpi struct {
	Value float64
}

// A deliberately naive classifier: the SDK ships none, so tests bring their
// own, exactly as an rApp does.
func threshold(limit float64) Classifier[kpi] {
	return ClassifierFunc[kpi]{
		Label: "threshold",
		Fn: func(samples []Sample[kpi]) Verdict {
			latest := samples[len(samples)-1].KPI.Value
			state := State("LOW")
			if latest > limit {
				state = State("HIGH")
			}
			return Verdict{State: state, Score: latest, Signals: map[string]float64{"value": latest}}
		},
	}
}

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *clock {
	return &clock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}
