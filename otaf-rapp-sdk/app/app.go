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

// Package app runs an rApp: one HTTP server, one ingest pipeline, the probes
// rApp Manager relies on, and an orderly shutdown when the platform stops it.
package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	rappsdk "github.com/coranlabs/OTAF/otaf-rapp-sdk"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/auth"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/config"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/health"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/ingest"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/log"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/metrics"
)

type Component interface {
	Name() string
	Start(ctx context.Context) error
}

// A source or component implementing routeRegistrar gets its endpoints wired
// before the server starts listening.
type routeRegistrar interface {
	Register(r *mux.Router)
}

// openLister names paths that must stay reachable without an operator session.
type openLister interface {
	Open() []string
}

type App struct {
	cfg    config.Rapp
	logger *logrus.Logger
	router *mux.Router

	guard    *auth.Guard
	health   *health.Registry
	pipeline *ingest.Pipeline
	metrics  *metrics.Metrics

	components   []Component
	statusExtra  func() map[string]any
	healthEvery  time.Duration
	shutdownWait time.Duration

	startedAt time.Time
	server    *http.Server
}

type Option func(*App)

func WithLogger(l *logrus.Logger) Option { return func(a *App) { a.logger = l } }

func WithGuard(g *auth.Guard) Option { return func(a *App) { a.guard = g } }

func WithPipeline(p *ingest.Pipeline) Option { return func(a *App) { a.pipeline = p } }

func WithComponent(c Component) Option {
	return func(a *App) { a.components = append(a.components, c) }
}

func WithHealthInterval(d time.Duration) Option {
	return func(a *App) {
		if d > 0 {
			a.healthEvery = d
		}
	}
}

// WithStatusDetail adds rApp-specific fields to the status payload.
func WithStatusDetail(fn func() map[string]any) Option {
	return func(a *App) { a.statusExtra = fn }
}

func New(cfg config.Rapp, opts ...Option) (*App, error) {
	a := &App{
		cfg:          cfg,
		router:       mux.NewRouter(),
		healthEvery:  2 * time.Minute,
		shutdownWait: 10 * time.Second,
		startedAt:    time.Now(),
	}
	for _, o := range opts {
		o(a)
	}
	if a.logger == nil {
		a.logger = log.New(cfg.LogLevel, cfg.LogFormat)
	}
	if a.health == nil {
		a.health = health.NewRegistry(a.logger)
	}
	if cfg.HTTPPort == "" {
		return nil, errs.New(errs.CategoryConfig, "APP_NO_HTTP_PORT",
			"app: no HTTP port configured")
	}

	// Snapshots are read at scrape time, so the metrics can never disagree
	// with what /status reports.
	a.metrics = metrics.New(cfg.Name, cfg.Version, metrics.Snapshots{
		Ingest: func() metrics.IngestStats {
			if a.pipeline == nil {
				return metrics.IngestStats{}
			}
			s := a.pipeline.Stats()
			return metrics.IngestStats{
				Queued: s.Queued, Capacity: s.Capacity,
				Accepted: s.Accepted, Dropped: s.Dropped,
				Failed: s.Failed, Processed: s.Processed,
			}
		},
		Dependency: func() map[string]bool {
			out := map[string]bool{}
			for name, status := range a.health.Snapshot() {
				out[name] = status.Healthy
			}
			return out
		},
	})
	if a.pipeline != nil {
		a.pipeline.SetObserver(ingest.ObserverFunc(a.metrics.Handled))
	}

	a.registerBuiltins()
	return a, nil
}

// Router exposes the mux an rApp adds its own endpoints to. Anything
// registered here sits behind the guard unless explicitly opened.
func (a *App) Router() *mux.Router { return a.router }

func (a *App) Logger() *logrus.Logger { return a.logger }

func (a *App) Health() *health.Registry { return a.health }

// Metrics exposes the rApp's metric set so it can register measurements of its
// own on the same endpoint.
func (a *App) Metrics() *metrics.Metrics { return a.metrics }

func (a *App) Config() config.Rapp { return a.cfg }
