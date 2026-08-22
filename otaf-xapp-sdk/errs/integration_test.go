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

package errs_test

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coranlabs/OTAF/otaf-xapp-sdk/appmgr"
	"github.com/coranlabs/OTAF/otaf-xapp-sdk/e2"
	"github.com/coranlabs/OTAF/otaf-xapp-sdk/errs"
	"github.com/coranlabs/OTAF/otaf-xapp-sdk/retry"
)

func descriptor() appmgr.Descriptor {
	return appmgr.Descriptor{
		Name: "otaf-probe", Version: "1.0.0",
		HTTPPort: 8098, RMRDataPort: 4560, RMRRoutePort: appmgr.RoutePort,
	}
}

func ranFunction(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../e2/testdata/ranfunction-cucp.hex")
	if err != nil {
		t.Fatal(err)
	}
	b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A failure carries its category across package boundaries without either side
// importing the other, so retrying, logging and alerting all follow one label.
func TestFailuresClassifyAcrossPackages(t *testing.T) {
	unreachable := httptest.NewServer(nil)
	unreachable.Close()

	rejecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer rejecting.Close()

	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer unavailable.Close()

	bad := descriptor()
	bad.Version = "not-a-version"

	fn, err := e2.DecodeRANFunction(ranFunction(t))
	if err != nil {
		t.Fatal(err)
	}
	_, unknownStyle := fn.Subscribe(e2.Request{
		Style: 99, Measurements: []string{"RRC.ConnMean"}, Period: time.Second,
	})
	_, badPayload := e2.DecodeIndication([]byte{0xde, 0xad})

	cases := map[string]struct {
		err       error
		category  errs.Category
		retryable bool
	}{
		"descriptor the platform would reject": {
			bad.Validate(), errs.CategoryConfig, false,
		},
		"platform not answering": {
			appmgr.New(unreachable.URL).Register(context.Background(), descriptor()),
			errs.CategoryPlatform, true,
		},
		"platform refusing the descriptor": {
			appmgr.New(rejecting.URL).Register(context.Background(), descriptor()),
			errs.CategoryConfig, false,
		},
		"platform temporarily unavailable": {
			appmgr.New(unavailable.URL).Register(context.Background(), descriptor()),
			errs.CategoryPlatform, true,
		},
		"style the RAN does not offer": {
			unknownStyle, errs.CategoryConfig, false,
		},
		"payload that is not an indication": {
			badPayload, errs.CategoryData, false,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if c.err == nil {
				t.Fatal("expected a failure, got nil")
			}
			if got := errs.CategoryOf(c.err); got != c.category {
				t.Errorf("category = %q, want %q (%v)", got, c.category, c.err)
			}
			if got := retry.Retryable(c.err); got != c.retryable {
				t.Errorf("retryable = %v, want %v (%v)", got, c.retryable, c.err)
			}
			if errs.CodeOf(c.err) == "" {
				t.Errorf("no code on %v", c.err)
			}
		})
	}
}

// Registration is worth retrying when the platform is not answering yet, which
// is the ordinary case while a cluster is still coming up.
func TestRetryGivesUpOnAConfigFailure(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	client := appmgr.New(srv.URL)
	err := retry.Do(context.Background(), retry.Default(), func(ctx context.Context, _ int) error {
		return client.Register(ctx, descriptor())
	})
	if err == nil {
		t.Fatal("expected a failure")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1: a rejected descriptor does not improve with repetition", attempts)
	}
}
