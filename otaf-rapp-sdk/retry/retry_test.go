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

package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func fast() Policy {
	return Policy{Attempts: 4, Initial: time.Millisecond, Max: 4 * time.Millisecond, Multiplier: 2}
}

type statusError struct {
	status int
}

func (e *statusError) Error() string   { return "status error" }

func (e *statusError) Retryable() bool { return e.status >= 500 }

func TestSucceedsWithoutRetrying(t *testing.T) {
	var calls int
	err := Do(context.Background(), fast(), func(context.Context, int) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestRetriesUntilItWorks(t *testing.T) {
	var calls int
	err := Do(context.Background(), fast(), func(context.Context, int) error {
		calls++
		if calls < 3 {
			return errors.New("not yet")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestGivesUpAfterTheLastAttempt(t *testing.T) {
	var calls int
	sentinel := errors.New("still failing")

	err := Do(context.Background(), fast(), func(context.Context, int) error {
		calls++
		return sentinel
	})

	if calls != 4 {
		t.Errorf("calls = %d, want 4", calls)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("the real cause should stay reachable, got %v", err)
	}

	var exhausted *ExhaustedError
	if !errors.As(err, &exhausted) || exhausted.Attempts != 4 {
		t.Errorf("expected an exhausted error naming 4 attempts, got %v", err)
	}
}

// Retrying a request the service refused on its own merits only wastes time.
func TestPermanentErrorStopsImmediately(t *testing.T) {
	var calls int
	err := Do(context.Background(), fast(), func(context.Context, int) error {
		calls++
		return Permanent(errors.New("malformed"))
	})

	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
	if err == nil {
		t.Fatal("expected the error to be returned")
	}
	var exhausted *ExhaustedError
	if errors.As(err, &exhausted) {
		t.Error("a permanent failure is not an exhausted retry")
	}
}

func TestErrorsDecideForThemselves(t *testing.T) {
	cases := map[string]struct {
		status    int
		wantCalls int
	}{
		"server error is retried":     {503, 4},
		"client error is not retried": {400, 1},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var calls int
			_ = Do(context.Background(), fast(), func(context.Context, int) error {
				calls++
				return &statusError{status: tc.status}
			})
			if calls != tc.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tc.wantCalls)
			}
		})
	}
}

func TestUnclassifiedErrorsAreRetried(t *testing.T) {
	if !Retryable(errors.New("connection refused")) {
		t.Error("a bare error should be retried; a dropped connection usually is worth another go")
	}
	if Retryable(nil) {
		t.Error("no error is not retryable")
	}
}

func TestContextCancellationStopsRetrying(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var calls int
	err := Do(ctx, Policy{Attempts: 10, Initial: 50 * time.Millisecond, Multiplier: 1}, func(context.Context, int) error {
		calls++
		if calls == 2 {
			cancel()
		}
		return errors.New("failing")
	})

	if err == nil {
		t.Fatal("expected an error once the context ended")
	}
	if calls > 3 {
		t.Errorf("calls = %d, want the loop to stop promptly after cancellation", calls)
	}
}

func TestNoneDisablesRetrying(t *testing.T) {
	var calls int
	_ = Do(context.Background(), None(), func(context.Context, int) error {
		calls++
		return errors.New("failing")
	})
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}
