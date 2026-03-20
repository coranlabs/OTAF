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

package app

import (
	"encoding/json"
	"net/http"
	"time"

	rappsdk "github.com/coranlabs/OTAF/otaf-rapp-sdk"
)

func (a *App) registerBuiltins() {
	a.router.HandleFunc("/health", a.handleHealth).Methods(http.MethodGet)
	a.router.HandleFunc("/ready", a.handleReady).Methods(http.MethodGet)
	a.router.HandleFunc("/status", a.handleStatus).Methods(http.MethodGet)
	a.router.Handle("/metrics", a.metrics.Handler()).Methods(http.MethodGet)
}

// Liveness stays independent of dependency state: an rApp that cannot reach a
// platform service is still alive, and restarting it would not help.
func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "healthy",
		"rapp":    a.cfg.Name,
		"version": a.cfg.Version,
	})
}

func (a *App) handleReady(w http.ResponseWriter, r *http.Request) {
	ready := a.health.Healthy()
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]any{
		"ready":        ready,
		"dependencies": a.health.Snapshot(),
	})
}

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{
		"rapp":         a.cfg.Name,
		"version":      a.cfg.Version,
		"sdk":          rappsdk.UserAgent,
		"uptime_s":     int(time.Since(a.startedAt).Seconds()),
		"started_at":   a.startedAt.UTC().Format(time.RFC3339),
		"dependencies": a.health.Snapshot(),
	}
	if a.pipeline != nil {
		body["ingest"] = a.pipeline.Stats()
	}
	if a.statusExtra != nil {
		for k, v := range a.statusExtra() {
			body[k] = v
		}
	}
	writeJSON(w, http.StatusOK, body)
}
