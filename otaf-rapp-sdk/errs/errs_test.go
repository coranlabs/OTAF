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

type stated struct{ retryable bool }
