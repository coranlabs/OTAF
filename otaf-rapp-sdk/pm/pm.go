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

// Package pm decodes performance measurement data into one shape, whichever
// standard form it arrived in.
//
// A RAN emits the same counters as 3GPP TS 32.435 XML files or as VES
// perf3gpp events, and an rApp should not care which. Both decode to a Report
// of Measurements, each naming the object it describes and the counters read
// from it.
//
// Counter names and what they mean stay with the rApp. This package delivers
// the numbers; deciding which ones matter is the rApp's own work.
package pm

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
)

type Format string

const (
	FormatXML Format = "3gpp-xml"
	FormatVES Format = "ves-perf3gpp"
)

// Report is one collection of measurements from one network element.
type Report struct {
	Format Format `json:"format"`

	// Element is the distinguished name of what produced the data.
	Element string `json:"element,omitempty"`
	// DNPrefix is prepended to Element to form a fully distinguished name.
	DNPrefix string `json:"dn_prefix,omitempty"`
	Vendor   string `json:"vendor,omitempty"`

	Begin time.Time `json:"begin,omitempty"`
	End   time.Time `json:"end,omitempty"`

	// Granularity is the period the counters cover.
	Granularity time.Duration `json:"granularity,omitempty"`

	Measurements []Measurement `json:"measurements"`
}

// Measurement is the set of counters read from one object over one period.
type Measurement struct {
	// Group is the measurement family, from measInfoId.
	Group string `json:"group,omitempty"`

	// Object is the distinguished name of what was measured, which is what an
	// rApp keys its per-entity state on.
	Object string `json:"object"`

	// Suspect marks data the element itself flagged as unreliable, usually
	// because the collection period was disturbed. Acting on it is a choice.
	Suspect bool `json:"suspect,omitempty"`

	At          time.Time     `json:"at,omitempty"`
	Granularity time.Duration `json:"granularity,omitempty"`

	// Counters are kept as written. Both encodings carry values as text, and
	// some counters are not numbers at all.
	Counters map[string]string `json:"counters"`
}

// Codes carried by every failure this package returns. They are stable, so an
// rApp can branch on them and an operator can search for them.
const (
	CodeUnknownFormat  = "PM_UNKNOWN_FORMAT"
	CodeDecodeFailed   = "PM_DECODE_FAILED"
	CodeNotMeasurement = "PM_NOT_A_MEASUREMENT_FILE"
	CodeNoPerfEvents   = "PM_NO_PERF_EVENTS"
)

// ErrUnknownFormat matches any failure to recognise the encoding. Compare with
// errors.Is rather than by identity.
var ErrUnknownFormat = badData(CodeUnknownFormat, "unrecognised format")

// Everything this package rejects is bad data, and bad data does not improve
// on a second attempt. Saying so here means callers do not have to.
func badData(code, format string, args ...any) *errs.Error {
	return errs.Newf(errs.CategoryData, code, "pm: "+format, args...).Permanent()
}

func wrapBadData(cause error, code, format string, args ...any) *errs.Error {
	return errs.Wrapf(cause, errs.CategoryData, code, "pm: "+format, args...).Permanent()
}

// Parse decodes whichever standard form the data is in.
func Parse(data []byte) ([]*Report, error) {
	switch sniff(data) {
	case FormatXML:
		report, err := ParseXML(data)
		if err != nil {
			return nil, err
		}
		return []*Report{report}, nil
	case FormatVES:
		return ParseVES(data)
	default:
		return nil, badData(CodeUnknownFormat, "unrecognised format")
	}
}

func sniff(data []byte) Format {
	for _, b := range data {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case '<':
			return FormatXML
		case '{', '[':
			return FormatVES
		default:
			return ""
		}
	}
	return ""
}

// Get returns a counter as written.
func (m Measurement) Get(name string) (string, bool) {
	v, ok := m.Counters[name]
	return v, ok
}

// Float reads a counter as a number. Values the element could not supply are
// written as "NaN" or left empty by both encodings, and report false here.
func (m Measurement) Float(name string) (float64, bool) {
	raw, ok := m.Counters[name]
	if !ok {
		return 0, false
	}
	return parseFloat(raw)
}

// FloatOr reads a counter, falling back when it is absent or not a number.
func (m Measurement) FloatOr(name string, fallback float64) float64 {
	if v, ok := m.Float(name); ok {
		return v
	}
	return fallback
}

// Int reads a counter as a whole number.
func (m Measurement) Int(name string) (int64, bool) {
	raw, ok := m.Counters[name]
	if !ok {
		return 0, false
	}
	raw = strings.TrimSpace(raw)
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return n, true
	}
	// Some elements write whole numbers with a decimal part.
	if f, ok := parseFloat(raw); ok {
		return int64(f), true
	}
	return 0, false
}

// Sum adds several counters, ignoring any that are absent. It reports false
// when none of them were present at all.
func (m Measurement) Sum(names ...string) (float64, bool) {
	var total float64
	var found bool
	for _, name := range names {
		if v, ok := m.Float(name); ok {
			total += v
			found = true
		}
	}
	return total, found
}

// Distribution reads a counter written as a comma-separated list, which is how
// both encodings carry histogram buckets.
func (m Measurement) Distribution(name string) ([]float64, bool) {
	raw, ok := m.Counters[name]
	if !ok {
		return nil, false
	}
	parts := strings.Split(raw, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		v, ok := parseFloat(p)
		if !ok {
			return nil, false
		}
		out = append(out, v)
	}
	return out, len(out) > 0
}

// Names lists the counters present, sorted.
func (m Measurement) Names() []string {
	out := make([]string, 0, len(m.Counters))
	for name := range m.Counters {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Objects lists the measured objects in the report, sorted and deduplicated.
func (r *Report) Objects() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(r.Measurements))
	for _, m := range r.Measurements {
		if _, dup := seen[m.Object]; dup {
			continue
		}
		seen[m.Object] = struct{}{}
		out = append(out, m.Object)
	}
	sort.Strings(out)
	return out
}

// For returns every measurement describing one object. An element commonly
// splits its counters across several groups, so one object has more than one.
func (r *Report) For(object string) []Measurement {
	var out []Measurement
	for _, m := range r.Measurements {
		if m.Object == object {
			out = append(out, m)
		}
	}
	return out
}

// Group returns every measurement in one family.
func (r *Report) Group(group string) []Measurement {
	var out []Measurement
	for _, m := range r.Measurements {
		if m.Group == group {
			out = append(out, m)
		}
	}
	return out
}
