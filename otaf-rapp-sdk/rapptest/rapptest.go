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

// Package rapptest exercises an rApp's own logic without a cluster. The fakes
// are real clients pointed at stand-in servers, so the code under test takes
// exactly the path it takes in production.
package rapptest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/ingest"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/r1"
)

func Logger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(discard{})
	l.SetLevel(logrus.PanicLevel)
	return l
}

// VerboseLogger prints, for when a test is being debugged.
func VerboseLogger() *logrus.Logger {
	l := logrus.New()
	l.SetLevel(logrus.DebugLevel)
	return l
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// Message builds an ingest message the way a source would. A []byte payload is
// passed through; anything else is marshalled to JSON.
func Message(t testing.TB, source string, payload any) ingest.Message {
	t.Helper()
	return ingest.Message{
		Source:   source,
		Payload:  encode(t, payload),
		Received: time.Now(),
	}
}

func encode(t testing.TB, payload any) []byte {
	t.Helper()
	switch v := payload.(type) {
	case nil:
		return nil
	case []byte:
		return v
	case string:
		return []byte(v)
	default:
		body, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("rapptest: could not encode payload: %v", err)
		}
		return body
	}
}

// Harness drives one handler the way the pipeline does, without goroutines or
// timing, so assertions run immediately after the call returns.
type Harness struct {
	t       testing.TB
	handler ingest.Handler
	ctx     context.Context
}

func NewHarness(t testing.TB, handler ingest.Handler) *Harness {
	t.Helper()
	return &Harness{t: t, handler: handler, ctx: context.Background()}
}

func (h *Harness) WithContext(ctx context.Context) *Harness {
	h.ctx = ctx
	return h
}

// Send hands one message to the handler and fails the test if it errors.
func (h *Harness) Send(source string, payload any) {
	h.t.Helper()
	if err := h.handler.Handle(h.ctx, Message(h.t, source, payload)); err != nil {
		h.t.Fatalf("rapptest: handler rejected the message: %v", err)
	}
}

// SendExpectingError hands over a message that should be refused, and returns
// the refusal for inspection.
func (h *Harness) SendExpectingError(source string, payload any) error {
	h.t.Helper()
	err := h.handler.Handle(h.ctx, Message(h.t, source, payload))
	if err == nil {
		h.t.Fatal("rapptest: expected the handler to reject this message")
	}
	return err
}

// SendAll replays a sequence, which is how logic that depends on history is
// driven to a verdict.
func (h *Harness) SendAll(source string, payloads ...any) {
	h.t.Helper()
	for _, p := range payloads {
		h.Send(source, p)
	}
}

// Snapshot calls the rApp's R1 snapshot function and decodes the result into
// dst. A snapshot that produced nothing fails the test.
func (h *Harness) Snapshot(snapshot r1.Snapshot, job r1.Job, dst any) {
	h.t.Helper()

	if job.ID == "" {
		job.ID = "test-job"
	}
	body, err := snapshot(h.ctx, job)
	if err != nil {
		h.t.Fatalf("rapptest: snapshot failed: %v", err)
	}
	if len(body) == 0 {
		h.t.Fatal("rapptest: snapshot produced nothing to deliver")
	}
	if dst == nil {
		return
	}
	if err := json.Unmarshal(body, dst); err != nil {
		h.t.Fatalf("rapptest: snapshot is not valid JSON: %v", err)
	}
}
