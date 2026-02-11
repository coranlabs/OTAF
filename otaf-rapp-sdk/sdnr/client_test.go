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

package sdnr

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func quietLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l
}

type capture struct {
	method string
	path   string
	body   string
	auth   bool
	accept string
}

func newTestClient(t *testing.T, status int, response string) (*Client, *capture) {
	t.Helper()

	got := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_, _, got.auth = r.BasicAuth()
		got.method, got.path, got.body = r.Method, r.URL.Path, string(raw)
		got.accept = r.Header.Get("Accept")

		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)

	c, err := New(Config{
		Endpoint: srv.URL,
		Username: "admin",
		Password: "secret",
		NodeID:   "gnb-1",
	}, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	return c, got
}
