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

// Component is any long-running part of an rApp started alongside the server.
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

// Open marks rApp endpoints as reachable without a session, for cases the SDK
// cannot infer, such as a callback a platform service posts to.
func (a *App) Open(paths ...string) {
	if a.guard != nil {
		a.guard.Open(paths...)
	}
}

// Run blocks until ctx ends, then drains the pipeline and stops the server.
func (a *App) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if a.guard != nil {
		a.guard.Register(a.router)
	}
	a.wireRoutes()

	var wg sync.WaitGroup

	if a.pipeline != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.pipeline.Run(ctx); err != nil {
				a.logger.WithError(err).Error("ingest pipeline stopped")
			}
		}()
	}

	for _, c := range a.components {
		wg.Add(1)
		go func(c Component) {
			defer wg.Done()
			if err := c.Start(ctx); err != nil && ctx.Err() == nil {
				a.logger.WithError(err).WithField("component", c.Name()).Error("component stopped")
			}
		}(c)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.health.Monitor(ctx, a.healthEvery)
	}()

	var handler http.Handler = a.router
	if a.guard != nil {
		handler = a.guard.Wrap(a.router)
		a.logger.Info("operator authentication enabled")
	}

	a.server = &http.Server{
		Addr:              ":" + a.cfg.HTTPPort,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		a.logger.WithFields(logrus.Fields{
			"rapp":    a.cfg.Name,
			"version": a.cfg.Version,
			"port":    a.cfg.HTTPPort,
			"sdk":     rappsdk.UserAgent,
		}).Info("rApp started")
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		cancel()
		wg.Wait()
		return err
	case <-ctx.Done():
	}

	a.logger.Info("shutting down")
	shutdownCtx, stop := context.WithTimeout(context.Background(), a.shutdownWait)
	defer stop()
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		a.logger.WithError(err).Warn("HTTP shutdown did not complete cleanly")
	}

	wg.Wait()
	a.logger.Info("rApp stopped")
	return nil
}

// wireRoutes lets sources and components contribute endpoints without the
// rApp having to remember to register each one.
func (a *App) wireRoutes() {
	var parts []any
	if a.pipeline != nil {
		for _, s := range a.pipeline.Sources() {
			parts = append(parts, s)
		}
	}
	for _, c := range a.components {
		parts = append(parts, c)
	}

	for _, p := range parts {
		if reg, ok := p.(routeRegistrar); ok {
			reg.Register(a.router)
		}
		if op, ok := p.(openLister); ok && a.guard != nil {
			a.guard.Open(op.Open()...)
		}
	}
}

// SignalContext cancels when the platform sends SIGTERM, which is how a
// deployment managed over R1 is stopped.
func SignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
}

// Fatal reports a startup failure the way the platform can see it, then exits.
// The classification goes with it, so a crash loop can be diagnosed from the
// logs alone: a config failure needs someone to change something, while a
// platform one may simply mean the rApp started first.
func Fatal(logger *logrus.Logger, err error, msg string) {
	if logger != nil {
		log.Failure(logger, err, msg)
	} else {
		fmt.Fprintf(os.Stderr, "%s [%s]: %v\n", msg, errs.CategoryOf(err), err)
	}
	os.Exit(1)
}
