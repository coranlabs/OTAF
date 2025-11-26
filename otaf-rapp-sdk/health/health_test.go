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

package health

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func recordingLogger() (*logrus.Logger, *lines) {
	l := logrus.New()
	l.SetLevel(logrus.DebugLevel)
	out := &lines{}
	l.SetOutput(out)
	return l, out
}

type lines struct {
	mu   sync.Mutex
	text strings.Builder
}

type flappy struct {
	mu  sync.Mutex
	err error
}
