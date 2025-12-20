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

func message(payload string) ingest.Message {
	return ingest.Message{Source: "test", Payload: []byte(payload), Received: time.Now()}
}

// The point of the queue: a handler that failed because something it depends
// on was briefly away does not lose the data.
func TestFailedMessageIsParkedAndRecovered(t *testing.T) {
	q, c := newQueue(t, Config{})
	handler := &flaky{failing: true}
	wrapped := q.Wrap(handler)

	ctx := context.Background()
	if err := wrapped.Handle(ctx, message("one")); err == nil {
		t.Fatal("the failure should still surface to the pipeline")
	}
	if q.Len() != 1 {
		t.Fatalf("parked = %d, want 1", q.Len())
	}

	handler.recover()
	c.advance(time.Hour)

	recovered, failed := q.RetryDue(ctx)
	if recovered != 1 || failed != 0 {
		t.Errorf("recovered/failed = %d/%d, want 1/0", recovered, failed)
	}
	if q.Len() != 0 {
		t.Error("a recovered message should leave the queue")
	}
	if handler.count() != 1 {
		t.Errorf("handler accepted %d messages, want 1", handler.count())
	}
}

// Backoff must actually hold a message back, or the queue becomes a hot loop
// against a service that is still down.
func TestBackoffHoldsAMessageBack(t *testing.T) {
	q, c := newQueue(t, Config{
		Backoff: retry.Policy{Attempts: 5, Initial: time.Minute, Max: time.Hour, Multiplier: 2},
	})
	handler := &flaky{failing: true}
	wrapped := q.Wrap(handler)

	_ = wrapped.Handle(context.Background(), message("one"))
	handler.recover()

	if recovered, _ := q.RetryDue(context.Background()); recovered != 0 {
		t.Error("a message should not be retried before its backoff has elapsed")
	}

	c.advance(2 * time.Minute)
	if recovered, _ := q.RetryDue(context.Background()); recovered != 1 {
		t.Error("the message should be retried once the backoff has elapsed")
	}
}

// An operator who knows the obstacle has gone should not have to wait.
func TestRetryAllIgnoresBackoff(t *testing.T) {
	q, _ := newQueue(t, Config{
		Backoff: retry.Policy{Attempts: 5, Initial: time.Hour, Max: time.Hour, Multiplier: 1},
	})
	handler := &flaky{failing: true}
	wrapped := q.Wrap(handler)

	_ = wrapped.Handle(context.Background(), message("one"))
	handler.recover()

	if recovered, _ := q.RetryAll(context.Background()); recovered != 1 {
		t.Error("RetryAll should replay regardless of the backoff")
	}
}

// A message no amount of retrying will fix must not fill the queue.
func TestPermanentFailuresAreNotParked(t *testing.T) {
	q, _ := newQueue(t, Config{})
	handler := &flaky{failing: true, err: retry.Permanent(errors.New("malformed payload"))}
	wrapped := q.Wrap(handler)

	if err := wrapped.Handle(context.Background(), message("junk")); err == nil {
		t.Fatal("the failure should still surface")
	}
	if q.Len() != 0 {
		t.Error("a permanently bad message should not be parked")
	}
	if q.Stats().Rejected != 1 {
		t.Errorf("rejected = %d, want 1", q.Stats().Rejected)
	}
}

// A classified failure needs no hand-labelling: the category already says
// whether another attempt could help.
func TestClassifiedFailuresDecideParkingByThemselves(t *testing.T) {
	cases := map[string]struct {
		err        error
		wantParked int
	}{
		"bad data is dropped":           {errs.New(errs.CategoryData, "MALFORMED", "not JSON"), 0},
		"a platform outage is parked":   {errs.New(errs.CategoryPlatform, "A1_UNAVAILABLE", "service away"), 1},
		"an unreachable node is parked": {errs.New(errs.CategoryNetwork, "O1_UNREACHABLE", "no answer"), 1},
		"misconfiguration is dropped":   {errs.New(errs.CategoryConfig, "CONFIG_MISSING", "no file"), 0},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			q, _ := newQueue(t, Config{})
			handler := &flaky{failing: true, err: tc.err}

			_ = q.Wrap(handler).Handle(context.Background(), message("one"))

			if q.Len() != tc.wantParked {
				t.Errorf("parked = %d, want %d", q.Len(), tc.wantParked)
			}
		})
	}
}

func TestGivesUpAfterMaxAttempts(t *testing.T) {
	q, c := newQueue(t, Config{
		MaxAttempts: 3,
		Backoff:     retry.Policy{Attempts: 3, Initial: time.Second, Max: time.Second, Multiplier: 1},
	})
	handler := &flaky{failing: true}
	wrapped := q.Wrap(handler)

	_ = wrapped.Handle(context.Background(), message("one"))

	for i := 0; i < 5 && q.Len() > 0; i++ {
		c.advance(time.Minute)
		q.RetryDue(context.Background())
	}

	if q.Len() != 0 {
		t.Error("a message that keeps failing should eventually be given up on")
	}
	if q.Stats().Exhausted != 1 {
		t.Errorf("exhausted = %d, want 1", q.Stats().Exhausted)
	}
}

func TestOldMessagesExpire(t *testing.T) {
	q, c := newQueue(t, Config{MaxAge: time.Hour})
	handler := &flaky{failing: true}
	wrapped := q.Wrap(handler)

	_ = wrapped.Handle(context.Background(), message("one"))
	c.advance(2 * time.Hour)

	q.RetryDue(context.Background())

	if q.Len() != 0 {
		t.Error("a message parked longer than the maximum age should be discarded")
	}
	if q.Stats().Expired != 1 {
		t.Errorf("expired = %d, want 1", q.Stats().Expired)
	}
}

// At the limit the newest failure is the one still worth recovering.
func TestOverflowDropsTheOldest(t *testing.T) {
	q, c := newQueue(t, Config{MaxEntries: 2})
	handler := &flaky{failing: true}
	wrapped := q.Wrap(handler)

	for _, payload := range []string{"first", "second", "third"} {
		c.advance(time.Second)
		_ = wrapped.Handle(context.Background(), message(payload))
	}

	if q.Len() != 2 {
		t.Fatalf("parked = %d, want it capped at 2", q.Len())
	}
	if q.Stats().Overflow != 1 {
		t.Errorf("overflow = %d, want 1", q.Stats().Overflow)
	}

	entries := q.Entries()
	if string(entries[0].Message.Payload) == "first" {
		t.Error("the oldest entry should have been dropped")
	}
}

// A restart must not lose what was parked; that is the reason for the disk.
func TestParkedMessagesSurviveARestart(t *testing.T) {
	dir := t.TempDir()

	first, _ := newQueue(t, Config{Dir: dir})
	handler := &flaky{failing: true}
	_ = first.Wrap(handler).Handle(context.Background(), message("survive me"))

	if first.Len() != 1 {
		t.Fatalf("parked = %d, want 1", first.Len())
	}

	// A fresh queue over the same directory stands in for a restarted rApp.
	second, c := newQueue(t, Config{Dir: dir})
	if second.Len() != 1 {
		t.Fatalf("after restart parked = %d, want 1", second.Len())
	}

	recovered := &flaky{}
	wrapped := second.Wrap(recovered)
	_ = wrapped
	c.advance(time.Hour)

	if got, _ := second.RetryAll(context.Background()); got != 1 {
		t.Fatal("the restored message should be replayable")
	}
	if recovered.count() != 1 {
		t.Fatal("the handler should have received the restored message")
	}
	if string(recovered.accepted[0]) != "survive me" {
		t.Errorf("payload = %q, want it intact across the restart", recovered.accepted[0])
	}
}

func TestRecoveredMessageIsRemovedFromDisk(t *testing.T) {
	dir := t.TempDir()
	q, c := newQueue(t, Config{Dir: dir})

	handler := &flaky{failing: true}
	wrapped := q.Wrap(handler)
	_ = wrapped.Handle(context.Background(), message("one"))

	if files, _ := filepath.Glob(filepath.Join(dir, "*.json")); len(files) != 1 {
		t.Fatalf("files on disk = %d, want 1", len(files))
	}

	handler.recover()
	c.advance(time.Hour)
	q.RetryDue(context.Background())

	if files, _ := filepath.Glob(filepath.Join(dir, "*.json")); len(files) != 0 {
		t.Errorf("files on disk = %d, want none once recovered", len(files))
	}
}

// Losing one unreadable file silently would be worse than keeping it.
func TestCorruptFileIsSetAsideNotDropped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	q, _ := newQueue(t, Config{Dir: dir})

	if q.Len() != 0 {
		t.Error("an unreadable file should not become an entry")
	}
	if _, err := os.Stat(filepath.Join(dir, "broken.json.corrupt")); err != nil {
		t.Error("an unreadable file should be set aside for inspection, not deleted")
	}
}

func TestMemoryOnlyWhenNoDirectoryConfigured(t *testing.T) {
	q, _ := newQueue(t, Config{})

	handler := &flaky{failing: true}
	_ = q.Wrap(handler).Handle(context.Background(), message("one"))

	if q.Len() != 1 {
		t.Error("a queue with no directory should still hold messages in memory")
	}
}

func TestRetryByID(t *testing.T) {
	q, _ := newQueue(t, Config{})
	handler := &flaky{failing: true}
	wrapped := q.Wrap(handler)

	_ = wrapped.Handle(context.Background(), message("one"))
	id := q.Entries()[0].ID

	if err := q.Retry(context.Background(), id); err == nil {
		t.Error("retrying while the handler still fails should report the failure")
	}

	handler.recover()
	if err := q.Retry(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if q.Len() != 0 {
		t.Error("a recovered message should leave the queue")
	}
	if err := q.Retry(context.Background(), "no-such-id"); err == nil {
		t.Error("retrying an unknown id should report an error")
	}
}

func TestDiscard(t *testing.T) {
	q, _ := newQueue(t, Config{})
	handler := &flaky{failing: true}
	_ = q.Wrap(handler).Handle(context.Background(), message("one"))

	id := q.Entries()[0].ID
	if !q.Discard(id) {
		t.Error("discarding a parked message should report true")
	}
	if q.Discard(id) {
		t.Error("discarding it twice should report false")
	}
	if q.Len() != 0 {
		t.Error("the message should be gone")
	}
}

func TestStartReplaysUntilContextEnds(t *testing.T) {
	// Real time here: the loop and the backoff have to agree on a clock, and
	// this is the one they use in production.
	q, err := New(Config{
		Interval: 10 * time.Millisecond,
		Backoff:  retry.Policy{Attempts: 5, Initial: time.Millisecond, Max: 5 * time.Millisecond, Multiplier: 1},
	}, quietLogger())
	if err != nil {
		t.Fatal(err)
	}

	handler := &flaky{failing: true}
	wrapped := q.Wrap(handler)
	_ = wrapped.Handle(context.Background(), message("one"))

	handler.recover()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_ = q.Start(ctx)

	if q.Len() != 0 {
		t.Error("the background loop should have replayed the parked message")
	}
}
