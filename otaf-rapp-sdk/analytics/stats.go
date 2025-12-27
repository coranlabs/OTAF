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

func Max(values []float64) float64 {
	out, seen := 0.0, false
	for _, v := range values {
		if math.IsNaN(v) {
			continue
		}
		if !seen || v > out {
			out, seen = v, true
		}
	}
	return out
}

// Last is the most recent value, which is usually what a threshold compares.
func Last(values []float64) float64 {
	for i := len(values) - 1; i >= 0; i-- {
		if !math.IsNaN(values[i]) {
			return values[i]
		}
	}
	return 0
}

// Percentile interpolates between the surrounding values, with p from 0 to 1.
// The input is not modified.
func Percentile(values []float64, p float64) float64 {
	clean := make([]float64, 0, len(values))
	for _, v := range values {
		if !math.IsNaN(v) {
			clean = append(clean, v)
		}
	}
	if len(clean) == 0 {
		return 0
	}
	sort.Float64s(clean)

	switch {
	case p <= 0:
		return clean[0]
	case p >= 1:
		return clean[len(clean)-1]
	}

	pos := p * float64(len(clean)-1)
	lower := int(math.Floor(pos))
	upper := int(math.Ceil(pos))
	if lower == upper {
		return clean[lower]
	}
	return clean[lower] + (clean[upper]-clean[lower])*(pos-float64(lower))
}

// StdDev is the population standard deviation.
func StdDev(values []float64) float64 {
	mean := Mean(values)

	var sum float64
	var n int
	for _, v := range values {
		if math.IsNaN(v) {
			continue
		}
		d := v - mean
		sum += d * d
		n++
	}
	if n == 0 {
		return 0
	}
	return math.Sqrt(sum / float64(n))
}
