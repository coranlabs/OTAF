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

// Package dlq holds on to data an rApp could not process, and tries again.
//
// The ingest pipeline keeps messages in memory: a handler that fails because
// something it depends on was briefly away loses that data, and a restart
// loses whatever was queued. A dead-letter queue parks those messages on disk
// and replays them once the obstacle has passed.
//
// It is for failures that might pass. A message the handler can never process
// is not parked at all, so the queue does not fill with data no amount of
// retrying will fix; return retry.Permanent to say so.
package dlq

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/ingest"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/retry"
)

type Config struct {
	// Dir is where parked messages are written. Left empty, the queue keeps
	// them in memory only and a restart loses them.
	Dir string `yaml:"dir" env:"DLQ_DIR"`

	// MaxEntries bounds the queue. At the limit the oldest is dropped, on the
	// grounds that the newest failure is the one still worth recovering.
	MaxEntries int `yaml:"max_entries" env:"DLQ_MAX_ENTRIES"`

	// MaxAge discards a message that has been parked too long to be useful.
	MaxAge time.Duration `yaml:"max_age" env:"DLQ_MAX_AGE"`

	// MaxAttempts gives up on a message that keeps failing.
	MaxAttempts int `yaml:"max_attempts" env:"DLQ_MAX_ATTEMPTS"`

	// Interval is how often the queue looks for messages due a retry.
	Interval time.Duration `yaml:"interval" env:"DLQ_INTERVAL"`

	// Backoff spaces out the attempts on one message.
	Backoff retry.Policy `yaml:"-"`
}

func (c *Config) applyDefaults() {
	if c.MaxEntries <= 0 {
		c.MaxEntries = 1000
	}
	if c.MaxAge <= 0 {
		c.MaxAge = 24 * time.Hour
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 10
	}
	if c.Interval <= 0 {
		c.Interval = 30 * time.Second
	}
	if c.Backoff.Initial <= 0 {
		c.Backoff = retry.Policy{
			Attempts: c.MaxAttempts, Initial: 30 * time.Second,
			Max: 30 * time.Minute, Multiplier: 2, Jitter: 0.2,
		}
	}
}

// Entry is one parked message and why it is here.
type Entry struct {
	ID      string         `json:"id"`
	Message ingest.Message `json:"message"`

	Reason      string    `json:"reason"`
	Attempts    int       `json:"attempts"`
	FirstFailed time.Time `json:"first_failed"`
	LastFailed  time.Time `json:"last_failed"`
	NextAttempt time.Time `json:"next_attempt"`
}

// Due reports whether the entry is ready to be tried again.
func (e Entry) Due(at time.Time) bool { return !at.Before(e.NextAttempt) }

type Stats struct {
	Parked    int    `json:"parked"`
	Accepted  uint64 `json:"accepted"`
	Recovered uint64 `json:"recovered"`
	Exhausted uint64 `json:"exhausted"`
	Expired   uint64 `json:"expired"`
	Rejected  uint64 `json:"rejected"`
	Overflow  uint64 `json:"overflow"`
}

type Queue struct {
	cfg     Config
	logger  *logrus.Logger
	handler ingest.Handler
	now     func() time.Time

	mu      sync.Mutex
	entries map[string]*Entry
	stats   Stats
}

// New builds a queue. A configured directory is created if absent, and any
// messages parked by a previous run are loaded back in.
func New(cfg Config, logger *logrus.Logger) (*Queue, error) {
	cfg.applyDefaults()

	q := &Queue{
		cfg:     cfg,
		logger:  logger,
		now:     time.Now,
		entries: map[string]*Entry{},
	}

	if cfg.Dir != "" {
		if err := os.MkdirAll(cfg.Dir, 0o750); err != nil {
			return nil, fmt.Errorf("dlq: create %s: %w", cfg.Dir, err)
		}
		if err := q.load(); err != nil {
			return nil, err
		}
	}
	return q, nil
}

// WithClock replaces the source of time, for tests.
func (q *Queue) WithClock(now func() time.Time) *Queue {
	if now != nil {
		q.now = now
	}
	return q
}

func (q *Queue) Name() string { return "dlq" }

// Wrap returns a handler that parks whatever the inner one could not process.
// This is the whole integration: build the pipeline around the wrapped handler
// and failures become recoverable.
func (q *Queue) Wrap(handler ingest.Handler) ingest.Handler {
	q.mu.Lock()
	q.handler = handler
	q.mu.Unlock()

	return ingest.HandlerFunc(func(ctx context.Context, m ingest.Message) error {
		err := handler.Handle(ctx, m)
		if err == nil {
			return nil
		}
		q.Park(m, err)

		// The pipeline counts this as failed either way; parking it means the
		// data is not gone with the count.
		return err
	})
}

// Park holds a message for another attempt. A message the handler says can
// never be processed is counted and dropped rather than parked.
func (q *Queue) Park(m ingest.Message, cause error) {
	if cause != nil && !retry.Retryable(cause) {
		q.mu.Lock()
		q.stats.Rejected++
		q.mu.Unlock()

		q.logger.WithError(cause).WithField("source", m.Source).
			Warn("message cannot be processed, discarding rather than parking")
		return
	}

	now := q.now()
	entry := &Entry{
		ID:          newID(),
		Message:     m,
		Reason:      reasonOf(cause),
		Attempts:    0,
		FirstFailed: now,
		LastFailed:  now,
		NextAttempt: now.Add(q.cfg.Backoff.Backoff(1)),
	}

	q.mu.Lock()
	q.evictLocked(now)
	q.entries[entry.ID] = entry
	q.stats.Accepted++
	depth := len(q.entries)
	q.mu.Unlock()

	q.persist(entry)
	q.logger.WithFields(logrus.Fields{
		"id": entry.ID, "source": m.Source, "parked": depth,
	}).Warn("message parked for retry")
}

// Start replays due messages until ctx ends.
func (q *Queue) Start(ctx context.Context) error {
	if q == nil {
		return nil
	}

	ticker := time.NewTicker(q.cfg.Interval)
	defer ticker.Stop()

	if depth := q.Len(); depth > 0 {
		q.logger.WithField("parked", depth).Info("dead-letter queue restored from disk")
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			q.RetryDue(ctx)
		}
	}
}

// RetryDue replays every message whose backoff has elapsed.
func (q *Queue) RetryDue(ctx context.Context) (recovered, failed int) {
	return q.replay(ctx, false)
}

// RetryAll replays everything parked, ignoring the backoff. It is what an
// operator reaches for once they know the obstacle has gone.
func (q *Queue) RetryAll(ctx context.Context) (recovered, failed int) {
	return q.replay(ctx, true)
}

func (q *Queue) replay(ctx context.Context, ignoreBackoff bool) (recovered, failed int) {
	now := q.now()

	q.mu.Lock()
	handler := q.handler
	q.expireLocked(now)

	var due []Entry
	for _, entry := range q.entries {
		if ignoreBackoff || entry.Due(now) {
			due = append(due, *entry)
		}
	}
	q.mu.Unlock()

	if handler == nil || len(due) == 0 {
		return 0, 0
	}
	sort.Slice(due, func(i, j int) bool { return due[i].FirstFailed.Before(due[j].FirstFailed) })

	for _, entry := range due {
		if ctx.Err() != nil {
			break
		}

		if err := handler.Handle(ctx, entry.Message); err != nil {
			failed++
			q.recordFailure(entry.ID, err)
			continue
		}

		recovered++
		q.remove(entry.ID)

		q.mu.Lock()
		q.stats.Recovered++
		q.mu.Unlock()

		q.logger.WithFields(logrus.Fields{
			"id": entry.ID, "attempts": entry.Attempts + 1,
		}).Info("parked message recovered")
	}
	return recovered, failed
}

// Retry replays one message by id.
func (q *Queue) Retry(ctx context.Context, id string) error {
	q.mu.Lock()
	entry, known := q.entries[id]
	handler := q.handler
	var message ingest.Message
	if known {
		message = entry.Message
	}
	q.mu.Unlock()

	if !known {
		return fmt.Errorf("dlq: no parked message %s", id)
	}
	if handler == nil {
		return errors.New("dlq: no handler attached; call Wrap first")
	}

	if err := handler.Handle(ctx, message); err != nil {
		q.recordFailure(id, err)
		return err
	}

	q.remove(id)
	q.mu.Lock()
	q.stats.Recovered++
	q.mu.Unlock()
	return nil
}

func (q *Queue) recordFailure(id string, cause error) {
	now := q.now()

	q.mu.Lock()
	entry, known := q.entries[id]
	if !known {
		q.mu.Unlock()
		return
	}

	entry.Attempts++
	entry.LastFailed = now
	entry.Reason = reasonOf(cause)
	entry.NextAttempt = now.Add(q.cfg.Backoff.Backoff(entry.Attempts + 1))

	exhausted := entry.Attempts >= q.cfg.MaxAttempts
	if exhausted {
		delete(q.entries, id)
		q.stats.Exhausted++
	}
	snapshot := *entry
	q.mu.Unlock()

	if exhausted {
		q.deleteFile(id)
		q.logger.WithFields(logrus.Fields{
			"id": id, "attempts": snapshot.Attempts, "reason": snapshot.Reason,
		}).Error("giving up on parked message")
		return
	}
	q.persist(&snapshot)
}

// Discard forgets one parked message.
func (q *Queue) Discard(id string) bool {
	q.mu.Lock()
	_, known := q.entries[id]
	delete(q.entries, id)
	q.mu.Unlock()

	if known {
		q.deleteFile(id)
	}
	return known
}

func (q *Queue) remove(id string) {
	q.mu.Lock()
	delete(q.entries, id)
	q.mu.Unlock()
	q.deleteFile(id)
}

// Entries copies what is parked, oldest failure first.
func (q *Queue) Entries() []Entry {
	q.mu.Lock()
	defer q.mu.Unlock()

	out := make([]Entry, 0, len(q.entries))
	for _, entry := range q.entries {
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FirstFailed.Before(out[j].FirstFailed) })
	return out
}

func (q *Queue) Len() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries)
}

func (q *Queue) Stats() Stats {
	if q == nil {
		return Stats{}
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	out := q.stats
	out.Parked = len(q.entries)
	return out
}

// expireLocked drops messages too old to be worth replaying.
func (q *Queue) expireLocked(now time.Time) {
	for id, entry := range q.entries {
		if now.Sub(entry.FirstFailed) > q.cfg.MaxAge {
			delete(q.entries, id)
			q.stats.Expired++
			q.deleteFile(id)
		}
	}
}

// evictLocked makes room by dropping the oldest, on the grounds that the
// newest failure is the one still worth recovering.
func (q *Queue) evictLocked(now time.Time) {
	q.expireLocked(now)

	for len(q.entries) >= q.cfg.MaxEntries {
		var oldestID string
		var oldest time.Time
		for id, entry := range q.entries {
			if oldestID == "" || entry.FirstFailed.Before(oldest) {
				oldestID, oldest = id, entry.FirstFailed
			}
		}
		if oldestID == "" {
			return
		}
		delete(q.entries, oldestID)
		q.stats.Overflow++
		q.deleteFile(oldestID)
	}
}

func (q *Queue) persist(entry *Entry) {
	if q.cfg.Dir == "" {
		return
	}
	body, err := json.Marshal(entry)
	if err != nil {
		q.logger.WithError(err).WithField("id", entry.ID).Warn("could not encode parked message")
		return
	}
	if err := os.WriteFile(q.pathFor(entry.ID), body, 0o600); err != nil {
		q.logger.WithError(err).WithField("id", entry.ID).Warn("could not write parked message")
	}
}

func (q *Queue) deleteFile(id string) {
	if q.cfg.Dir == "" {
		return
	}
	if err := os.Remove(q.pathFor(id)); err != nil && !os.IsNotExist(err) {
		q.logger.WithError(err).WithField("id", id).Warn("could not remove parked message")
	}
}

func (q *Queue) pathFor(id string) string { return filepath.Join(q.cfg.Dir, id+".json") }

// load reads back what a previous run parked. A file that will not decode is
// moved aside rather than dropped, so nothing is lost silently.
func (q *Queue) load() error {
	files, err := os.ReadDir(q.cfg.Dir)
	if err != nil {
		return fmt.Errorf("dlq: read %s: %w", q.cfg.Dir, err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		path := filepath.Join(q.cfg.Dir, file.Name())

		body, err := os.ReadFile(path)
		if err != nil {
			q.logger.WithError(err).WithField("file", file.Name()).Warn("could not read parked message")
			continue
		}

		var entry Entry
		if err := json.Unmarshal(body, &entry); err != nil || entry.ID == "" {
			q.logger.WithField("file", file.Name()).Warn("parked message is unreadable, setting it aside")
			_ = os.Rename(path, path+".corrupt")
			continue
		}
		q.entries[entry.ID] = &entry
	}
	return nil
}

func reasonOf(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
