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

// Compared against when the account does not exist, so a wrong username and a
// wrong password take the same time to reject. The password behind it is
// random and discarded; nothing is meant to match this.
var decoyHash = []byte("$2a$10$g/00j5A0w1be6K3ycmko3O7AZvyPJNSfOGqBuxDIaXrp8QyfQL6Uy")

type ctxKey struct{}

// UserOf returns the operator behind the request, or "" when unauthenticated.
func UserOf(r *http.Request) string {
	if v, ok := r.Context().Value(ctxKey{}).(string); ok {
		return v
	}
	return ""
}

// Hash produces the bcrypt value stored in the chart for an operator account.
func Hash(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h), err
}

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

type Option func(*Guard)

func WithTTL(d time.Duration) Option {
	return func(g *Guard) {
		if d > 0 {
			g.ttl = d
		}
	}
}

// WithSecureCookie marks the session cookie Secure. Enable it whenever the
// rApp is reached over TLS.
func WithSecureCookie(on bool) Option { return func(g *Guard) { g.secure = on } }

// WithTrustedProxy derives the client address from X-Forwarded-For. Without
// it, every request behind an ingress shares one address and a handful of bad
// logins would lock out every operator at once.
func WithTrustedProxy(on bool) Option { return func(g *Guard) { g.trustProxy = on } }

// NewGuard builds a guard from "name:bcrypt-hash" pairs. An empty spec returns
// a nil guard, which leaves the rApp's APIs unauthenticated.
func NewGuard(spec string, logger *logrus.Logger, opts ...Option) (*Guard, error) {
	if strings.TrimSpace(spec) == "" {
		if logger != nil {
			logger.Warn("no operator accounts configured: rApp APIs are unauthenticated")
		}
		return nil, nil
	}

	users := map[string][]byte{}
	for _, entry := range strings.Split(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, hash, ok := strings.Cut(entry, ":")
		if !ok || name == "" || hash == "" {
			return nil, badAccount(entry, "want name:bcrypt-hash")
		}
		if _, err := bcrypt.Cost([]byte(hash)); err != nil {
			return nil, badAccount(name, "value is not a bcrypt hash")
		}
		users[name] = []byte(hash)
	}
	if len(users) == 0 {
		return nil, nil
	}

	g := &Guard{
		logger:   logger,
		users:    users,
		ttl:      defaultTTL,
		sessions: map[string]session{},
		peers:    map[string]*peer{},
		open:     map[string]struct{}{},
	}
	for _, o := range opts {
		o(g)
	}
	g.Open("/health", "/ready", "/status", "/metrics", "/api/login")
	g.OpenPrefix("/r1/")
	return g, nil
}

// A malformed account spec is a deployment that will never work, so it is
// classified the same as any other misconfiguration.
func badAccount(entry, reason string) error {
	return errs.Newf(errs.CategoryConfig, "AUTH_BAD_ACCOUNT_SPEC",
		"operator account %s is invalid: %s", entry, reason).WithField("entry", entry)
}

func (g *Guard) Open(paths ...string) {
	g.openMu.Lock()
	defer g.openMu.Unlock()
	for _, p := range paths {
		g.open[p] = struct{}{}
	}
}

func (g *Guard) OpenPrefix(prefixes ...string) {
	g.openMu.Lock()
	defer g.openMu.Unlock()
	g.prefix = append(g.prefix, prefixes...)
}

func (g *Guard) isOpen(path string) bool {
	g.openMu.RLock()
	defer g.openMu.RUnlock()
	if _, ok := g.open[path]; ok {
		return true
	}
	for _, p := range g.prefix {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func (g *Guard) Register(r *mux.Router) {
	r.HandleFunc("/api/login", g.login).Methods(http.MethodPost)
	r.HandleFunc("/api/logout", g.logout).Methods(http.MethodPost)
	r.HandleFunc("/api/me", g.me).Methods(http.MethodGet)
}

func (g *Guard) Wrap(next http.Handler) http.Handler {
	if g == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := g.userOf(r)
		if ok {
			r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, user))
		}
		if ok || g.isOpen(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	})
}

func (g *Guard) userOf(r *http.Request) (string, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return "", false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	s, ok := g.sessions[c.Value]
	if !ok {
		return "", false
	}
	if time.Now().After(s.expires) {
		delete(g.sessions, c.Value)
		return "", false
	}
	return s.user, true
}

func (g *Guard) clientAddr(r *http.Request) string {
	if g.trustProxy {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			if first, _, found := strings.Cut(fwd, ","); found {
				return strings.TrimSpace(first)
			}
			return strings.TrimSpace(fwd)
		}
		if real := r.Header.Get("X-Real-IP"); real != "" {
			return strings.TrimSpace(real)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
