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
