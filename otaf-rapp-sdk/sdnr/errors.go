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

package sdnr

import (
	"errors"
	"fmt"
	"net/http"
)

type Error struct {
	Method string
	Path   string

	// Status is zero when the request never reached the controller.
	Status int
	Detail string
	Cause  error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("sdnr %s %s: %v", e.Method, e.Path, e.Cause)
	}
	return fmt.Sprintf("sdnr %s %s returned %d: %s", e.Method, e.Path, e.Status, e.Detail)
}

func (e *Error) Unwrap() error { return e.Cause }

// Retryable reports whether another attempt could succeed. A node that refused
// the request will refuse it again; one that could not be reached might not be
// unreachable for long.
//
// Note that this says nothing about whether retrying is *safe*. A write that
// is not idempotent should not be retried whatever this reports.
func (e *Error) Retryable() bool {
	if e.Status == 0 {
		return true
	}
	switch e.Status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	}
	return e.Status >= 500
}

// The three methods below let errs classify this failure without either
// package importing the other.
