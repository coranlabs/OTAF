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

// Package rapptest exercises an rApp's own logic without a cluster. The fakes
// are real clients pointed at stand-in servers, so the code under test takes
// exactly the path it takes in production.
package rapptest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/ingest"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/r1"
)

func Logger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(discard{})
	l.SetLevel(logrus.PanicLevel)
	return l
}

// VerboseLogger prints, for when a test is being debugged.
func VerboseLogger() *logrus.Logger {
	l := logrus.New()
	l.SetLevel(logrus.DebugLevel)
	return l
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// Message builds an ingest message the way a source would. A []byte payload is
// passed through; anything else is marshalled to JSON.
func Message(t testing.TB, source string, payload any) ingest.Message {
	t.Helper()
	return ingest.Message{
		Source:   source,
		Payload:  encode(t, payload),
		Received: time.Now(),
	}
}

func encode(t testing.TB, payload any) []byte {
	t.Helper()
	switch v := payload.(type) {
	case nil:
		return nil
	case []byte:
		return v
	case string:
		return []byte(v)
	default:
		body, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("rapptest: could not encode payload: %v", err)
		}
		return body
	}
}

// Harness drives one handler the way the pipeline does, without goroutines or
// timing, so assertions run immediately after the call returns.
type Harness struct {
	t       testing.TB
	handler ingest.Handler
	ctx     context.Context
}
