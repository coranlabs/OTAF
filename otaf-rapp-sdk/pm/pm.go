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
