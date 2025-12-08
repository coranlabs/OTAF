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

// Package httpsrc accepts data pushed to the rApp over HTTP, which is how a
// DME job delivers to an rApp acting as an information consumer.
package httpsrc

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/ingest"
)

const (
	defaultMaxBytes = 8 << 20
	defaultRelay    = 256
)

type Source struct {
	path     string
	maxBytes int64
	logger   *logrus.Logger
	relay    chan ingest.Message
}

type Option func(*Source)

func WithMaxBytes(n int64) Option {
	return func(s *Source) {
		if n > 0 {
			s.maxBytes = n
		}
	}
}

func WithLogger(l *logrus.Logger) Option { return func(s *Source) { s.logger = l } }

func WithBuffer(n int) Option {
	return func(s *Source) {
		if n > 0 {
			s.relay = make(chan ingest.Message, n)
		}
	}
}

func New(path string, opts ...Option) *Source {
	s := &Source{
		path:     path,
		maxBytes: defaultMaxBytes,
		relay:    make(chan ingest.Message, defaultRelay),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *Source) Name() string { return "http" + s.path }

func (s *Source) Path() string { return s.path }

// Register wires the receiving endpoint. The app calls this for every source
// that implements it, before the server starts listening.
func (s *Source) Register(r *mux.Router) {
	r.HandleFunc(s.path, s.serve).Methods(http.MethodPost)
}

// Open marks the receiving endpoint as reachable without a session, since the
// pushing side is a platform component, not an operator.
func (s *Source) Open() []string { return []string{s.path} }

func (s *Source) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.maxBytes))
	if err != nil {
		http.Error(w, `{"error":"unreadable body"}`, http.StatusBadRequest)
		return
	}

	msg := ingest.Message{
		Source:   s.Name(),
		Key:      r.URL.Query().Get("job"),
		Payload:  body,
		Received: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	select {
	case s.relay <- msg:
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	default:
		// Telling the sender to retry beats acknowledging data we discarded.
		if s.logger != nil {
			s.logger.WithField("path", s.path).Warn("receive buffer full, rejecting delivery")
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"busy, retry"}`))
	}
}
