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
