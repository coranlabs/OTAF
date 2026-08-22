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

package submgr_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coranlabs/OTAF/otaf-xapp-sdk/appmgr"
	"github.com/coranlabs/OTAF/otaf-xapp-sdk/e2"
	"github.com/coranlabs/OTAF/otaf-xapp-sdk/errs"
	"github.com/coranlabs/OTAF/otaf-xapp-sdk/submgr"
	"github.com/coranlabs/OTAF/otaf-xapp-sdk/xapptest"
)

func descriptor() appmgr.Descriptor {
	return appmgr.Descriptor{
		Name: "demo-xapp", Version: "1.0.0",
		HTTPPort: 8080, RMRDataPort: 4560, RMRRoutePort: appmgr.RoutePort,
	}
}

func subscription(t *testing.T) *e2.Subscription {
	t.Helper()
	fn, err := e2.DecodeRANFunction(xapptest.RANFunction(t))
	if err != nil {
		t.Fatal(err)
	}
	style := fn.ReportStyles[0]
	sub, err := fn.Subscribe(e2.Request{
		Style:        style.Type,
		Measurements: []string{style.Measurements[0].Name},
		Period:       10 * time.Second,
		Cell:         &e2.Cell{PLMN: [3]byte{0x00, 0xf1, 0x10}, NRCI: [5]byte{0, 0, 0, 6, 0x70}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

// The response comes back to the name the registration published, so the host
// in a subscription has to be the one routes are built from, not the http
// service beside it.
func TestSubscriptionAnswersToTheRegisteredName(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"SubscriptionId":"abc123"}`)
	}))
	defer srv.Close()

	resp, err := submgr.New(srv.URL).Subscribe(context.Background(), submgr.Request{
		Descriptor: descriptor(), Meid: "gnb_001", RANFunctionID: 2,
		Subscription: subscription(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.SubscriptionID != "abc123" {
		t.Errorf("subscription id = %q", resp.SubscriptionID)
	}

	endpoint, _ := got["ClientEndpoint"].(map[string]any)
	if endpoint["Host"] != "service-ricxapp-demo-xapp-rmr.ricxapp" {
		t.Errorf("host = %v", endpoint["Host"])
	}
	if endpoint["RMRPort"] != float64(4560) || endpoint["HTTPPort"] != float64(8080) {
		t.Errorf("ports = %v", endpoint)
	}
	if got["Meid"] != "gnb_001" || got["RANFunctionID"] != float64(2) {
		t.Errorf("meid = %v, ran function = %v", got["Meid"], got["RANFunctionID"])
	}
}

func TestSubscribeRejectsIncompleteRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("an incomplete request reached the platform")
	}))
	defer srv.Close()
	client := submgr.New(srv.URL)

	bad := descriptor()
	bad.RMRRoutePort = 4571

	cases := map[string]submgr.Request{
		"no subscription": {Descriptor: descriptor(), Meid: "gnb_001"},
		"no RAN named":    {Descriptor: descriptor(), Subscription: subscription(t)},
		"bad descriptor":  {Descriptor: bad, Meid: "gnb_001", Subscription: subscription(t)},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := client.Subscribe(context.Background(), req)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if got := errs.CategoryOf(err); got != errs.CategoryConfig {
				t.Errorf("category = %q, want config", got)
			}
		})
	}
}

func TestSubscribeClassifiesPlatformFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		status   int
		category errs.Category
	}{
		"refused":     {http.StatusBadRequest, errs.CategoryConfig},
		"unavailable": {http.StatusServiceUnavailable, errs.CategoryPlatform},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			_, err := submgr.New(srv.URL).Subscribe(context.Background(), submgr.Request{
				Descriptor: descriptor(), Meid: "gnb_001", Subscription: subscription(t),
			})
			if got := errs.CategoryOf(err); got != tc.category {
				t.Errorf("category = %q, want %q", got, tc.category)
			}
		})
	}
}

func TestUnsubscribe(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.Method + " " + r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := submgr.New(srv.URL).Unsubscribe(context.Background(), "abc123"); err != nil {
		t.Fatal(err)
	}
	if path != "DELETE /ric/v1/subscriptions/abc123" {
		t.Errorf("request = %q", path)
	}
}
