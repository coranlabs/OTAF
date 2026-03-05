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

package a1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/sirupsen/logrus"
)

type Registration struct {
	ServiceID   string `json:"service_id"`
	KeepAlive   int    `json:"keep_alive_interval_seconds"`
	CallbackURL string `json:"callback_url"`
}

type registered struct {
	ServiceID   string `json:"service_id"`
	KeepAlive   int    `json:"keep_alive_interval_seconds"`
	CallbackURL string `json:"callback_url"`
	IdleSeconds int    `json:"time_since_last_activity_seconds"`
}

// Register announces this rApp so the platform will accept policies from it.
// Registering again with the same service id is how an rApp reclaims the
// policies it created before a restart.
func (c *Client) Register(ctx context.Context) error {
	body, err := json.Marshal(Registration{
		ServiceID:   c.cfg.ServiceID,
		KeepAlive:   int(c.cfg.KeepAlive.Seconds()),
		CallbackURL: c.cfg.CallbackURL,
	})
	if err != nil {
		return fmt.Errorf("a1 register: %w", err)
	}
	if _, err := c.do(ctx, http.MethodPut, "/services", body, "register service"); err != nil {
		return err
	}

	c.logger.WithFields(logrus.Fields{
		"service":    c.cfg.ServiceID,
		"keep_alive": c.cfg.KeepAlive,
	}).Info("registered with A1 policy management")
	return nil
}

// KeepAlive tells the platform this rApp is still alive. Letting it lapse has
// the platform withdraw every policy the rApp created.
func (c *Client) KeepAlive(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodPut,
		"/services/"+url.PathEscape(c.cfg.ServiceID)+"/keepalive", nil, "keep alive")
	return err
}

// Deregister removes the registration. The platform withdraws this rApp's
// policies along with it, so this is a deliberate stand-down, not a shutdown
// step.
func (c *Client) Deregister(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodDelete,
		"/services/"+url.PathEscape(c.cfg.ServiceID), nil, "deregister service")
	if err != nil && !IsNotFound(err) {
		return err
	}
	c.logger.WithField("service", c.cfg.ServiceID).Info("deregistered from A1 policy management")
	return nil
}

// Registered reports whether the platform currently knows this rApp.
func (c *Client) Registered(ctx context.Context) (bool, error) {
	body, err := c.do(ctx, http.MethodGet,
		"/services?service_id="+url.QueryEscape(c.cfg.ServiceID), nil, "list services")
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	var out struct {
		Services []registered `json:"service_list"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return false, fmt.Errorf("a1 list services: %w", err)
	}
	return len(out.Services) > 0, nil
}

// DeregisterOnStop has the rApp stand down cleanly when it shuts down, which
// withdraws every policy it created. Off by default: a rolling restart would
// otherwise revert the network for as long as the rApp takes to come back.
func DeregisterOnStop(on bool) Option { return func(c *Client) { c.deregisterOnStop = on } }

type Option func(*Client)

func (c *Client) apply(opts ...Option) {
	for _, o := range opts {
		o(c)
	}
}

func (c *Client) Name() string { return "a1:" + c.cfg.ServiceID }
