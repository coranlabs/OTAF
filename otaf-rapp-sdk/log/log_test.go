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

package log

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
)

func capture(t *testing.T) (*logrus.Logger, func() map[string]any) {
	t.Helper()

	buf := &bytes.Buffer{}
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetOutput(buf)
	logger.SetLevel(logrus.DebugLevel)

	return logger, func() map[string]any {
		var record map[string]any
		if buf.Len() == 0 {
			return nil
		}
		if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
			t.Fatalf("log line is not JSON: %v: %s", err, buf.String())
		}
		return record
	}
}

// How loud a failure is should follow from what kind of failure it is, not
// from whichever level the call site happened to pick.
func TestSeverityPicksTheLevel(t *testing.T) {
	cases := map[errs.Category]string{
		errs.CategoryNetwork:  "warning",
		errs.CategoryData:     "warning",
		errs.CategoryPlatform: "error",
		errs.CategoryInternal: "error",
		errs.CategoryConfig:   "error",
	}

	for category, wantLevel := range cases {
		t.Run(string(category), func(t *testing.T) {
			logger, read := capture(t)
			Failure(logger, errs.New(category, "CODE", "something failed"), "could not proceed")

			record := read()
			if record["level"] != wantLevel {
				t.Errorf("level = %v, want %v", record["level"], wantLevel)
			}
		})
	}
}

// A library that called Fatal would take the decision to stop away from the
// rApp, which is the only thing that knows whether it can carry on.
func TestCriticalDoesNotKillTheProcess(t *testing.T) {
	logger, read := capture(t)

	misconfigured := errs.New(errs.CategoryConfig, "CONFIG_MISSING", "no config file")
	if !errs.IsCritical(misconfigured) {
		t.Fatal("expected a config failure to be critical")
	}

	Failure(logger, misconfigured, "cannot start")

	if record := read(); record["level"] != "error" {
		t.Errorf("level = %v, want error rather than fatal", record["level"])
	}
}

func TestClassificationIsAttached(t *testing.T) {
	logger, read := capture(t)

	failure := errs.New(errs.CategoryPlatform, "A1_REJECTED", "policy refused").
		WithField("policy", "p1")
	Failure(logger, failure, "could not apply the decision")

	record := read()
	if record["category"] != "platform" {
		t.Errorf("category = %v", record["category"])
	}
	if record["code"] != "A1_REJECTED" {
		t.Errorf("code = %v", record["code"])
	}
	if record["severity"] != "error" {
		t.Errorf("severity = %v", record["severity"])
	}
	if record["policy"] != "p1" {
		t.Errorf("attached detail should survive, got %v", record)
	}
	if record["error"] == nil {
		t.Error("the failure itself should still be in the record")
	}
	if record["msg"] != "could not apply the decision" {
		t.Errorf("msg = %v", record["msg"])
	}
}
