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

package analytics

import (
	"math"
	"sort"
)

func Mean(values []float64) float64 {
	var sum float64
	var n int
	for _, v := range values {
		if math.IsNaN(v) {
			continue
		}
		sum += v
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func Min(values []float64) float64 {
	out, seen := 0.0, false
	for _, v := range values {
		if math.IsNaN(v) {
			continue
		}
		if !seen || v < out {
			out, seen = v, true
		}
	}
	return out
}
