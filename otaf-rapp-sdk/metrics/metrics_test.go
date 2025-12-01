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

package metrics

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
)

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("scrape status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("content type = %q, want Prometheus text exposition", ct)
	}
	return rec.Body.String()
}

func TestExpositionCarriesBuildInfo(t *testing.T) {
	m := New("demo", "1.2.3", Snapshots{})
	body := scrape(t, m)

	if !strings.Contains(body, `rapp_build_info{`) {
		t.Fatal("build info metric is missing")
	}
	for _, want := range []string{`rapp="demo"`, `version="1.2.3"`, `sdk="`} {
		if !strings.Contains(body, want) {
			t.Errorf("build info is missing label %s", want)
		}
	}
	if !strings.Contains(body, "rapp_start_time_seconds") {
		t.Error("start time metric is missing")
	}
}

// The snapshot collector reads the rApp's own counters at scrape time, so the
// numbers cannot drift from what the rApp reports elsewhere.
func TestIngestStatsAreReadAtScrapeTime(t *testing.T) {
	stats := IngestStats{Queued: 3, Capacity: 256, Accepted: 10, Dropped: 2, Failed: 1, Processed: 7}
	m := New("demo", "1.0.0", Snapshots{
		Ingest: func() IngestStats { return stats },
	})

	body := scrape(t, m)
	for _, want := range []string{
		"rapp_ingest_queue_depth 3",
		"rapp_ingest_queue_capacity 256",
		`rapp_ingest_messages_total{outcome="accepted"} 10`,
		`rapp_ingest_messages_total{outcome="dropped"} 2`,
		`rapp_ingest_messages_total{outcome="processed"} 7`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("exposition is missing %q", want)
		}
	}

	stats.Queued = 99
	if body := scrape(t, m); !strings.Contains(body, "rapp_ingest_queue_depth 99") {
		t.Error("a later scrape should see the updated value")
	}
}

func TestDependencyGauge(t *testing.T) {
	m := New("demo", "1.0.0", Snapshots{
		Dependency: func() map[string]bool { return map[string]bool{"a1": true, "sdnr": false} },
	})

	body := scrape(t, m)
	if !strings.Contains(body, `rapp_dependency_up{dependency="a1"} 1`) {
		t.Error("a reachable dependency should report 1")
	}
	if !strings.Contains(body, `rapp_dependency_up{dependency="sdnr"} 0`) {
		t.Error("an unreachable dependency should report 0")
	}
}

func TestHandlerDurationRecordsOutcome(t *testing.T) {
	m := New("demo", "1.0.0", Snapshots{})

	m.Handled("http/data", 12*time.Millisecond, nil)
	m.Handled("http/data", 3*time.Millisecond, errors.New("bad payload"))

	body := scrape(t, m)
	if !strings.Contains(body, `rapp_handler_duration_seconds_count{outcome="ok",source="http/data"} 1`) {
		t.Error("a successful pass should be counted as ok")
	}
	if !strings.Contains(body, `rapp_handler_duration_seconds_count{outcome="error",source="http/data"} 1`) {
		t.Error("a failed pass should be counted as an error")
	}
}
