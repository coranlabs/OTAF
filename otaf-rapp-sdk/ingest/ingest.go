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
