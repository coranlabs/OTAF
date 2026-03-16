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

package r1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeICS struct {
	mu       sync.Mutex
	jobs     map[string]consumerJob
	types    map[string]bool
	producer int
}

func newFakeICS(types ...string) *fakeICS {
	f := &fakeICS{jobs: map[string]consumerJob{}, types: map[string]bool{}}
	for _, t := range types {
		f.types[t] = true
	}
	return f
}

func (f *fakeICS) addType(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.types[id] = true
}

func (f *fakeICS) jobCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.jobs)
}

func (f *fakeICS) job(id string) (consumerJob, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	return j, ok
}
