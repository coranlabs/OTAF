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

type Cooldown struct {
	mu     sync.Mutex
	period time.Duration
	last   map[string]time.Time
}

func NewCooldown(period time.Duration) *Cooldown {
	return &Cooldown{period: period, last: map[string]time.Time{}}
}

func (c *Cooldown) Period() time.Duration { return c.period }

// Allow reports whether the key is free to act on, without claiming it. Use it
// to decide, then Mark once the action succeeded, so a failed attempt does not
// block the next one.
func (c *Cooldown) Allow(key string, at time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.allowLocked(key, at)
}

// Take claims the key if it is free, in one step. Use it when the action
// cannot fail, or when a failed attempt should still count.
func (c *Cooldown) Take(key string, at time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.allowLocked(key, at) {
		return false
	}
	c.last[key] = at
	return true
}

// Mark records that the key was acted on.
func (c *Cooldown) Mark(key string, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last[key] = at
}
