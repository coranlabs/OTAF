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

// Package health tracks the reachability of the platform services an rApp
// depends on and exposes the result through the rApp's status endpoint.
package health

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

type CheckerFunc struct {
	Label string
	Fn    func(ctx context.Context) error
}

func (c CheckerFunc) Name() string                    { return c.Label }

func (c CheckerFunc) Check(ctx context.Context) error { return c.Fn(ctx) }

type Status struct {
	Healthy bool      `json:"healthy"`
	Error   string    `json:"error,omitempty"`
	Checked time.Time `json:"checked_at"`
}

type Registry struct {
	logger *logrus.Logger

	mu       sync.RWMutex
	checkers []Checker
	status   map[string]Status
}
