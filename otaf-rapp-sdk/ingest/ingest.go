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

// Package ingest moves data from the interfaces an rApp subscribes to into the
// rApp's own logic. Sources produce messages, one Handler consumes them.
package ingest

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/log"
)

type Message struct {
	Source   string
	Key      string
	Payload  []byte
	Received time.Time
}

type HandlerFunc func(ctx context.Context, m Message) error

const (
	// OverflowBlock applies backpressure to the source. Correct whenever
	// losing a report would corrupt the rApp's view of the network.
	OverflowBlock Overflow = iota
	// OverflowDrop keeps the newest data flowing and counts the losses.
	OverflowDrop
)
