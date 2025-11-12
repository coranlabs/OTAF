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

// defaults keep the common case to one call: naming the category is usually
// enough to say how serious it is, whether to retry, and what to answer over
// HTTP.
type defaults struct {
	severity  Severity
	status    int
	retryable bool
}

var byCategory = map[Category]defaults{
	CategoryConfig:   {SeverityCritical, http.StatusInternalServerError, false},
	CategoryPlatform: {SeverityError, http.StatusBadGateway, true},
	CategoryNetwork:  {SeverityWarning, http.StatusBadGateway, true},
	CategoryData:     {SeverityWarning, http.StatusBadRequest, false},
	CategoryInternal: {SeverityError, http.StatusInternalServerError, false},
	CategoryUnknown:  {SeverityError, http.StatusInternalServerError, true},
}

// Error is a failure that knows what kind of failure it is.
type Error struct {
	Category Category
	Severity Severity

	// Code is a stable identifier, meant to be searched for. It outlives
	// rewording of the message, which is what makes it useful in a ticket.
	Code string

	Message string
	Cause   error

	// Status is what to answer if this failure surfaces through an HTTP API.
	Status int

	retryable bool

	// Fields carry structured detail for logs.
	Fields map[string]any
}

// New builds a failure. Severity, status and retryability follow from the
// category unless you say otherwise.
func New(category Category, code, message string) *Error {
	d, known := byCategory[category]
	if !known {
		category, d = CategoryUnknown, byCategory[CategoryUnknown]
	}
	return &Error{
		Category:  category,
		Severity:  d.severity,
		Code:      code,
		Message:   message,
		Status:    d.status,
		retryable: d.retryable,
	}
}

// Newf is New with a formatted message.
func Newf(category Category, code, format string, args ...any) *Error {
	return New(category, code, fmt.Sprintf(format, args...))
}

// Wrap classifies an existing failure, keeping it reachable through
// errors.Is and errors.As.
func Wrap(err error, category Category, code, message string) *Error {
	if err == nil {
		return nil
	}
	out := New(category, code, message)
	out.Cause = err

	// An error that already said whether it was worth retrying knows better
	// than the category default does.
	if stated, ok := statedRetryable(err); ok {
		out.retryable = stated
	}
	return out
}

func Wrapf(err error, category Category, code, format string, args ...any) *Error {
	return Wrap(err, category, code, fmt.Sprintf(format, args...))
}

func (e *Error) Error() string {
	var b strings.Builder
	if e.Code != "" {
		b.WriteString(e.Code)
		b.WriteString(": ")
	}
	b.WriteString(e.Message)
	if e.Cause != nil {
		if e.Message != "" {
			b.WriteString(": ")
		}
		b.WriteString(e.Cause.Error())
	}
	return b.String()
}

func (e *Error) Unwrap() error { return e.Cause }

// Is matches on code, so a caller can test for a specific failure without
// holding on to the value that produced it.
func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	if !ok {
		return false
	}
	if other.Code != "" && e.Code != "" {
		return other.Code == e.Code
	}
	return other.Category != "" && other.Category == e.Category
}

// Retryable satisfies what retry looks for, so a classified failure drives
// retrying without anything else being told about it.
func (e *Error) Retryable() bool { return e.retryable }

// ErrorCategory, ErrorCode and HTTPStatus let other packages classify their
// own errors without importing this one.
func (e *Error) ErrorCategory() string { return string(e.Category) }

func (e *Error) ErrorCode() string     { return e.Code }

func (e *Error) ErrorSeverity() string { return string(e.Severity) }
