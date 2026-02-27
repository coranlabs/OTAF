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

package influx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func quietLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l
}

func TestDisabledWithoutURL(t *testing.T) {
	w, err := New(Config{}, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if w != nil {
		t.Fatal("no URL should yield a nil writer")
	}

	// A nil writer has to be safe to use, so persistence stays optional
	// without the rApp guarding every call.
	w.Point("m", nil, map[string]any{"v": 1}, time.Now())
	w.Flush(context.Background())
	if w.Bucket() != "" {
		t.Error("a nil writer should report no bucket")
	}
	if got := w.Stats(); got != (Stats{}) {
		t.Errorf("stats = %#v, want the zero value", got)
	}
}

func TestBucketAndOrgAreRequiredWithAURL(t *testing.T) {
	if _, err := New(Config{URL: "http://influx", Org: "o"}, quietLogger()); err == nil {
		t.Error("expected an error when the bucket is missing")
	}
	if _, err := New(Config{URL: "http://influx", Bucket: "b"}, quietLogger()); err == nil {
		t.Error("expected an error when the org is missing")
	}
}

func TestLineProtocolEncoding(t *testing.T) {
	at := time.Unix(0, 1700000000000000000)

	cases := map[string]struct {
		measurement string
		tags        map[string]string
		fields      map[string]any
		want        string
	}{
		"floats and ints": {
			"cell_kpis",
			map[string]string{"cell": "c1"},
			map[string]any{"load": 42.5, "ues": 7},
			`cell_kpis,cell=c1 load=42.5,ues=7i 1700000000000000000`,
		},
		"tags are sorted": {
			"m",
			map[string]string{"z": "1", "a": "2"},
			map[string]any{"v": 1.0},
			`m,a=2,z=1 v=1 1700000000000000000`,
		},
		"separators are escaped": {
			"m",
			map[string]string{"cell": "pci=1,plmn=00101"},
			map[string]any{"v": 1.0},
			`m,cell=pci\=1\,plmn\=00101 v=1 1700000000000000000`,
		},
		"strings are quoted": {
			"m",
			nil,
			map[string]any{"state": "CONGESTED"},
			`m state="CONGESTED" 1700000000000000000`,
		},
		"booleans": {
			"m", nil, map[string]any{"up": true},
			`m up=true 1700000000000000000`,
		},
		"empty tags are dropped": {
			"m",
			map[string]string{"cell": "c1", "nci": ""},
			map[string]any{"v": 1.0},
			`m,cell=c1 v=1 1700000000000000000`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := encode(tc.measurement, tc.tags, tc.fields, at); got != tc.want {
				t.Errorf("\n got %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestPointWithNoUsableFieldsIsSkipped(t *testing.T) {
	if got := encode("m", nil, map[string]any{"bad": struct{}{}}, time.Now()); got != "" {
		t.Errorf("got %q, want nothing written for an unencodable field", got)
	}
}

func TestFlushSendsQueuedPoints(t *testing.T) {
	var (
		mu   sync.Mutex
		body string
		path string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		body, path = string(raw), r.URL.RequestURI()
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	writer, err := New(Config{URL: srv.URL, Org: "coran", Bucket: "rapp", Token: "t"}, quietLogger())
	if err != nil {
		t.Fatal(err)
	}

	writer.Point("cell_kpis", map[string]string{"cell": "c1"}, map[string]any{"load": 10.0}, time.Now())
	writer.Point("cell_kpis", map[string]string{"cell": "c2"}, map[string]any{"load": 20.0}, time.Now())
	writer.Flush(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if got := strings.Count(body, "\n") + 1; got != 2 {
		t.Errorf("wrote %d lines, want 2 batched together", got)
	}
	if !strings.Contains(path, "bucket=rapp") || !strings.Contains(path, "org=coran") {
		t.Errorf("write went to %q, want it to name the org and bucket", path)
	}
	if writer.Stats().Written != 2 {
		t.Errorf("written = %d, want 2", writer.Stats().Written)
	}
}

// Persistence must never stall the decision path that feeds it.
func TestPointsAreDroppedRatherThanBlocking(t *testing.T) {
	writer, err := New(Config{URL: "http://127.0.0.1:1", Org: "o", Bucket: "b"}, quietLogger())
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		for i := 0; i < defaultBuffer*2; i++ {
			writer.Point("m", nil, map[string]any{"v": float64(i)}, time.Now())
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("writing points blocked when the buffer filled")
	}

	if writer.Stats().Dropped == 0 {
		t.Error("overflowing points should be counted as dropped")
	}
}
