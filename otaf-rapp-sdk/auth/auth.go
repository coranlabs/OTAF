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

// Package auth puts an rApp's own APIs behind operator accounts. Platform
// endpoints stay open so rApp Manager, DME and the probes can reach them.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
)

const (
	CookieName      = "rapp_session"
	defaultTTL      = 12 * time.Hour
	maxLoginFails   = 5
	lockWindow      = time.Minute
	maxTrackedPeers = 4096
)

type ctxKey struct{}

type session struct {
	user    string
	expires time.Time
}

type peer struct {
	fails  int
	locked time.Time
	seen   time.Time
}

type Guard struct {
	logger     *logrus.Logger
	users      map[string][]byte
	ttl        time.Duration
	secure     bool
	trustProxy bool

	mu       sync.Mutex
	sessions map[string]session
	peers    map[string]*peer

	openMu sync.RWMutex
	open   map[string]struct{}
	prefix []string
}
