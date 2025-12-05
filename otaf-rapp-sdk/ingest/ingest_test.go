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

package ingest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu   sync.Mutex
	seen []Message
	fail bool
}

func (r *recorder) Handle(_ context.Context, m Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errors.New("refused")
	}
	r.seen = append(r.seen, m)
	return nil
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.seen)
}

type burst struct {
	messages []Message
}

func (b *burst) Name() string { return "burst" }

func (b *burst) Run(ctx context.Context, out chan<- Message) error {
	for _, m := range b.messages {
		select {
		case out <- m:
		case <-ctx.Done():
			return nil
		}
	}
	return nil
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before the deadline")
}

func TestPipelineDeliversFromSource(t *testing.T) {
	h := &recorder{}
	p := NewPipeline(h, WithBuffer(8))
	p.AddSource(&burst{messages: []Message{
		{Source: "burst", Payload: []byte("one")},
		{Source: "burst", Payload: []byte("two")},
	}})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = p.Run(ctx)
		close(done)
	}()

	waitFor(t, func() bool { return h.count() == 2 })
	cancel()
	<-done

	stats := p.Stats()
	if stats.Processed != 2 {
		t.Errorf("processed = %d, want 2", stats.Processed)
	}
	// Messages from a source must be counted the same as submitted ones, or
	// the accepted total silently excludes the normal path.
	if stats.Accepted != 2 {
		t.Errorf("accepted = %d, want 2", stats.Accepted)
	}
}

func TestSourceMessagesHonourTheOverflowPolicy(t *testing.T) {
	h := &recorder{}
	p := NewPipeline(h, WithBuffer(1), WithOverflow(OverflowDrop))

	many := make([]Message, 50)
	for i := range many {
		many[i] = Message{Source: "burst", Payload: []byte("x")}
	}
	p.AddSource(&burst{messages: many})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = p.Run(ctx)
		close(done)
	}()

	waitFor(t, func() bool {
		s := p.Stats()
		return s.Accepted+s.Dropped == uint64(len(many))
	})
	cancel()
	<-done

	if p.Stats().Accepted == 0 {
		t.Error("some messages should have been accepted")
	}
}
