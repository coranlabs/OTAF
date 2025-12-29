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

// Slope is the least-squares gradient per sample: positive when the values are
// rising, negative when falling. It says nothing about whether that matters.
func Slope(values []float64) float64 {
	clean := make([]float64, 0, len(values))
	for _, v := range values {
		if !math.IsNaN(v) {
			clean = append(clean, v)
		}
	}
	n := float64(len(clean))
	if n < 2 {
		return 0
	}

	var sumX, sumY, sumXY, sumXX float64
	for i, v := range clean {
		x := float64(i)
		sumX += x
		sumY += v
		sumXY += x * v
		sumXX += x * x
	}

	denominator := n*sumXX - sumX*sumX
	if denominator == 0 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / denominator
}

// ChangePct is the change from first to last as a percentage of the first.
// It returns zero when the first value is zero, since a change from nothing
// has no meaningful proportion.
func ChangePct(values []float64) float64 {
	clean := make([]float64, 0, len(values))
	for _, v := range values {
		if !math.IsNaN(v) {
			clean = append(clean, v)
		}
	}
	if len(clean) < 2 || clean[0] == 0 {
		return 0
	}
	return (clean[len(clean)-1] - clean[0]) / math.Abs(clean[0]) * 100
}

// Clamp01 keeps a value inside 0..1, for classifiers that normalise before
// combining. NaN becomes zero.
func Clamp01(v float64) float64 {
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
