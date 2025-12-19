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

package dlq

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/ingest"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/retry"
)

func quietLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l
}

// flaky refuses until it is told to stop.
type flaky struct {
	mu       sync.Mutex
	failing  bool
	err      error
	accepted [][]byte
}

func (f *flaky) Handle(_ context.Context, m ingest.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failing {
		if f.err != nil {
			return f.err
		}
		return errors.New("dependency unavailable")
	}
	f.accepted = append(f.accepted, m.Payload)
	return nil
}

func (f *flaky) recover() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failing = false
}

func (f *flaky) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.accepted)
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

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newQueue(t *testing.T, cfg Config) (*Queue, *clock) {
	t.Helper()
	c := newClock()
	q, err := New(cfg, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	return q.WithClock(c.Now), c
}
