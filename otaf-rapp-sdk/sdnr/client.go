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
