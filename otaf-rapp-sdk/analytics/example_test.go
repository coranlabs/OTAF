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

package analytics_test

import (
	"fmt"
	"time"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/analytics"
)

type load struct {
	Used float64
}

// A deliberately simple classifier. The SDK ships none: deciding what the
// numbers mean is the whole of an rApp's value, and none of the SDK's.
type busy struct{ limit float64 }

func (busy) Name() string { return "busy" }

func (b busy) Classify(samples []analytics.Sample[load]) analytics.Verdict {
	used := make([]float64, len(samples))
	for i, s := range samples {
		used[i] = s.KPI.Used
	}

	latest := analytics.Last(used)
	verdict := analytics.Verdict{
		Score: latest,
		Signals: map[string]float64{
			"latest": latest,
			"mean":   analytics.Mean(used),
			"slope":  analytics.Slope(used),
		},
	}

	if latest > b.limit {
		verdict.State = "BUSY"
		verdict.Reason = fmt.Sprintf("used %.0f is over %.0f", latest, b.limit)
		return verdict
	}
	verdict.State = "QUIET"
	return verdict
}

// A record of something the rApp did, kept for an operator to look at.
type action struct {
	At     time.Time
	Entity string
	Reason string
}

// The pieces fit together the same way in every rApp: observe, decide, guard,
// act, record, count.
func Example() {
	at := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

	entities := analytics.NewRegistry(
		analytics.WithClassifier[load](busy{limit: 80}),
		analytics.WithHistorySize[load](8),
		analytics.WithStaleAfter[load](5*time.Minute),
	)
	journal := analytics.NewJournal[action](32)
	guard := analytics.NewCooldown(2 * time.Minute)
	counts := analytics.NewBuckets(time.Hour, 24)

	readings := []float64{40, 55, 72, 88, 91, 93}

	for i, used := range readings {
		when := at.Add(time.Duration(i) * time.Minute)

		result := entities.Observe("cell-1", when, load{Used: used})
		if !result.Accepted {
			continue
		}

		// Act on the verdict, not on every report, and not twice in a row.
		if result.Verdict.State == "BUSY" && guard.Take("cell-1", when) {
			journal.Append(action{At: when, Entity: result.Entity, Reason: result.Verdict.Reason})
			counts.Incr(when, "acted")
		}
	}

	fmt.Println("state:  ", must(entities.Verdict("cell-1")).State)
	fmt.Println("actions:", journal.Len())
	for _, a := range journal.Entries() {
		fmt.Println("  -", a.Entity, a.Reason)
	}

	// Output:
	// state:   BUSY
	// actions: 2
	//   - cell-1 used 88 is over 80
	//   - cell-1 used 93 is over 80
}

func must[T any](v T, _ bool) T { return v }
