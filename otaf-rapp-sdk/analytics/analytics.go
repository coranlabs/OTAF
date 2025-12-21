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

// Package analytics holds the machinery an rApp needs around a decision, and
// none of the decision itself.
//
// An rApp that watches something over time keeps per-entity state, a bounded
// window of recent samples, a verdict, a record of what it did and a guard
// against doing it again too soon. That is what this package provides. What a
// verdict means, and which numbers lead to it, is the rApp's own: the SDK
// ships the Classifier interface and no implementations of it.
//
// The package is optional. An rApp that forwards data without judging it has
// no reason to import it.
package analytics

import "time"

type State string
