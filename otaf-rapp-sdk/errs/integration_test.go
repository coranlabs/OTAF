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

// A misconfigured rApp cannot work until a person fixes it, and nothing about
// that changes on a retry.
func TestConfigErrorsAreCriticalAndPermanent(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.yaml")

	var settings struct {
		Rapp config.Rapp `yaml:"rapp"`
	}
	failure := config.Load(&settings, missing)
	if failure == nil {
		t.Fatal("expected loading a missing config to fail")
	}

	if got := errs.CategoryOf(failure); got != errs.CategoryConfig {
		t.Errorf("category = %s, want config", got)
	}
	if !errs.IsCritical(failure) {
		t.Error("a misconfigured rApp is critical")
	}
	if retry.Retryable(failure) {
		t.Error("retrying will not make the file appear")
	}
	if !errs.HasCode(failure, "CONFIG_MISSING") {
		t.Errorf("code = %q, want CONFIG_MISSING", errs.CodeOf(failure))
	}
}

// Every way an rApp can be set up wrong has to classify the same. They all
// surface through the same path at startup, so one of them reporting
// "unknown" would make an alert on misconfiguration silently incomplete.
func TestEveryStartupMisconfigurationClassifiesTheSame(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.yaml")

	starts := map[string]func() error{
		"no config file": func() error {
			var settings struct {
				Rapp config.Rapp `yaml:"rapp"`
			}
			return config.Load(&settings, missing)
		},
		"no HTTP port": func() error {
			_, err := app.New(config.Rapp{Name: "demo", Version: "1"}, app.WithLogger(quietLogger()))
			return err
		},
		"no controller endpoint": func() error {
			_, err := sdnr.New(sdnr.Config{}, quietLogger())
			return err
		},
		"no A1 service id": func() error {
			_, err := a1.New(a1.Config{Endpoint: "http://pms"}, quietLogger())
			return err
		},
		"no consumer owner": func() error {
			_, err := r1.NewConsumer(r1.ConsumerConfig{
				Endpoint: "http://ics", SelfURL: "http://me",
			}, quietLogger())
			return err
		},
		"no consumer self URL": func() error {
			_, err := r1.NewConsumer(r1.ConsumerConfig{
				Endpoint: "http://ics", Owner: "me",
			}, quietLogger())
			return err
		},
		"store without a bucket": func() error {
			_, err := influx.New(influx.Config{URL: "http://influx", Org: "o"}, quietLogger())
			return err
		},
		"store without an org": func() error {
			_, err := influx.New(influx.Config{URL: "http://influx", Bucket: "b"}, quietLogger())
			return err
		},
		"no kafka brokers": func() error {
			_, err := kafkasrc.New(kafkasrc.Config{Topic: "t"}, quietLogger())
			return err
		},
		"no kafka topic": func() error {
			_, err := kafkasrc.New(kafkasrc.Config{Brokers: []string{"b:9092"}}, quietLogger())
			return err
		},
		"malformed operator account": func() error {
			_, err := auth.NewGuard("operator:not-a-bcrypt-hash", quietLogger())
			return err
		},
	}

	for name, start := range starts {
		t.Run(name, func(t *testing.T) {
			err := start()
			if err == nil {
				t.Fatal("expected this to be refused at startup")
			}

			if got := errs.CategoryOf(err); got != errs.CategoryConfig {
				t.Errorf("category = %s, want config", got)
			}
			if !errs.IsCritical(err) {
				t.Error("a misconfigured rApp cannot work until someone fixes it")
			}
			if retry.Retryable(err) {
				t.Error("retrying will not change the configuration")
			}
			if errs.CodeOf(err) == "" {
				t.Error("the failure should carry a code an operator can search for")
			}
			if errs.StatusOf(err) != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500", errs.StatusOf(err))
			}
		})
	}
}

// A caller passing an incomplete argument is a bug in the rApp, not a
// deployment that was set up wrong, and the two want different attention.
func TestCallerMistakesClassifyAsInternal(t *testing.T) {
	client, err := a1.New(a1.Config{Endpoint: "http://pms", ServiceID: "s"}, quietLogger())
	if err != nil {
		t.Fatal(err)
	}

	mistakes := map[string]error{
		"policy with no id":  client.PutPolicy(context.Background(), a1.Policy{RicID: "ric-1"}),
		"policy with no ric": client.PutPolicy(context.Background(), a1.Policy{ID: "p1"}),
	}

	consumer, err := r1.NewConsumer(r1.ConsumerConfig{
		Endpoint: "http://ics", Owner: "me", SelfURL: "http://me",
	}, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	mistakes["subscription with no job id"] = consumer.Subscribe(context.Background(),
		r1.Subscription{InfoTypeID: "t", DeliverTo: "/data"})

	for name, err := range mistakes {
		t.Run(name, func(t *testing.T) {
			if err == nil {
				t.Fatal("expected this to be refused")
			}
			if got := errs.CategoryOf(err); got != errs.CategoryInternal {
				t.Errorf("category = %s, want internal", got)
			}
			if retry.Retryable(err) {
				t.Error("retrying will not supply the missing field")
			}
		})
	}
}

func TestBadEnvironmentValueIsAConfigFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rapp.yaml")
	if err := os.WriteFile(path, []byte("rapp:\n  name: probe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTTP_PORT", "8080")

	var settings struct {
		Rapp    config.Rapp   `yaml:"rapp"`
		Timeout time.Duration `yaml:"timeout" env:"TEST_ERRS_TIMEOUT"`
	}
	t.Setenv("TEST_ERRS_TIMEOUT", "not-a-duration")

	failure := config.Load(&settings, path)
	if failure == nil {
		t.Fatal("expected an unusable environment value to fail")
	}

	if got := errs.CategoryOf(failure); got != errs.CategoryConfig {
		t.Errorf("category = %s, want config", got)
	}
	fields := errs.FieldsOf(failure)
	if fields["variable"] != "TEST_ERRS_TIMEOUT" {
		t.Errorf("fields = %v, want the offending variable named", fields)
	}
}

// Classification has to survive an rApp adding its own context on the way up.
func TestClassificationSurvivesRewrapping(t *testing.T) {
	srv := serverReturning(t, http.StatusServiceUnavailable, "")

	client, err := a1.New(a1.Config{
		Endpoint: srv.URL, ServiceID: "test-rapp",
	}, quietLogger(), a1.WithRetry(retry.None()))
	if err != nil {
		t.Fatal(err)
	}

	failure := client.PutPolicy(context.Background(), a1.Policy{
		ID: "p1", RicID: "ric-1", PolicyTypeID: "20100",
	})
	wrapped := fmt.Errorf("while relieving cell-1: %w", failure)

	if got := errs.CategoryOf(wrapped); got != errs.CategoryPlatform {
		t.Errorf("category = %s, want platform through the wrapper", got)
	}
	if got := errs.CodeOf(wrapped); got != "A1_UNAVAILABLE" {
		t.Errorf("code = %q, want it found through the wrapper", got)
	}
	if !retry.Retryable(wrapped) {
		t.Error("an unavailable service is worth another attempt, wrapper or not")
	}
}
