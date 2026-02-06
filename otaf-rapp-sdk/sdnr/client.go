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
