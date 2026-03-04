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

func (r Ric) Supports(policyTypeID string) bool {
	for _, id := range r.PolicyTypeIDs {
		if id == policyTypeID {
			return true
		}
	}
	return false
}

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

// Status is whatever the RIC reports about a policy. The outer envelope is
// fixed; what is inside varies by RIC and policy type, so it stays raw.
type Status struct {
	LastModified time.Time       `json:"last_modified"`
	Status       json.RawMessage `json:"status"`
}

// Error carries what the policy management service said about a rejection.
type Error struct {
	Op     string
	Status int
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("a1 %s: status %d", e.Op, e.Status)
	}
	return fmt.Sprintf("a1 %s: status %d: %s", e.Op, e.Status, e.Detail)
}

// Retryable reports whether trying the same call again could succeed. A
// refusal on the request's own merits never can; the service being briefly
// unavailable usually does.
func (e *Error) Retryable() bool {
	switch e.Status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	}
	return e.Status >= 500
}

// The three methods below let errs classify this failure without either
// package importing the other.

func (e *Error) ErrorCategory() string { return "platform" }

func (e *Error) HTTPStatus() int { return e.Status }

// ErrorCode is stable across rewording of the message, so it survives in a
// ticket or an alert rule.
func (e *Error) ErrorCode() string {
	switch {
	case e.Status == http.StatusNotFound:
		return "A1_NOT_FOUND"
	case e.Status == http.StatusConflict:
		return "A1_CONFLICT"
	case e.Status >= 500:
		return "A1_UNAVAILABLE"
	case e.Status >= 400:
		return "A1_REJECTED"
	default:
		return "A1_FAILED"
	}
}

func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

// IsRejected reports a policy the platform refused, usually because its data
// does not match the policy type's schema.
func IsRejected(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusBadRequest
}

type Client struct {
	base   string
	cfg    Config
	http   *http.Client
	logger *logrus.Logger

	retry            retry.Policy
	deregisterOnStop bool
}

// New returns nil when no endpoint is configured, so an rApp that does not
// steer the RIC can hold a nil *Client safely.
func New(cfg Config, logger *logrus.Logger, opts ...Option) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.Enabled() {
		return nil, nil
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.KeepAlive == 0 {
		cfg.KeepAlive = 5 * time.Minute
	}
	c := &Client{
		base:   strings.TrimRight(cfg.Endpoint, "/") + basePath,
		cfg:    cfg,
		http:   &http.Client{Timeout: cfg.Timeout},
		logger: logger,
		retry:  retry.Default(),
	}
	c.apply(opts...)
	return c, nil
}

// WithRetry replaces how transient failures are re-attempted. Pass
// retry.None() to have every call fail on its first refusal.
func WithRetry(p retry.Policy) Option { return func(c *Client) { c.retry = p } }

func (c *Client) ServiceID() string { return c.cfg.ServiceID }

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "/status", nil, "status")
	return err
}

// Rics lists the Near-RT RICs known to the platform, optionally only those
// supporting a policy type.
func (c *Client) Rics(ctx context.Context, policyTypeID string) ([]Ric, error) {
	path := "/rics"
	if policyTypeID != "" {
		path += "?policytype_id=" + url.QueryEscape(policyTypeID)
	}
	body, err := c.do(ctx, http.MethodGet, path, nil, "list rics")
	if err != nil {
		return nil, err
	}
	var out struct {
		Rics []Ric `json:"rics"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("a1 list rics: %w", err)
	}
	return out.Rics, nil
}

// RicFor picks an available RIC that supports the policy type, preferring one
// that also manages the given element when an id is supplied.
func (c *Client) RicFor(ctx context.Context, policyTypeID, managedElementID string) (*Ric, error) {
	rics, err := c.Rics(ctx, policyTypeID)
	if err != nil {
		return nil, err
	}

	var fallback *Ric
	for i := range rics {
		ric := rics[i]
		if !ric.Available() {
			continue
		}
		if managedElementID == "" {
			return &ric, nil
		}
		for _, me := range ric.ManagedElementIDs {
			if me == managedElementID {
				return &ric, nil
			}
		}
		if fallback == nil {
			fallback = &ric
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	// Not a misconfiguration: a RIC that is down or has not registered the
	// type yet may well be there on the next attempt.
	return nil, errs.Newf(errs.CategoryPlatform, "A1_NO_SUITABLE_RIC",
		"a1: no available Near-RT RIC supports policy type %s", policyTypeID).
		WithField("policy_type", policyTypeID).Transient()
}

func (c *Client) PolicyTypes(ctx context.Context, ricID string) ([]string, error) {
	path := "/policy-types"
	if ricID != "" {
		path += "?ric_id=" + url.QueryEscape(ricID)
	}
	body, err := c.do(ctx, http.MethodGet, path, nil, "list policy types")
	if err != nil {
		return nil, err
	}
	var out struct {
		IDs []string `json:"policytype_ids"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("a1 list policy types: %w", err)
	}
	return out.IDs, nil
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

// PutPolicy creates or replaces a policy instance. The service id is filled in
// from the client's configuration when the caller leaves it blank.
func (c *Client) PutPolicy(ctx context.Context, p Policy) error {
	// Missing identifiers are the caller's mistake, not the platform's, and no
	// retry or outage explains them.
	if p.ID == "" {
		return errs.New(errs.CategoryInternal, "A1_MISSING_POLICY_ID", "a1: policy id is required")
	}
	if p.RicID == "" {
		return errs.New(errs.CategoryInternal, "A1_MISSING_RIC_ID", "a1: ric id is required")
	}
	if p.ServiceID == "" {
		p.ServiceID = c.cfg.ServiceID
	}
	if len(p.Data) == 0 {
		p.Data = json.RawMessage("{}")
	}

	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("a1 put policy: %w", err)
	}
	if _, err := c.do(ctx, http.MethodPut, "/policies", body, "put policy"); err != nil {
		return err
	}

	c.logger.WithFields(logrus.Fields{
		"policy": p.ID,
		"ric":    p.RicID,
		"type":   p.PolicyTypeID,
	}).Info("A1 policy applied")
	return nil
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

func (c *Client) PolicyStatus(ctx context.Context, id string) (*Status, error) {
	body, err := c.do(ctx, http.MethodGet, "/policies/"+url.PathEscape(id)+"/status", nil, "get policy status")
	if err != nil {
		return nil, err
	}
	var s Status
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("a1 get policy status: %w", err)
	}
	return &s, nil
}

// Policies lists policy ids. With no filter it returns only this rApp's own.
func (c *Client) Policies(ctx context.Context, filter Filter) ([]string, error) {
	if filter.ServiceID == "" && !filter.AllServices {
		filter.ServiceID = c.cfg.ServiceID
	}
	body, err := c.do(ctx, http.MethodGet, "/policies"+filter.query(), nil, "list policies")
	if err != nil {
		return nil, err
	}
	var out struct {
		IDs []string `json:"policy_ids"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("a1 list policies: %w", err)
	}
	return out.IDs, nil
}

type Filter struct {
	ServiceID    string
	RicID        string
	PolicyTypeID string
	// AllServices lifts the default of listing only this rApp's policies.
	AllServices bool
}

func (f Filter) query() string {
	values := url.Values{}
	if f.ServiceID != "" {
		values.Set("service_id", f.ServiceID)
	}
	if f.RicID != "" {
		values.Set("ric_id", f.RicID)
	}
	if f.PolicyTypeID != "" {
		values.Set("policytype_id", f.PolicyTypeID)
	}
	if len(values) == 0 {
		return ""
	}
	return "?" + values.Encode()
}

// DeletePolicy withdraws a policy. Deleting one that is already gone is not an
// error, so cleanup paths stay simple.
func (c *Client) DeletePolicy(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/policies/"+url.PathEscape(id), nil, "delete policy")
	if err != nil && !IsNotFound(err) {
		return err
	}
	return nil
}

// DeleteAllPolicies withdraws everything this rApp created, which is what an
// rApp should do when it stops steering.
func (c *Client) DeleteAllPolicies(ctx context.Context) error {
	ids, err := c.Policies(ctx, Filter{})
	if err != nil {
		return err
	}
	var failed []string
	for _, id := range ids {
		if err := c.DeletePolicy(ctx, id); err != nil {
			failed = append(failed, id)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("a1: could not withdraw policies %s", strings.Join(failed, ", "))
	}
	return nil
}

// Every verb this client uses is idempotent, so retrying a call that failed
// for a transient reason is always safe.
func (c *Client) do(ctx context.Context, method, path string, body []byte, op string) ([]byte, error) {
	return retry.DoValue(ctx, c.retry, func(ctx context.Context, _ int) ([]byte, error) {
		return c.attempt(ctx, method, path, body, op)
	})
}
