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

type Stats struct {
	Parked    int    `json:"parked"`
	Accepted  uint64 `json:"accepted"`
	Recovered uint64 `json:"recovered"`
	Exhausted uint64 `json:"exhausted"`
	Expired   uint64 `json:"expired"`
	Rejected  uint64 `json:"rejected"`
	Overflow  uint64 `json:"overflow"`
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
