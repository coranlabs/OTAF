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

package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/config"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/health"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/ingest"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/ingest/httpsrc"
)

func quietLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(discard{})
	return l
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func testConfig(port string) config.Rapp {
	return config.Rapp{Name: "demo", Version: "0.1.0", HTTPPort: port, LogLevel: "panic", LogFormat: "text"}
}

// run starts the rApp and returns a base URL plus a stop function.
func run(t *testing.T, port string, opts ...Option) (string, func()) {
	t.Helper()

	a, err := New(testConfig(port), append([]Option{WithLogger(quietLogger())}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = a.Run(ctx)
		close(done)
	}()

	base := "http://127.0.0.1:" + port
	waitReachable(t, base+"/health")

	return base, func() {
		cancel()
		<-done
	}
}

func waitReachable(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never became reachable", url)
}

func getJSON(t *testing.T, url string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	_ = json.Unmarshal(body, &parsed)
	return resp.StatusCode, parsed
}

func TestPlatformEndpointsAreServed(t *testing.T) {
	base, stop := run(t, "18191")
	defer stop()

	for _, path := range []string{"/health", "/status", "/metrics", "/ready"} {
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s status = %d, want 200", path, resp.StatusCode)
		}
	}
}

// The monitoring stack scrapes this endpoint directly, so it has to be
// Prometheus exposition rather than something that merely looks like metrics.
func TestMetricsEndpointIsScrapable(t *testing.T) {
	handler := ingest.HandlerFunc(func(context.Context, ingest.Message) error { return nil })
	pipeline := ingest.NewPipeline(handler, ingest.WithBuffer(8))
	pipeline.AddSource(httpsrc.New("/data"))

	base, stop := run(t, "18195", WithPipeline(pipeline))
	defer stop()

	resp, err := http.Post(base+"/data", "application/json", strings.NewReader(`{"id":"a"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	var body string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		body = scrapeText(t, base+"/metrics")
		if strings.Contains(body, `rapp_handler_duration_seconds_count{outcome="ok"`) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	for _, want := range []string{
		"# HELP rapp_build_info",
		"# TYPE rapp_build_info gauge",
		`rapp="demo"`,
		"rapp_ingest_queue_capacity 8",
		`rapp_ingest_messages_total{outcome="accepted"} 1`,
		`rapp_handler_duration_seconds_count{outcome="ok"`,
		"go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape is missing %q", want)
		}
	}
	if strings.HasPrefix(strings.TrimSpace(body), "{") {
		t.Error("metrics must not be JSON: no scraper can read it")
	}
}
