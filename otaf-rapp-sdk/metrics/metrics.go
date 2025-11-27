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

// Package metrics exposes an rApp in Prometheus exposition format, so the
// monitoring stack already running beside it can scrape without a translator.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	rappsdk "github.com/coranlabs/OTAF/otaf-rapp-sdk"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
)

const namespace = "rapp"

type IngestStats struct {
	Queued    int
	Capacity  int
	Accepted  uint64
	Dropped   uint64
	Failed    uint64
	Processed uint64
}

type Metrics struct {
	registry *prometheus.Registry

	handlerDuration *prometheus.HistogramVec
	deliveries      *prometheus.CounterVec
	policyOps       *prometheus.CounterVec
	failures        *prometheus.CounterVec
}
