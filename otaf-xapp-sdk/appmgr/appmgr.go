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

// Package appmgr registers an xApp with the platform so the routing manager
// will carry its traffic.
package appmgr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/coranlabs/OTAF/otaf-xapp-sdk/errs"
)

// Descriptor is what an xApp tells the platform about itself.
type Descriptor struct {
	Name         string
	Version      string
	HTTPPort     int
	RMRDataPort  int
	RMRRoutePort int
	RxMessages   []string
	TxMessages   []string
	Policies     []int
}

var nameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
var versionRE = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// Validate reports what the platform would reject, before anything is sent.
func (d Descriptor) Validate() error {
	if !nameRE.MatchString(d.Name) {
		return errs.Newf(errs.CategoryConfig, "APPMGR_BAD_NAME", "name %q must be lower case letters, digits and dashes", d.Name)
	}
	if !versionRE.MatchString(d.Version) {
		return errs.Newf(errs.CategoryConfig, "APPMGR_BAD_VERSION", "version %q must be major.minor.patch", d.Version)
	}
	seen := map[int]string{}
	for _, p := range []struct {
		port int
		what string
	}{{d.HTTPPort, "http"}, {d.RMRDataPort, "rmr-data"}, {d.RMRRoutePort, "rmr-route"}} {
		if p.port < 1 || p.port > 65535 {
			return errs.Newf(errs.CategoryConfig, "APPMGR_BAD_PORT", "%s port %d out of range", p.what, p.port)
		}
		if other, dup := seen[p.port]; dup {
			return errs.Newf(errs.CategoryConfig, "APPMGR_BAD_PORT", "%s and %s share port %d", other, p.what, p.port)
		}
		seen[p.port] = p.what
	}
	if d.RMRRoutePort != RoutePort {
		return errs.Newf(errs.CategoryConfig, "APPMGR_BAD_PORT",
			"rmr route port must be %d: the routing manager opens its connection there "+
				"whatever the descriptor says, and reports only that the xApp could not "+
				"be reached", RoutePort)
	}
	return nil
}

// RoutePort is where the routing manager expects to reach an xApp. It is fixed
// in the platform rather than read from the descriptor, so declaring anything
// else leaves the xApp registered but unreachable.
const RoutePort = 4561

// RMREndpoint is the address the routing manager will use to reach this xApp.
// It is derived from the name, not from where the process runs, and a service
// of this name has to resolve for routes to be created.
func (d Descriptor) RMREndpoint() string {
	return fmt.Sprintf("service-ricxapp-%s-rmr.ricxapp:%d", d.Name, d.RMRDataPort)
}

// HTTPEndpoint is where the platform posts subscription responses.
func (d Descriptor) HTTPEndpoint() string {
	return fmt.Sprintf("service-ricxapp-%s-http.ricxapp:%d", d.Name, d.HTTPPort)
}

func (d Descriptor) config() map[string]any {
	port := func(name string, n int, rx, tx []string, pol []int) map[string]any {
		p := map[string]any{"name": name, "container": d.Name, "port": n, "description": name}
		if rx != nil {
			p["rxMessages"] = rx
		}
		if tx != nil {
			p["txMessages"] = tx
		}
		if pol != nil {
			p["policies"] = pol
		}
		return p
	}
	rx, tx := d.RxMessages, d.TxMessages
	if rx == nil {
		rx = []string{}
	}
	if tx == nil {
		tx = []string{}
	}
	pol := d.Policies
	if pol == nil {
		pol = []int{}
	}
	return map[string]any{
		"xapp_name": d.Name,
		"version":   d.Version,
		"containers": []map[string]any{{
			"name":  d.Name,
			"image": map[string]any{"registry": "localhost", "name": d.Name, "tag": d.Version},
		}},
		"messaging": map[string]any{"ports": []map[string]any{
			port("http", d.HTTPPort, nil, nil, nil),
			port("rmr-data", d.RMRDataPort, rx, tx, pol),
			port("rmr-route", d.RMRRoutePort, nil, nil, nil),
		}},
	}
}

// Client talks to the xApp manager.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a client for an xApp manager reachable at addr, such as
// "service-ricplt-appmgr-http.ricplt:8080".
func New(addr string) *Client {
	base := addr
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}
	return &Client{BaseURL: strings.TrimSuffix(base, "/"), HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// registerBody carries the descriptor as a JSON string rather than an object.
// The platform decodes it a second time, and rejects a nested object.
type registerBody struct {
	AppName         string `json:"appName"`
	AppVersion      string `json:"appVersion"`
	ConfigPath      string `json:"configPath"`
	AppInstanceName string `json:"appInstanceName"`
	HTTPEndpoint    string `json:"httpEndpoint"`
	RMREndpoint     string `json:"rmrEndpoint"`
	Config          string `json:"config"`
}

// Register makes the platform aware of an already running xApp.
func (c *Client) Register(ctx context.Context, d Descriptor) error {
	if err := d.Validate(); err != nil {
		return err
	}
	cfg, err := json.Marshal(d.config())
	if err != nil {
		return errs.Wrap(err, errs.CategoryInternal, "APPMGR_ENCODE", "encode descriptor")
	}
	body, err := json.Marshal(registerBody{
		AppName:         d.Name,
		AppVersion:      d.Version,
		AppInstanceName: d.Name,
		HTTPEndpoint:    d.HTTPEndpoint(),
		RMREndpoint:     d.RMREndpoint(),
		Config:          string(cfg),
	})
	if err != nil {
		return errs.Wrap(err, errs.CategoryInternal, "APPMGR_ENCODE", "encode request")
	}
	return c.post(ctx, "/ric/v1/register", body)
}

// Deregister removes an xApp the platform is holding.
func (c *Client) Deregister(ctx context.Context, name string) error {
	body, err := json.Marshal(map[string]string{"appName": name, "appInstanceName": name})
	if err != nil {
		return err
	}
	return c.post(ctx, "/ric/v1/deregister", body)
}

func (c *Client) post(ctx context.Context, path string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return errs.Wrap(err, errs.CategoryPlatform, "APPMGR_UNAVAILABLE", path)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	category, code := errs.CategoryPlatform, "APPMGR_UNAVAILABLE"
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		category, code = errs.CategoryConfig, "APPMGR_REJECTED"
	}
	return errs.Newf(category, code, "%s: %s: %s", path, resp.Status, strings.TrimSpace(string(detail)))
}
