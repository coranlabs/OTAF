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

package httpsrc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/ingest"
)

func serve(s *Source) http.Handler {
	r := mux.NewRouter()
	s.Register(r)
	return r
}

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rec
}

func TestAcceptedDeliveryReachesThePipeline(t *testing.T) {
	s := New("/data")
	rec := post(t, serve(s), "/data", `{"id":"c1"}`)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}

	out := make(chan ingest.Message, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = s.Run(ctx, out) }()
	defer cancel()

	select {
	case m := <-out:
		if string(m.Payload) != `{"id":"c1"}` {
			t.Errorf("payload = %s, want what was posted", m.Payload)
		}
		if m.Source != "http/data" {
			t.Errorf("source = %q, want http/data", m.Source)
		}
		if m.Received.IsZero() {
			t.Error("arrival time should be stamped")
		}
	case <-time.After(time.Second):
		t.Fatal("the message never reached the pipeline")
	}
}

// Acknowledging data that was then discarded is worse than asking the sender
// to try again, because the sender has no way to know it was lost.
func TestFullBufferIsRefusedRatherThanAcknowledged(t *testing.T) {
	s := New("/data", WithBuffer(1))
	h := serve(s)

	if rec := post(t, h, "/data", `{"n":1}`); rec.Code != http.StatusAccepted {
		t.Fatalf("first delivery status = %d, want 202", rec.Code)
	}

	rec := post(t, h, "/data", `{"n":2}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 once the buffer is full", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "retry") {
		t.Errorf("body = %q, want it to tell the sender to retry", rec.Body.String())
	}
}

func TestOversizedBodyIsRejected(t *testing.T) {
	s := New("/data", WithMaxBytes(16))
	rec := post(t, serve(s), "/data", strings.Repeat("x", 128))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a body over the limit", rec.Code)
	}
}

// The endpoint is reached by a platform component, not an operator, so it must
// not sit behind the operator guard.
func TestPathIsDeclaredOpen(t *testing.T) {
	s := New("/data")

	open := s.Open()
	if len(open) != 1 || open[0] != "/data" {
		t.Errorf("open paths = %v, want the receiving path", open)
	}
	if s.Path() != "/data" {
		t.Errorf("path = %q, want /data", s.Path())
	}
}

func TestJobQueryBecomesTheMessageKey(t *testing.T) {
	s := New("/data")
	post(t, serve(s), "/data?job=job-7", `{}`)

	out := make(chan ingest.Message, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = s.Run(ctx, out) }()
	defer cancel()

	select {
	case m := <-out:
		if m.Key != "job-7" {
			t.Errorf("key = %q, want the job that delivered it", m.Key)
		}
	case <-time.After(time.Second):
		t.Fatal("the message never arrived")
	}
}
