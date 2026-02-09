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

// Package sdnr reaches the managed network over RESTCONF through the SMO's
// controller, which fronts each node's O1 NETCONF session.
package sdnr

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	rappsdk "github.com/coranlabs/OTAF/otaf-rapp-sdk"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
)

const (
	yangJSON       = "application/yang-data+json"
	snippetLen     = 512
	defaultTimeout = 30 * time.Second
)

type Config struct {
	Endpoint string        `yaml:"endpoint" env:"SDNR_ADDR"`
	Username string        `yaml:"username" env:"SDNR_USER"`
	Password string        `yaml:"password" env:"SDNR_PASSWORD"`
	NodeID   string        `yaml:"node_id" env:"NODE_ID"`
	Timeout  time.Duration `yaml:"timeout" env:"SDNR_TIMEOUT"`
}

func (c Config) Validate() error {
	if c.Endpoint == "" {
		return errs.New(errs.CategoryConfig, "SDNR_NO_ENDPOINT",
			"sdnr: no endpoint configured")
	}
	if _, err := url.Parse(c.Endpoint); err != nil {
		return errs.Wrap(err, errs.CategoryConfig, "SDNR_BAD_ENDPOINT",
			"sdnr: endpoint is not a URL").WithField("endpoint", c.Endpoint)
	}
	return nil
}

type Client struct {
	base   string
	cfg    Config
	http   *http.Client
	logger *logrus.Logger
}

func New(cfg Config, logger *logrus.Logger) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return &Client{
		base:   strings.TrimRight(cfg.Endpoint, "/"),
		cfg:    cfg,
		http:   &http.Client{Timeout: cfg.Timeout},
		logger: logger,
	}, nil
}

func (c *Client) NodeID() string { return c.cfg.NodeID }

// Ping verifies the controller answers. Pass it to health.Func to have the
// rApp's readiness reflect controller reachability.
func (c *Client) Ping(ctx context.Context) error {
	path := c.base + "/rests/operations"
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode >= 300 {
		return &Error{Method: http.MethodGet, Path: path, Status: resp.StatusCode, Detail: snippet(resp)}
	}
	return nil
}

// MountPath builds a RESTCONF path into the node's mounted O1 datastore.
// Segments are joined as given, so pass them already YANG-qualified.
func (c *Client) MountPath(segments ...string) string {
	p := c.base + "/rests/data/network-topology:network-topology" +
		"/topology=topology-netconf/node=" + url.PathEscape(c.cfg.NodeID) + "/yang-ext:mount"
	for _, s := range segments {
		p += "/" + strings.TrimPrefix(s, "/")
	}
	return p
}

func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &Error{Method: http.MethodGet, Path: path, Cause: err}
	}
	if resp.StatusCode >= 300 {
		return nil, &Error{Method: http.MethodGet, Path: path, Status: resp.StatusCode, Detail: clip(body)}
	}
	return body, nil
}

func (c *Client) Patch(ctx context.Context, path string, body []byte) error {
	return c.write(ctx, http.MethodPatch, path, body)
}

func (c *Client) Put(ctx context.Context, path string, body []byte) error {
	return c.write(ctx, http.MethodPut, path, body)
}

func (c *Client) Post(ctx context.Context, path string, body []byte) error {
	return c.write(ctx, http.MethodPost, path, body)
}

func (c *Client) write(ctx context.Context, method, path string, body []byte) error {
	resp, err := c.do(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode >= 300 {
		return &Error{Method: method, Path: path, Status: resp.StatusCode, Detail: snippet(resp)}
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, path, reader)
	if err != nil {
		return nil, &Error{Method: method, Path: path, Cause: err}
	}
	if c.cfg.Username != "" {
		req.SetBasicAuth(c.cfg.Username, c.cfg.Password)
	}
	req.Header.Set("Accept", yangJSON)
	req.Header.Set("User-Agent", rappsdk.UserAgent)
	if body != nil {
		req.Header.Set("Content-Type", yangJSON)
	}

	if c.logger != nil {
		c.logger.WithFields(logrus.Fields{"method": method, "path": path}).Debug("controller request")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// No status: the request never reached the controller, which is a
		// different problem from the node refusing it.
		return nil, &Error{Method: method, Path: path, Cause: err}
	}
	return resp, nil
}
