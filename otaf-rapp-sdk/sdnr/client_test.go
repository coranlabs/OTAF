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

func TestEndpointIsRequired(t *testing.T) {
	if _, err := New(Config{}, quietLogger()); err == nil {
		t.Fatal("expected an error when no endpoint is configured")
	}
}

// The path has to address the node's mounted datastore exactly, or the
// controller answers 404 and the reason is far from obvious.
func TestMountPathAddressesTheNode(t *testing.T) {
	c, _ := newTestClient(t, http.StatusOK, "")

	path := c.MountPath(
		"_3gpp-common-managed-element:ManagedElement=me1",
		"/_3gpp-nr-nrm-gnbcucpfunction:GNBCUCPFunction=cucp1",
	)

	for _, want := range []string{
		"/rests/data/network-topology:network-topology",
		"/topology=topology-netconf/node=gnb-1/yang-ext:mount",
		"/_3gpp-common-managed-element:ManagedElement=me1",
		"/_3gpp-nr-nrm-gnbcucpfunction:GNBCUCPFunction=cucp1",
	} {
		if !strings.Contains(path, want) {
			t.Errorf("mount path is missing %q\n  got %s", want, path)
		}
	}
	if strings.Contains(path, "//_3gpp") {
		t.Errorf("segments should join with single slashes, got %s", path)
	}
}

func TestNodeIDIsEscaped(t *testing.T) {
	c, err := New(Config{Endpoint: "http://ctrl", NodeID: "node/with space"}, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(c.MountPath(), "with space") {
		t.Errorf("node id should be escaped in the path, got %s", c.MountPath())
	}
}
