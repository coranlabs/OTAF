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

package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

func quietLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(nopWriter{})
	return l
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

func newTestGuard(t *testing.T, opts ...Option) *Guard {
	t.Helper()
	hash, err := Hash("correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	g, err := NewGuard("operator:"+hash, quietLogger(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	if g == nil {
		t.Fatal("guard should not be nil for a valid account spec")
	}
	return g
}

func protectedServer(g *Guard) http.Handler {
	r := mux.NewRouter()
	g.Register(r)
	r.HandleFunc("/private", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(UserOf(req)))
	})
	return g.Wrap(r)
}

func login(t *testing.T, h http.Handler, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	req.RemoteAddr = "10.0.0.1:5000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestEmptySpecDisablesAuthentication(t *testing.T) {
	g, err := NewGuard("  ", quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if g != nil {
		t.Fatal("an empty account spec should leave the rApp unauthenticated")
	}
}
