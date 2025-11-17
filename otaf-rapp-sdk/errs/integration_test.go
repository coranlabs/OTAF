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
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/a1"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/app"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/auth"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/config"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/influx"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/ingest/kafkasrc"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/r1"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/retry"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/sdnr"
)

func quietLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(os.NewFile(0, os.DevNull))
	l.SetLevel(logrus.PanicLevel)
	return l
}

func serverReturning(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The platform clients classify themselves without importing errs, which is
// the whole point of the small interfaces.
func TestPlatformClientErrorsClassify(t *testing.T) {
	srv := serverReturning(t, http.StatusBadRequest, `{"status":400,"detail":""}`)

	client, err := a1.New(a1.Config{
		Endpoint: srv.URL, ServiceID: "test-rapp",
	}, quietLogger(), a1.WithRetry(retry.None()))
	if err != nil {
		t.Fatal(err)
	}

	failure := client.PutPolicy(context.Background(), a1.Policy{
		ID: "p1", RicID: "ric-1", PolicyTypeID: "20100",
	})
	if failure == nil {
		t.Fatal("expected the policy to be refused")
	}

	if got := errs.CategoryOf(failure); got != errs.CategoryPlatform {
		t.Errorf("category = %s, want platform", got)
	}
	if got := errs.CodeOf(failure); got != "A1_REJECTED" {
		t.Errorf("code = %q, want A1_REJECTED", got)
	}
	if got := errs.StatusOf(failure); got != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
	if errs.IsCritical(failure) {
		t.Error("one refused policy is not critical")
	}
}

func TestConsumerErrorsClassify(t *testing.T) {
	srv := serverReturning(t, http.StatusNotFound, `{"detail":"Information type not found"}`)

	consumer, err := r1.NewConsumer(r1.ConsumerConfig{
		Endpoint: srv.URL, Owner: "test-rapp", SelfURL: "http://test-rapp:8080",
	}, quietLogger())
	if err != nil {
		t.Fatal(err)
	}

	failure := consumer.Subscribe(context.Background(), r1.Subscription{
		JobID: "job-1", InfoTypeID: "absent", DeliverTo: "/data",
	})
	if failure == nil {
		t.Fatal("expected the subscription to be refused")
	}

	if got := errs.CategoryOf(failure); got != errs.CategoryPlatform {
		t.Errorf("category = %s, want platform", got)
	}
	if got := errs.CodeOf(failure); got != "R1_NOT_FOUND" {
		t.Errorf("code = %q, want R1_NOT_FOUND", got)
	}
}

// Reaching the managed network is a different kind of failure from a platform
// service refusing, and the taxonomy has to keep them apart.
func TestControllerErrorsClassifyAsNetwork(t *testing.T) {
	srv := serverReturning(t, http.StatusConflict, "value out of range")

	client, err := sdnr.New(sdnr.Config{
		Endpoint: srv.URL, NodeID: "gnb-1", Username: "admin",
	}, quietLogger())
	if err != nil {
		t.Fatal(err)
	}

	failure := client.Patch(context.Background(), client.MountPath("x"), []byte(`{}`))
	if failure == nil {
		t.Fatal("expected the write to be refused")
	}

	if got := errs.CategoryOf(failure); got != errs.CategoryNetwork {
		t.Errorf("category = %s, want network", got)
	}
	if got := errs.CodeOf(failure); got != "O1_CONFLICT" {
		t.Errorf("code = %q, want O1_CONFLICT", got)
	}
	if !sdnr.IsRejected(failure) {
		t.Error("a 409 is the node refusing, not the node being away")
	}
	if sdnr.IsUnreachable(failure) {
		t.Error("a request that got an answer was not unreachable")
	}
}

// A controller that never answered is a different problem, and worth retrying.
func TestUnreachableControllerIsRetryable(t *testing.T) {
	client, err := sdnr.New(sdnr.Config{
		Endpoint: "http://127.0.0.1:1", NodeID: "gnb-1", Timeout: 100 * time.Millisecond,
	}, quietLogger())
	if err != nil {
		t.Fatal(err)
	}

	failure := client.Ping(context.Background())
	if failure == nil {
		t.Fatal("expected the ping to fail")
	}

	if !sdnr.IsUnreachable(failure) {
		t.Error("a request that never got an answer should classify as unreachable")
	}
	if got := errs.CodeOf(failure); got != "O1_UNREACHABLE" {
		t.Errorf("code = %q, want O1_UNREACHABLE", got)
	}
	if !retry.Retryable(failure) {
		t.Error("an unreachable controller is worth another attempt")
	}
}
