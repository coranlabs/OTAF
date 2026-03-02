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

// Package a1 steers the Near-RT RIC. An rApp registers as a service with the
// A1 policy management service, then creates and withdraws policy instances on
// the RICs that support the policy types it cares about.
package a1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	rappsdk "github.com/coranlabs/OTAF/otaf-rapp-sdk"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/retry"
)

const (
	basePath       = "/a1-policy/v2"
	defaultTimeout = 30 * time.Second

	// A service that stops calling keep-alive has its policies withdrawn, so
	// the heartbeat runs well inside the interval it registered.
	keepAliveDivisor = 3
	minKeepAlive     = 30 * time.Second
)

type Config struct {
	Endpoint string `yaml:"endpoint" env:"A1_PMS_ADDR"`

	// ServiceID identifies this rApp's policies. Reusing it after a restart is
	// what lets an rApp reclaim the policies it created before.
	ServiceID string `yaml:"service_id" env:"A1_SERVICE_ID"`

	KeepAlive   time.Duration `yaml:"keep_alive" env:"A1_KEEP_ALIVE"`
	CallbackURL string        `yaml:"callback_url" env:"A1_CALLBACK_URL"`
	Timeout     time.Duration `yaml:"timeout" env:"A1_TIMEOUT"`
}

func (c Config) Enabled() bool { return strings.TrimSpace(c.Endpoint) != "" }

func (c Config) Validate() error {
	if !c.Enabled() {
		return nil
	}
	if _, err := url.Parse(c.Endpoint); err != nil {
		return errs.Wrap(err, errs.CategoryConfig, "A1_BAD_ENDPOINT",
			"a1: endpoint is not a URL").WithField("endpoint", c.Endpoint)
	}
	if strings.TrimSpace(c.ServiceID) == "" {
		return errs.New(errs.CategoryConfig, "A1_NO_SERVICE_ID",
			"a1: service_id is required; it is how the platform attributes policies to this rApp")
	}
	if c.KeepAlive < 0 {
		return errs.New(errs.CategoryConfig, "A1_BAD_KEEP_ALIVE",
			"a1: keep_alive must not be negative")
	}
	return nil
}

type Ric struct {
	ID                string   `json:"ric_id"`
	ManagedElementIDs []string `json:"managed_element_ids"`
	State             string   `json:"state"`
	PolicyTypeIDs     []string `json:"policytype_ids"`
}

func (r Ric) Available() bool { return r.State == "AVAILABLE" }

type PolicyType struct {
	ID string `json:"-"`
	// Schema is the JSON schema a policy's data must satisfy. The platform
	// validates against it and rejects anything that does not fit.
	Schema json.RawMessage `json:"policy_schema"`
}

type Policy struct {
	ID           string          `json:"policy_id"`
	RicID        string          `json:"ric_id"`
	ServiceID    string          `json:"service_id"`
	PolicyTypeID string          `json:"policytype_id"`
	Data         json.RawMessage `json:"policy_data"`

	// Transient policies are dropped if the RIC restarts, rather than being
	// restored. Use it for decisions that are only valid right now.
	Transient bool `json:"transient"`

	StatusNotificationURI string `json:"status_notification_uri,omitempty"`
}

type Client struct {
	base   string
	cfg    Config
	http   *http.Client
	logger *logrus.Logger

	retry            retry.Policy
	deregisterOnStop bool
}

func (c *Client) PolicyType(ctx context.Context, id string) (*PolicyType, error) {
	body, err := c.do(ctx, http.MethodGet, "/policy-types/"+url.PathEscape(id), nil, "get policy type")
	if err != nil {
		return nil, err
	}
	pt := &PolicyType{ID: id}
	if err := json.Unmarshal(body, pt); err != nil {
		return nil, fmt.Errorf("a1 get policy type: %w", err)
	}
	return pt, nil
}

func (c *Client) Policy(ctx context.Context, id string) (*Policy, error) {
	body, err := c.do(ctx, http.MethodGet, "/policies/"+url.PathEscape(id), nil, "get policy")
	if err != nil {
		return nil, err
	}
	var p Policy
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("a1 get policy: %w", err)
	}
	return &p, nil
}

type Filter struct {
	ServiceID    string
	RicID        string
	PolicyTypeID string
	// AllServices lifts the default of listing only this rApp's policies.
	AllServices bool
}
