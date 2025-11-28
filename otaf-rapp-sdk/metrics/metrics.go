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

// Snapshots are read at scrape time rather than mirrored into counters, so the
// numbers can never drift from the values the rApp reports elsewhere.
type Snapshots struct {
	Ingest     func() IngestStats
	Dependency func() map[string]bool
}

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

// New builds the metric set for one rApp. Its own collectors are registered
// alongside the Go runtime and process collectors.
func New(rappName, version string, snaps Snapshots) *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		registry: reg,
		handlerDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "handler_duration_seconds",
			Help:      "Time the rApp's handler spent on one message.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"source", "outcome"}),
		deliveries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "r1_deliveries_total",
			Help:      "Deliveries attempted to R1 information job targets.",
		}, []string{"outcome"}),
		policyOps: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "a1_policy_operations_total",
			Help:      "A1 policy operations by verb and outcome.",
		}, []string{"operation", "outcome"}),
		failures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "failures_total",
			Help: "Failures by what kind they were. Both labels must come from a " +
				"fixed set: a code derived from per-message data would make this " +
				"series unbounded.",
		}, []string{"category", "code"}),
	}

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "build_info",
		Help:      "Constant 1, labelled with what this rApp is and what built it.",
	}, []string{"rapp", "version", "sdk"})
	buildInfo.WithLabelValues(rappName, version, rappsdk.UserAgent).Set(1)

	startTime := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "start_time_seconds",
		Help:      "Unix time the rApp started.",
	})
	startTime.Set(float64(time.Now().Unix()))

	reg.MustRegister(
		m.handlerDuration,
		m.deliveries,
		m.policyOps,
		m.failures,
		buildInfo,
		startTime,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		&snapshotCollector{snaps: snaps},
	)

	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Registerer lets an rApp add metrics of its own to the same endpoint.
func (m *Metrics) Registerer() prometheus.Registerer { return m.registry }

var (
	queuedDesc = prometheus.NewDesc(
		namespace+"_ingest_queue_depth", "Messages waiting to be handled.", nil, nil)
	capacityDesc = prometheus.NewDesc(
		namespace+"_ingest_queue_capacity", "How many messages the queue can hold.", nil, nil)
	messagesDesc = prometheus.NewDesc(
		namespace+"_ingest_messages_total", "Messages by what became of them.", []string{"outcome"}, nil)
	dependencyDesc = prometheus.NewDesc(
		namespace+"_dependency_up", "1 when a platform dependency answered its last check.",
		[]string{"dependency"}, nil)
)
