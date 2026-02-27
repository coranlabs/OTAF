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

// Package a1 steers the Near-RT RIC. An rApp registers as a service with the
// A1 policy management service, then creates and withdraws policy instances on
// the RICs that support the policy types it cares about.
package a1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	rappsdk "github.com/coranlabs/OTAF/otaf-rapp-sdk"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/retry"
)

const (
	basePath       = "/a1-policy/v2"
	defaultTimeout = 30 * time.Second

	// A service that stops calling keep-alive has its policies withdrawn, so
	// the heartbeat runs well inside the interval it registered.
	keepAliveDivisor = 3
	minKeepAlive     = 30 * time.Second
)
