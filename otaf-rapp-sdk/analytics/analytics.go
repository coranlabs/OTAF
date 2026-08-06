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

// State is a verdict's name. An rApp defines its own; the only value with a
// meaning here is StateUnknown, which is what an entity has before it has been
// classified.
type State string

const StateUnknown State = "UNKNOWN"

func (s State) String() string { return string(s) }

// Verdict is what a Classifier concluded from a window of samples.
type Verdict struct {
	State State `json:"state"`

	// Score is free for the classifier to use, or leave at zero.
	Score float64 `json:"score,omitempty"`

	// Signals are the intermediate values behind the verdict, carried so an
	// operator can see why rather than only what.
	Signals map[string]float64 `json:"signals,omitempty"`

	// Reason explains the verdict in a line, for logs and for the UI.
	Reason string `json:"reason,omitempty"`
}

// Sample is one observation of one entity.
type Sample[K any] struct {
	At  time.Time `json:"at"`
	KPI K         `json:"kpi"`
}

// Classifier turns a window of samples into a verdict. Implementations belong
// to the rApp: this is the seam between the SDK's plumbing and your judgement.
//
// Classify is called with at least one sample, oldest first, and must not
// retain or modify the slice.
type Classifier[K any] interface {
	Name() string
	Classify(samples []Sample[K]) Verdict
}

// ClassifierFunc adapts a function to Classifier.
type ClassifierFunc[K any] struct {
	Label string
	Fn    func(samples []Sample[K]) Verdict
}

func (c ClassifierFunc[K]) Name() string { return c.Label }

func (c ClassifierFunc[K]) Classify(samples []Sample[K]) Verdict { return c.Fn(samples) }
