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

package health

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func recordingLogger() (*logrus.Logger, *lines) {
	l := logrus.New()
	l.SetLevel(logrus.DebugLevel)
	out := &lines{}
	l.SetOutput(out)
	return l, out
}

type lines struct {
	mu   sync.Mutex
	text strings.Builder
}

func (l *lines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.text.Write(p)
}

func (l *lines) count(substr string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Count(l.text.String(), substr)
}

type flappy struct {
	mu  sync.Mutex
	err error
}

func (f *flappy) set(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *flappy) check(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}

func TestNoDependenciesIsHealthy(t *testing.T) {
	r := NewRegistry(nil)
	if !r.Healthy() {
		t.Error("an rApp with no dependencies has nothing that can be down")
	}
}

func TestUncheckedDependencyIsNotYetHealthy(t *testing.T) {
	r := NewRegistry(nil)
	r.Add(Func("controller", func(context.Context) error { return nil }))

	if r.Healthy() {
		t.Error("a dependency that has not been probed should not count as up")
	}
}

func TestProbeRecordsOutcome(t *testing.T) {
	r := NewRegistry(nil)
	dep := &flappy{err: errors.New("unreachable")}
	r.Add(Func("controller", dep.check))

	r.probeAll(context.Background())

	snap := r.Snapshot()["controller"]
	if snap.Healthy {
		t.Error("a failing dependency should report unhealthy")
	}
	if snap.Error != "unreachable" {
		t.Errorf("error = %q, want the checker's message", snap.Error)
	}
	if snap.Checked.IsZero() {
		t.Error("the probe time should be recorded")
	}

	dep.set(nil)
	r.probeAll(context.Background())

	if !r.Healthy() {
		t.Error("the registry should recover once the dependency answers")
	}
}

// A dependency that is down for an hour should not produce an hour of logs.
func TestOnlyTransitionsAreLoggedLoudly(t *testing.T) {
	logger, out := recordingLogger()
	r := NewRegistry(logger)

	dep := &flappy{err: errors.New("unreachable")}
	r.Add(Func("controller", dep.check))

	for i := 0; i < 5; i++ {
		r.probeAll(context.Background())
	}

	if got := out.count("dependency unreachable"); got != 1 {
		t.Errorf("logged the outage %d times, want 1", got)
	}

	dep.set(nil)
	r.probeAll(context.Background())
	r.probeAll(context.Background())

	if got := out.count("dependency reachable"); got != 1 {
		t.Errorf("logged the recovery %d times, want 1", got)
	}
}

func TestSnapshotIsACopy(t *testing.T) {
	r := NewRegistry(nil)
	r.Add(Func("a", func(context.Context) error { return nil }))
	r.probeAll(context.Background())

	snap := r.Snapshot()
	snap["a"] = Status{Healthy: false}

	if !r.Snapshot()["a"].Healthy {
		t.Error("mutating a snapshot must not affect the registry")
	}
}

func TestMonitorProbesUntilContextEnds(t *testing.T) {
	r := NewRegistry(nil)

	var probes int
	var mu sync.Mutex
	r.Add(Func("a", func(context.Context) error {
		mu.Lock()
		probes++
		mu.Unlock()
		return nil
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	r.Monitor(ctx, 20*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if probes < 2 {
		t.Errorf("probes = %d, want the monitor to check repeatedly", probes)
	}
}

func TestMonitorProbesImmediately(t *testing.T) {
	r := NewRegistry(nil)
	probed := make(chan struct{}, 1)
	r.Add(Func("a", func(context.Context) error {
		select {
		case probed <- struct{}{}:
		default:
		}
		return nil
	}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go r.Monitor(ctx, time.Hour)

	select {
	case <-probed:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("the first probe should not wait for a full interval")
	}
}
