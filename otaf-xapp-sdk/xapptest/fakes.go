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

// Package xapptest stands in for the platform an xApp runs on, so its logic
// can be exercised without a RIC.
package xapptest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/coranlabs/OTAF/otaf-xapp-sdk/appmgr"
)

// Registration is what an xApp told the platform about itself.
type Registration struct {
	Name         string
	Version      string
	HTTPEndpoint string
	RMREndpoint  string
	Descriptor   map[string]any
}

// Platform records what an xApp asked of the xApp manager, and can be made to
// refuse so the xApp's own handling of that is exercised.
type Platform struct {
	server *httptest.Server

	mu           sync.Mutex
	registered   []Registration
	deregistered []string
	reject       int
}

// NewPlatform returns a client wired to a stand-in xApp manager.
func NewPlatform(t testing.TB) (*appmgr.Client, *Platform) {
	t.Helper()
	p := &Platform{}
	p.server = httptest.NewServer(http.HandlerFunc(p.handle))
	t.Cleanup(p.server.Close)
	return appmgr.New(p.server.URL), p
}

func (p *Platform) handle(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	reject := p.reject
	p.mu.Unlock()
	if reject != 0 {
		w.WriteHeader(reject)
		io.WriteString(w, `{"message":"refused by the stand-in platform"}`)
		return
	}

	body, _ := io.ReadAll(r.Body)
	switch r.URL.Path {
	case "/ric/v1/register":
		var got struct {
			AppName      string `json:"appName"`
			AppVersion   string `json:"appVersion"`
			HTTPEndpoint string `json:"httpEndpoint"`
			RMREndpoint  string `json:"rmrEndpoint"`
			Config       string `json:"config"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		reg := Registration{
			Name: got.AppName, Version: got.AppVersion,
			HTTPEndpoint: got.HTTPEndpoint, RMREndpoint: got.RMREndpoint,
		}
		// The descriptor arrives as a string holding JSON, so it is decoded a
		// second time here exactly as the platform does.
		_ = json.Unmarshal([]byte(got.Config), &reg.Descriptor)
		p.mu.Lock()
		p.registered = append(p.registered, reg)
		p.mu.Unlock()
		w.WriteHeader(http.StatusCreated)

	case "/ric/v1/deregister":
		var got struct {
			AppName string `json:"appName"`
		}
		_ = json.Unmarshal(body, &got)
		p.mu.Lock()
		p.deregistered = append(p.deregistered, got.AppName)
		p.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// Registrations returns what has been registered, in order.
func (p *Platform) Registrations() []Registration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Registration(nil), p.registered...)
}

// Deregistrations returns the names that have been removed, in order.
func (p *Platform) Deregistrations() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.deregistered...)
}

// Refuse makes the platform answer everything with status until it is called
// again with zero.
func (p *Platform) Refuse(status int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reject = status
}
