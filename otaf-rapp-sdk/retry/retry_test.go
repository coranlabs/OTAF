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
