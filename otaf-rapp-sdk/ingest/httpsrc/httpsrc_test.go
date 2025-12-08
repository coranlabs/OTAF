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
