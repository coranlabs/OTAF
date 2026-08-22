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

package xapptest_test

import (
	"context"
	"testing"
	"time"

	"github.com/coranlabs/OTAF/otaf-xapp-sdk/appmgr"
	"github.com/coranlabs/OTAF/otaf-xapp-sdk/e2"
	"github.com/coranlabs/OTAF/otaf-xapp-sdk/errs"
	"github.com/coranlabs/OTAF/otaf-xapp-sdk/xapptest"
)

func descriptor() appmgr.Descriptor {
	return appmgr.Descriptor{
		Name: "demo-xapp", Version: "1.0.0",
		HTTPPort: 8080, RMRDataPort: 4560, RMRRoutePort: appmgr.RoutePort,
		RxMessages: []string{"RIC_INDICATION"},
		TxMessages: []string{"RIC_SUB_REQ"},
	}
}

func TestPlatformRecordsWhatWasRegistered(t *testing.T) {
	client, platform := xapptest.NewPlatform(t)

	if err := client.Register(context.Background(), descriptor()); err != nil {
		t.Fatal(err)
	}
	got := platform.Registrations()
	if len(got) != 1 {
		t.Fatalf("registrations = %d, want 1", len(got))
	}
	if got[0].Name != "demo-xapp" {
		t.Errorf("name = %q", got[0].Name)
	}
	if got[0].RMREndpoint != "service-ricxapp-demo-xapp-rmr.ricxapp:4560" {
		t.Errorf("rmr endpoint = %q", got[0].RMREndpoint)
	}
	if got[0].Descriptor["xapp_name"] != "demo-xapp" {
		t.Errorf("descriptor did not survive the second decoding: %v", got[0].Descriptor)
	}
}

func TestPlatformRecordsDeregistration(t *testing.T) {
	client, platform := xapptest.NewPlatform(t)

	if err := client.Deregister(context.Background(), "demo-xapp"); err != nil {
		t.Fatal(err)
	}
	if got := platform.Deregistrations(); len(got) != 1 || got[0] != "demo-xapp" {
		t.Errorf("deregistrations = %v", got)
	}
}

// An xApp has to tell a platform that is refusing it apart from one that is not
// answering, because only one of them is worth another attempt.
func TestPlatformCanRefuse(t *testing.T) {
	client, platform := xapptest.NewPlatform(t)

	platform.Refuse(422)
	err := client.Register(context.Background(), descriptor())
	if err == nil {
		t.Fatal("want a failure")
	}
	if got := errs.CategoryOf(err); got != errs.CategoryConfig {
		t.Errorf("category = %q, want config", got)
	}

	platform.Refuse(503)
	err = client.Register(context.Background(), descriptor())
	if got := errs.CategoryOf(err); got != errs.CategoryPlatform {
		t.Errorf("category = %q, want platform", got)
	}

	platform.Refuse(0)
	if err := client.Register(context.Background(), descriptor()); err != nil {
		t.Errorf("still refusing after being told to stop: %v", err)
	}
}

// The samples came off a real RAN, so a test that reads them is exercising the
// same bytes an xApp meets in a cluster.
func TestSamplesDecode(t *testing.T) {
	fn, err := e2.DecodeRANFunction(xapptest.RANFunction(t))
	if err != nil {
		t.Fatalf("ran function: %v", err)
	}
	if fn.ShortName != "ORAN-E2SM-KPM" {
		t.Errorf("short name = %q", fn.ShortName)
	}

	ind, err := e2.DecodeIndication(xapptest.Indication(t))
	if err != nil {
		t.Fatalf("indication: %v", err)
	}
	report, err := e2.DecodeIndicationMessage(ind.Message)
	if err != nil {
		t.Fatalf("measurements: %v", err)
	}
	if report.Period != 10*time.Second {
		t.Errorf("period = %v, want 10s", report.Period)
	}
	if len(report.Latest()) == 0 {
		t.Error("no measurements")
	}
}
