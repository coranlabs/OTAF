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

// Package influx persists the time series an rApp derives, in the store
// deployed beside it by the same rApp package.
package influx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	rappsdk "github.com/coranlabs/OTAF/otaf-rapp-sdk"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
)

const (
	defaultBuffer = 4096
	batchSize     = 500
	flushEvery    = 5 * time.Second
)

type Config struct {
	URL    string `yaml:"url" env:"INFLUX_URL"`
	Org    string `yaml:"org" env:"INFLUX_ORG"`
	Bucket string `yaml:"bucket" env:"INFLUX_BUCKET"`
	Token  string `yaml:"token" env:"INFLUX_TOKEN"`
}

func (c Config) Enabled() bool { return strings.TrimSpace(c.URL) != "" }

func (c Config) Validate() error {
	if !c.Enabled() {
		return nil
	}
	if c.Bucket == "" {
		return errs.New(errs.CategoryConfig, "INFLUX_NO_BUCKET",
			"influx: bucket is required when a URL is set")
	}
	if c.Org == "" {
		return errs.New(errs.CategoryConfig, "INFLUX_NO_ORG",
			"influx: org is required when a URL is set")
	}
	return nil
}

type Writer struct {
	cfg    Config
	logger *logrus.Logger
	http   *http.Client
	lines  chan string

	dropped atomic.Uint64
	written atomic.Uint64
}

type Stats struct {
	Queued  int    `json:"queued"`
	Written uint64 `json:"written"`
	Dropped uint64 `json:"dropped"`
}

func (w *Writer) Stats() Stats {
	if w == nil {
		return Stats{}
	}
	return Stats{Queued: len(w.lines), Written: w.written.Load(), Dropped: w.dropped.Load()}
}

var keyEscaper = strings.NewReplacer(",", `\,`, " ", `\ `, "=", `\=`)
