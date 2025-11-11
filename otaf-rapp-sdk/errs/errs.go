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

// Package errs classifies failures so an rApp can act on them without reading
// their text.
//
// Three questions come up about every failure, and a message answers none of
// them: whose fault is it, is it worth another attempt, and should anyone be
// woken up. A category answers the first, and the rest follow from it.
//
// Nothing has to be wrapped. The classification helpers work on any error,
// including the errors the SDK's own clients already return, and fall back to
// sensible defaults for errors that say nothing about themselves. Wrap where
// the extra detail earns its place, and no more.
package errs

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type Category string

const (
	// CategoryConfig means the rApp is misconfigured. No amount of retrying
	// helps; a person has to change something.
	CategoryConfig Category = "config"

	// CategoryPlatform means a platform service refused or was unavailable.
	CategoryPlatform Category = "platform"

	// CategoryNetwork means the managed network refused, or could not be
	// reached through the controller.
	CategoryNetwork Category = "network"

	// CategoryData means the data itself is wrong. Retrying sends the same
	// wrong data again.
	CategoryData Category = "data"

	// CategoryInternal means the rApp did something it should not have. It is
	// a bug, and it is worth saying so plainly.
	CategoryInternal Category = "internal"

	// CategoryUnknown is what an error that says nothing about itself gets.
	CategoryUnknown Category = "unknown"
)

// Severity is how much attention a failure deserves.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

var byCategory = map[Category]defaults{
	CategoryConfig:   {SeverityCritical, http.StatusInternalServerError, false},
	CategoryPlatform: {SeverityError, http.StatusBadGateway, true},
	CategoryNetwork:  {SeverityWarning, http.StatusBadGateway, true},
	CategoryData:     {SeverityWarning, http.StatusBadRequest, false},
	CategoryInternal: {SeverityError, http.StatusInternalServerError, false},
	CategoryUnknown:  {SeverityError, http.StatusInternalServerError, true},
}
