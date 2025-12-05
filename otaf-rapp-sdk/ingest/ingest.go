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

// Package ingest moves data from the interfaces an rApp subscribes to into the
// rApp's own logic. Sources produce messages, one Handler consumes them.
package ingest

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/log"
)

type Message struct {
	Source   string
	Key      string
	Payload  []byte
	Received time.Time
}

// Source feeds a pipeline until its context is cancelled. Returning nil means
// the source finished cleanly; any error is logged and ends that source only.
type Source interface {
	Name() string
	Run(ctx context.Context, out chan<- Message) error
}

// Handler is where an rApp's own logic lives.
type Handler interface {
	Handle(ctx context.Context, m Message) error
}

type HandlerFunc func(ctx context.Context, m Message) error

func (f HandlerFunc) Handle(ctx context.Context, m Message) error { return f(ctx, m) }

// Overflow decides what a full pipeline does with a new message.
type Overflow int

const (
	// OverflowBlock applies backpressure to the source. Correct whenever
	// losing a report would corrupt the rApp's view of the network.
	OverflowBlock Overflow = iota
	// OverflowDrop keeps the newest data flowing and counts the losses.
	OverflowDrop
)

// Observer is told about each pass through the handler. It exists so timing
// can be recorded without this package knowing anything about metrics.
type Observer interface {
	Handled(source string, d time.Duration, err error)
}

type ObserverFunc func(source string, d time.Duration, err error)

func (f ObserverFunc) Handled(source string, d time.Duration, err error) { f(source, d, err) }

func WithObserver(o Observer) Option { return func(p *Pipeline) { p.observer = o } }

// SetObserver attaches an observer after construction, which is how the app
// wires its metrics into a pipeline the rApp built itself.
func (p *Pipeline) SetObserver(o Observer) { p.observer = o }

type Stats struct {
	Queued    int    `json:"queued"`
	Capacity  int    `json:"capacity"`
	Accepted  uint64 `json:"accepted"`
	Dropped   uint64 `json:"dropped"`
	Failed    uint64 `json:"failed"`
	Processed uint64 `json:"processed"`
}

type Pipeline struct {
	handler  Handler
	logger   *logrus.Logger
	observer Observer
	sources  []Source
	ch       chan Message
	workers  int
	overflow Overflow

	accepted  atomic.Uint64
	dropped   atomic.Uint64
	failed    atomic.Uint64
	processed atomic.Uint64
}

type Option func(*Pipeline)

func WithBuffer(n int) Option {
	return func(p *Pipeline) {
		if n > 0 {
			p.ch = make(chan Message, n)
		}
	}
}

// WithWorkers runs n handlers concurrently. Leave it at 1 when the handler
// keeps ordered state, which is the usual case for KPI-driven logic.
func WithWorkers(n int) Option {
	return func(p *Pipeline) {
		if n > 0 {
			p.workers = n
		}
	}
}

func WithOverflow(o Overflow) Option { return func(p *Pipeline) { p.overflow = o } }

func WithLogger(l *logrus.Logger) Option { return func(p *Pipeline) { p.logger = l } }

func NewPipeline(h Handler, opts ...Option) *Pipeline {
	p := &Pipeline{
		handler: h,
		ch:      make(chan Message, 256),
		workers: 1,
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

func (p *Pipeline) AddSource(s Source) { p.sources = append(p.sources, s) }

// Sources lets the app wire up any source that also serves HTTP endpoints.
func (p *Pipeline) Sources() []Source { return p.sources }

// Submit hands a message to the pipeline. It reports false only when the
// message was dropped under OverflowDrop or the pipeline is shutting down.
func (p *Pipeline) Submit(ctx context.Context, m Message) bool {
	if m.Received.IsZero() {
		m.Received = time.Now()
	}
	switch p.overflow {
	case OverflowDrop:
		select {
		case p.ch <- m:
			p.accepted.Add(1)
			return true
		default:
			p.dropped.Add(1)
			if p.logger != nil {
				p.logger.WithField("source", m.Source).Warn("ingest queue full, message dropped")
			}
			return false
		}
	default:
		select {
		case p.ch <- m:
			p.accepted.Add(1)
			return true
		case <-ctx.Done():
			return false
		}
	}
}

func (p *Pipeline) Stats() Stats {
	return Stats{
		Queued:    len(p.ch),
		Capacity:  cap(p.ch),
		Accepted:  p.accepted.Load(),
		Dropped:   p.dropped.Load(),
		Failed:    p.failed.Load(),
		Processed: p.processed.Load(),
	}
}

// Run starts every source and the worker pool, and returns once ctx is done
// and the queue has drained.
func (p *Pipeline) Run(ctx context.Context) error {
	var sources sync.WaitGroup
	for _, s := range p.sources {
		sources.Add(1)
		go func(s Source) {
			defer sources.Done()

			// Everything a source produces goes through Submit, so counting
			// and the overflow policy work the same however a message arrived.
			feed := make(chan Message)
			var pump sync.WaitGroup
			pump.Add(1)
			go func() {
				defer pump.Done()
				for m := range feed {
					p.Submit(ctx, m)
				}
			}()

			err := s.Run(ctx, feed)
			close(feed)
			pump.Wait()

			if err != nil && ctx.Err() == nil && p.logger != nil {
				p.logger.WithError(err).WithField("source", s.Name()).Error("ingest source stopped")
			}
		}(s)
	}

	var workers sync.WaitGroup
	for i := 0; i < p.workers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			p.consume(ctx)
		}()
	}

	sources.Wait()
	<-ctx.Done()
	workers.Wait()
	return nil
}

func (p *Pipeline) consume(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			p.drain()
			return
		case m := <-p.ch:
			p.dispatch(ctx, m)
		}
	}
}

func (p *Pipeline) drain() {
	for {
		select {
		case m := <-p.ch:
			p.dispatch(context.Background(), m)
		default:
			return
		}
	}
}

func (p *Pipeline) dispatch(ctx context.Context, m Message) {
	if p.handler == nil {
		return
	}

	started := time.Now()
	err := p.handler.Handle(ctx, m)
	if p.observer != nil {
		p.observer.Handled(m.Source, time.Since(started), err)
	}

	if err != nil {
		p.failed.Add(1)
		// Logged at the level its severity calls for, carrying the
		// classification, so failures are greppable by kind rather than only
		// by the text of a message.
		log.FailureWith(p.logger, err, "handler failed", map[string]any{"source": m.Source})
		return
	}
	p.processed.Add(1)
}
