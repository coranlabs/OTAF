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

package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

func quietLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(nopWriter{})
	return l
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

func newTestGuard(t *testing.T, opts ...Option) *Guard {
	t.Helper()
	hash, err := Hash("correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	g, err := NewGuard("operator:"+hash, quietLogger(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	if g == nil {
		t.Fatal("guard should not be nil for a valid account spec")
	}
	return g
}

func protectedServer(g *Guard) http.Handler {
	r := mux.NewRouter()
	g.Register(r)
	r.HandleFunc("/private", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(UserOf(req)))
	})
	return g.Wrap(r)
}

func login(t *testing.T, h http.Handler, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	req.RemoteAddr = "10.0.0.1:5000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestEmptySpecDisablesAuthentication(t *testing.T) {
	g, err := NewGuard("  ", quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if g != nil {
		t.Fatal("an empty account spec should leave the rApp unauthenticated")
	}
}

func TestMalformedSpecIsRejected(t *testing.T) {
	if _, err := NewGuard("operator:not-a-bcrypt-hash", quietLogger()); err == nil {
		t.Fatal("expected an error for a non-bcrypt hash")
	}
	if _, err := NewGuard("missing-hash", quietLogger()); err == nil {
		t.Fatal("expected an error for an entry without a hash")
	}
}

func TestLoginGrantsAccessToProtectedRoute(t *testing.T) {
	g := newTestGuard(t)
	h := protectedServer(g)

	resp := login(t, h, `{"username":"operator","password":"correct-horse"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", resp.StatusCode)
	}

	cookies := resp.Cookies()
	if len(cookies) == 0 || cookies[0].Name != CookieName {
		t.Fatal("login should set the session cookie")
	}
	if !cookies[0].HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.AddCookie(cookies[0])
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("protected route status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "operator" {
		t.Errorf("handler saw user %q, want operator", rec.Body.String())
	}
}

func TestProtectedRouteRejectsAnonymous(t *testing.T) {
	h := protectedServer(newTestGuard(t))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/private", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestPlatformPathsStayOpen(t *testing.T) {
	g := newTestGuard(t)
	r := mux.NewRouter()
	for _, p := range []string{"/health", "/status", "/metrics", "/r1/producer-health"} {
		r.HandleFunc(p, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	}
	h := g.Wrap(r)

	for _, p := range []string{"/health", "/status", "/metrics", "/r1/producer-health"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200: platform endpoints must not need a session", p, rec.Code)
		}
	}
}

func TestRepeatedFailuresLockTheCaller(t *testing.T) {
	h := protectedServer(newTestGuard(t))

	for i := 0; i < maxLoginFails; i++ {
		resp := login(t, h, `{"username":"operator","password":"wrong"}`)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", i, resp.StatusCode)
		}
	}

	resp := login(t, h, `{"username":"operator","password":"correct-horse"}`)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status after repeated failures = %d, want 429", resp.StatusCode)
	}
}

// Without proxy awareness every caller behind an ingress shares one address,
// so one attacker would lock out every operator at once.
func TestLockoutIsPerForwardedClient(t *testing.T) {
	g := newTestGuard(t, WithTrustedProxy(true))
	h := protectedServer(g)

	attempt := func(forwarded, password string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(
			`{"username":"operator","password":"`+password+`"}`))
		req.RemoteAddr = "10.0.0.9:5000"
		req.Header.Set("X-Forwarded-For", forwarded+", 10.0.0.9")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < maxLoginFails; i++ {
		attempt("203.0.113.5", "wrong")
	}

	if got := attempt("203.0.113.5", "correct-horse"); got != http.StatusTooManyRequests {
		t.Errorf("offending client status = %d, want 429", got)
	}
	if got := attempt("198.51.100.7", "correct-horse"); got != http.StatusOK {
		t.Errorf("innocent client status = %d, want 200", got)
	}
}

func TestLogoutEndsTheSession(t *testing.T) {
	h := protectedServer(newTestGuard(t))

	resp := login(t, h, `{"username":"operator","password":"correct-horse"}`)
	cookie := resp.Cookies()[0]

	out := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	out.AddCookie(cookie)
	h.ServeHTTP(httptest.NewRecorder(), out)

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status after logout = %d, want 401", rec.Code)
	}
}
