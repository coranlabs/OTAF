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
