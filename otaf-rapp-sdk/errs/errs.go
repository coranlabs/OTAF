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

// Category is where a failure came from, which is what decides what to do
// about it.
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
func (e *Error) HTTPStatus() int       { return e.Status }

// Transient marks a failure as worth another attempt, against the default for
// its category.
func (e *Error) Transient() *Error {
	e.retryable = true
	return e
}

// Permanent marks a failure as not worth retrying.
func (e *Error) Permanent() *Error {
	e.retryable = false
	return e
}

func (e *Error) WithSeverity(s Severity) *Error {
	e.Severity = s
	return e
}

func (e *Error) WithStatus(status int) *Error {
	e.Status = status
	return e
}

// WithField attaches structured detail, for logs rather than for control flow.
func (e *Error) WithField(key string, value any) *Error {
	if e.Fields == nil {
		e.Fields = map[string]any{}
	}
	e.Fields[key] = value
	return e
}

func (e *Error) WithFields(fields map[string]any) *Error {
	for k, v := range fields {
		e = e.WithField(k, v)
	}
	return e
}

// The interfaces below are how an error from any package classifies itself
// without depending on this one. Implement whichever apply.
type (
	categorized interface{ ErrorCategory() string }
	coded       interface{ ErrorCode() string }
	severities  interface{ ErrorSeverity() string }
	statused    interface{ HTTPStatus() int }
	retryable   interface{ Retryable() bool }
	fielded     interface{ ErrorFields() map[string]any }
)

// CategoryOf classifies any error. An error that says nothing about itself is
// unknown rather than assumed harmless.
func CategoryOf(err error) Category {
	if err == nil {
		return ""
	}
	var c categorized
	if errors.As(err, &c) {
		if category := Category(c.ErrorCategory()); category != "" {
			return category
		}
	}
	return CategoryUnknown
}

// SeverityOf reports how much attention a failure deserves, from what it says
// about itself or from its category.
func SeverityOf(err error) Severity {
	if err == nil {
		return ""
	}
	var s severities
	if errors.As(err, &s) {
		if severity := Severity(s.ErrorSeverity()); severity != "" {
			return severity
		}
	}
	return byCategory[CategoryOf(err)].severity
}

// CodeOf returns the stable identifier, or "" when the error has none.
func CodeOf(err error) string {
	if err == nil {
		return ""
	}
	var c coded
	if errors.As(err, &c) {
		return c.ErrorCode()
	}
	return ""
}

// StatusOf is what to answer if this failure surfaces through an HTTP API.
func StatusOf(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var s statused
	if errors.As(err, &s) {
		if status := s.HTTPStatus(); status > 0 {
			return status
		}
	}
	return byCategory[CategoryOf(err)].status
}

// FieldsOf collects structured detail for logging.
func FieldsOf(err error) map[string]any {
	if err == nil {
		return nil
	}
	var f fielded
	if errors.As(err, &f) {
		return f.ErrorFields()
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Fields
	}
	return nil
}

// ErrorFields exposes the attached detail through the shared interface.
func (e *Error) ErrorFields() map[string]any { return e.Fields }

// IsCategory reports whether a failure came from where you think it did.
func IsCategory(err error, category Category) bool { return CategoryOf(err) == category }

// IsCritical marks a failure someone should be told about now.
func IsCritical(err error) bool { return SeverityOf(err) == SeverityCritical }

// HasCode reports whether this failure, or anything it wraps, carries a code.
func HasCode(err error, code string) bool {
	if code == "" {
		return false
	}
	for err != nil {
		if CodeOf(err) == code {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}

// LogFields is what to hand a structured logger: the classification plus
// whatever detail the failure carried.
func LogFields(err error) map[string]any {
	if err == nil {
		return nil
	}
	out := map[string]any{
		"category": string(CategoryOf(err)),
		"severity": string(SeverityOf(err)),
	}
	if code := CodeOf(err); code != "" {
		out["code"] = code
	}
	for k, v := range FieldsOf(err) {
		if _, taken := out[k]; !taken {
			out[k] = v
		}
	}
	return out
}

// Categories lists the vocabulary, for anything that needs to enumerate it.
func Categories() []Category {
	out := make([]Category, 0, len(byCategory))
	for category := range byCategory {
		out = append(out, category)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func statedRetryable(err error) (bool, bool) {
	var r retryable
	if errors.As(err, &r) {
		return r.Retryable(), true
	}
	return false, false
}
