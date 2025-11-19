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

// Package retry re-attempts work that failed for a reason that might pass.
// Platform services restart, roll and briefly disappear; an rApp that treats
// every blip as fatal spends its life restarting.
package retry

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
)

type Policy struct {
	// Attempts counts the first try, so 1 means no retry at all.
	Attempts   int
	Initial    time.Duration
	Max        time.Duration
	Multiplier float64

	// Jitter spreads delays by up to this fraction either way, so a fleet of
	// rApps recovering from the same outage does not retry in lockstep.
	Jitter float64
}

// Default suits a call to a platform service over the cluster network.
func Default() Policy {
	return Policy{Attempts: 4, Initial: 200 * time.Millisecond, Max: 5 * time.Second, Multiplier: 2, Jitter: 0.3}
}

// Patient suits something known to be slow to come back, such as a service
// still starting up.
func Patient() Policy {
	return Policy{Attempts: 6, Initial: time.Second, Max: 30 * time.Second, Multiplier: 2, Jitter: 0.3}
}

// None disables retrying, which is what a non-idempotent call wants.
func None() Policy { return Policy{Attempts: 1} }

func (p Policy) normalise() Policy {
	if p.Attempts < 1 {
		p.Attempts = 1
	}
	if p.Initial <= 0 {
		p.Initial = 100 * time.Millisecond
	}
	if p.Multiplier < 1 {
		p.Multiplier = 2
	}
	if p.Max <= 0 {
		p.Max = 30 * time.Second
	}
	if p.Jitter < 0 {
		p.Jitter = 0
	}
	if p.Jitter > 1 {
		p.Jitter = 1
	}
	return p
}

type permanentError struct{ err error }
