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

package errs

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestCategoryDrivesTheDefaults(t *testing.T) {
	cases := map[Category]struct {
		severity  Severity
		status    int
		retryable bool
	}{
		CategoryConfig:   {SeverityCritical, http.StatusInternalServerError, false},
		CategoryPlatform: {SeverityError, http.StatusBadGateway, true},
		CategoryNetwork:  {SeverityWarning, http.StatusBadGateway, true},
		CategoryData:     {SeverityWarning, http.StatusBadRequest, false},
		CategoryInternal: {SeverityError, http.StatusInternalServerError, false},
	}

	for category, want := range cases {
		t.Run(string(category), func(t *testing.T) {
			err := New(category, "CODE", "something went wrong")

			if err.Severity != want.severity {
				t.Errorf("severity = %s, want %s", err.Severity, want.severity)
			}
			if err.Status != want.status {
				t.Errorf("status = %d, want %d", err.Status, want.status)
			}
			if err.Retryable() != want.retryable {
				t.Errorf("retryable = %v, want %v", err.Retryable(), want.retryable)
			}
		})
	}
}

func TestUnknownCategoryFallsBack(t *testing.T) {
	err := New(Category("invented"), "CODE", "message")
	if err.Category != CategoryUnknown {
		t.Errorf("category = %s, want %s", err.Category, CategoryUnknown)
	}
}

func TestErrorText(t *testing.T) {
	cause := errors.New("connection refused")

	cases := map[string]struct {
		err  *Error
		want string
	}{
		"code and message": {New(CategoryData, "BAD_INPUT", "payload is malformed"),
			"BAD_INPUT: payload is malformed"},
		"wrapped": {Wrap(cause, CategoryNetwork, "O1_UNREACHABLE", "could not reach the node"),
			"O1_UNREACHABLE: could not reach the node: connection refused"},
		"no code": {New(CategoryInternal, "", "something broke"), "something broke"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Classifying a failure must not hide what caused it.
func TestWrappedCauseStaysReachable(t *testing.T) {
	sentinel := errors.New("the real problem")
	err := Wrap(sentinel, CategoryPlatform, "PLATFORM_DOWN", "policy service refused")

	if !errors.Is(err, sentinel) {
		t.Error("the cause should stay reachable through errors.Is")
	}
	if !errors.Is(errors.Unwrap(err), sentinel) {
		t.Error("Unwrap should return the cause")
	}
}

func TestWrapNilIsNil(t *testing.T) {
	if err := Wrap(nil, CategoryData, "CODE", "message"); err != nil {
		t.Errorf("wrapping nothing should give nothing, got %v", err)
	}
}

// A caller should be able to test for a specific failure without holding the
// value that produced it.
func TestIsMatchesOnCode(t *testing.T) {
	err := New(CategoryData, "SCHEMA_MISMATCH", "does not fit the schema")

	if !errors.Is(err, New(CategoryData, "SCHEMA_MISMATCH", "")) {
		t.Error("errors.Is should match on code")
	}
	if errors.Is(err, New(CategoryData, "OTHER_CODE", "")) {
		t.Error("a different code should not match")
	}
	if !errors.Is(err, New(CategoryData, "", "")) {
		t.Error("with no code to compare, the category should match")
	}
}

func TestHasCodeSearchesTheChain(t *testing.T) {
	inner := New(CategoryData, "INNER", "inner problem")
	outer := Wrap(inner, CategoryPlatform, "OUTER", "outer problem")

	if !HasCode(outer, "OUTER") {
		t.Error("the outer code should be found")
	}
	if !HasCode(outer, "INNER") {
		t.Error("a wrapped code should be found")
	}
	if HasCode(outer, "ABSENT") {
		t.Error("a code that is not there should not be found")
	}
	if HasCode(outer, "") {
		t.Error("an empty code should never match")
	}
}

// An error that already stated its retryability knows better than the category
// default does.
func TestWrapKeepsAStatedRetryability(t *testing.T) {
	permanent := &stated{retryable: false}

	err := Wrap(permanent, CategoryPlatform, "CODE", "message")
	if err.Retryable() {
		t.Error("wrapping a permanent failure as a platform one should stay permanent")
	}

	transient := &stated{retryable: true}
	if !Wrap(transient, CategoryData, "CODE", "message").Retryable() {
		t.Error("wrapping a transient failure as data should stay transient")
	}
}

func TestTransientAndPermanentOverrideTheDefault(t *testing.T) {
	if !New(CategoryData, "C", "m").Transient().Retryable() {
		t.Error("Transient should override the category default")
	}
	if New(CategoryPlatform, "C", "m").Permanent().Retryable() {
		t.Error("Permanent should override the category default")
	}
}

// Classification has to work on errors that know nothing about this package.
func TestClassifyingForeignErrors(t *testing.T) {
	plain := errors.New("something happened")

	if got := CategoryOf(plain); got != CategoryUnknown {
		t.Errorf("category = %s, want %s", got, CategoryUnknown)
	}
	if got := SeverityOf(plain); got != SeverityError {
		t.Errorf("severity = %s, want %s", got, SeverityError)
	}
	if got := StatusOf(plain); got != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", got)
	}
	if got := CodeOf(plain); got != "" {
		t.Errorf("code = %q, want empty", got)
	}
}

func TestClassifyingNil(t *testing.T) {
	if CategoryOf(nil) != "" {
		t.Error("no error has no category")
	}
	if StatusOf(nil) != http.StatusOK {
		t.Error("no error is a success")
	}
	if IsCritical(nil) {
		t.Error("no error is not critical")
	}
}

// An error from any package classifies itself by implementing the small
// interfaces, without importing this one.
func TestForeignErrorsCanClassifyThemselves(t *testing.T) {
	err := &foreign{category: "platform", code: "A1_REJECTED", status: http.StatusBadRequest}

	if got := CategoryOf(err); got != CategoryPlatform {
		t.Errorf("category = %s, want platform", got)
	}
	if got := CodeOf(err); got != "A1_REJECTED" {
		t.Errorf("code = %q", got)
	}
	if got := StatusOf(err); got != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
	// It said nothing about severity, so the category decides.
	if got := SeverityOf(err); got != SeverityError {
		t.Errorf("severity = %s, want the platform default", got)
	}
}

func TestClassificationSeesThroughWrapping(t *testing.T) {
	foreign := &foreign{category: "network", code: "O1_UNREACHABLE"}
	wrapped := fmt.Errorf("while applying the decision: %w", foreign)

	if got := CategoryOf(wrapped); got != CategoryNetwork {
		t.Errorf("category = %s, want network through the wrapper", got)
	}
	if got := CodeOf(wrapped); got != "O1_UNREACHABLE" {
		t.Errorf("code = %q, want it found through the wrapper", got)
	}
}

type stated struct{ retryable bool }
