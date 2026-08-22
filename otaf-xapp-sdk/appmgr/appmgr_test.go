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

package appmgr

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func descriptor() Descriptor {
	return Descriptor{
		Name:         "otaf-probe",
		Version:      "1.0.0",
		HTTPPort:     8098,
		RMRDataPort:  4560,
		RMRRoutePort: RoutePort,
		RxMessages:   []string{"RIC_INDICATION"},
		TxMessages:   []string{"RIC_SUB_REQ"},
	}
}

// The platform decodes config a second time, so it has to arrive as a string.
// Sent as a nested object it is rejected, which is not obvious from the error.
func TestConfigIsSentAsAString(t *testing.T) {
	var got map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	if err := New(srv.URL).Register(context.Background(), descriptor()); err != nil {
		t.Fatal(err)
	}
	raw, ok := got["config"]
	if !ok {
		t.Fatal("no config field")
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err != nil {
		t.Fatalf("config is not a JSON string: %v", err)
	}
	var inner map[string]any
	if err := json.Unmarshal([]byte(asString), &inner); err != nil {
		t.Fatalf("config string does not hold JSON: %v", err)
	}
	if inner["xapp_name"] != "otaf-probe" {
		t.Errorf("xapp_name = %v", inner["xapp_name"])
	}
}

func TestEndpointsFollowTheNamingConvention(t *testing.T) {
	d := descriptor()
	if got, want := d.RMREndpoint(), "service-ricxapp-otaf-probe-rmr.ricxapp:4560"; got != want {
		t.Errorf("rmr = %q, want %q", got, want)
	}
	if got, want := d.HTTPEndpoint(), "service-ricxapp-otaf-probe-http.ricxapp:8098"; got != want {
		t.Errorf("http = %q, want %q", got, want)
	}
}

func TestValidateRejectsBadDescriptors(t *testing.T) {
	cases := map[string]func(*Descriptor){
		"empty name":       func(d *Descriptor) { d.Name = "" },
		"upper case":       func(d *Descriptor) { d.Name = "Otaf" },
		"trailing dash":    func(d *Descriptor) { d.Name = "otaf-" },
		"short version":    func(d *Descriptor) { d.Version = "1.0" },
		"port zero":        func(d *Descriptor) { d.HTTPPort = 0 },
		"port too large":   func(d *Descriptor) { d.RMRDataPort = 70000 },
		"shared port":      func(d *Descriptor) { d.RMRDataPort = RoutePort },
		"wrong route port": func(d *Descriptor) { d.RMRRoutePort = 4571 },
	}
	for name, mangle := range cases {
		t.Run(name, func(t *testing.T) {
			d := descriptor()
			mangle(&d)
			if err := d.Validate(); err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}

func TestRegisterReportsPlatformErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		io.WriteString(w, `{"message":"descriptor rejected"}`)
	}))
	defer srv.Close()

	err := New(srv.URL).Register(context.Background(), descriptor())
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "descriptor rejected") {
		t.Errorf("error does not carry the platform's reason: %v", err)
	}
}

func TestRegisterValidatesBeforeSending(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	d := descriptor()
	d.Version = "not-a-version"
	if err := New(srv.URL).Register(context.Background(), d); err == nil {
		t.Fatal("want error, got nil")
	}
	if called {
		t.Error("a bad descriptor was sent to the platform")
	}
}
