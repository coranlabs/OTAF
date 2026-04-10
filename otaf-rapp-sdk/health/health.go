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

// Package health tracks the reachability of the platform services an rApp
// depends on and exposes the result through the rApp's status endpoint.
package health

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

type CheckerFunc struct {
	Label string
	Fn    func(ctx context.Context) error
}

func (c CheckerFunc) Name() string                    { return c.Label }
func (c CheckerFunc) Check(ctx context.Context) error { return c.Fn(ctx) }
func Func(name string, fn func(context.Context) error) Checker {
	return CheckerFunc{Label: name, Fn: fn}
}

type Status struct {
	Healthy bool      `json:"healthy"`
	Error   string    `json:"error,omitempty"`
	Checked time.Time `json:"checked_at"`
}

type Registry struct {
	logger *logrus.Logger

	mu       sync.RWMutex
	checkers []Checker
	status   map[string]Status
}

func NewRegistry(logger *logrus.Logger) *Registry {
	return &Registry{logger: logger, status: map[string]Status{}}
}

func (r *Registry) Add(c Checker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkers = append(r.checkers, c)
	r.status[c.Name()] = Status{}
}

func (r *Registry) Snapshot() map[string]Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]Status, len(r.status))
	for k, v := range r.status {
		out[k] = v
	}
	return out
}

// Healthy reports whether every registered dependency passed its last check.
// An rApp with no dependencies is healthy.
func (r *Registry) Healthy() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.status {
		if !s.Healthy {
			return false
		}
	}
	return true
}

// Monitor probes every dependency on an interval until ctx ends. Only
// transitions are logged at warn/info; steady state stays at debug so a long
// outage does not flood the platform's log pipeline.
func (r *Registry) Monitor(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	r.probeAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.probeAll(ctx)
		}
	}
}

func (r *Registry) probeAll(ctx context.Context) {
	r.mu.RLock()
	checkers := make([]Checker, len(r.checkers))
	copy(checkers, r.checkers)
	r.mu.RUnlock()

	for _, c := range checkers {
		r.probe(ctx, c)
	}
}

func (r *Registry) probe(ctx context.Context, c Checker) {
	err := c.Check(ctx)
	now := time.Now()

	r.mu.Lock()
	prev := r.status[c.Name()]
	next := Status{Healthy: err == nil, Checked: now}
	if err != nil {
		next.Error = err.Error()
	}
	r.status[c.Name()] = next
	r.mu.Unlock()

	if r.logger == nil {
		return
	}
	// Add seeds the map so an unprobed dependency counts as not ready, which
	// means a zero timestamp, not a missing key, marks the first probe.
	first := prev.Checked.IsZero()

	entry := r.logger.WithField("dependency", c.Name())
	switch {
	case first || prev.Healthy != next.Healthy:
		if next.Healthy {
			entry.Info("dependency reachable")
		} else {
			entry.WithError(err).Warn("dependency unreachable")
		}
	case !next.Healthy:
		entry.WithError(err).Debug("dependency still unreachable")
	}
}
