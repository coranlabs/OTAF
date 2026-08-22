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

package e2

import (
	"testing"
	"time"
)

// The E2SM message inside the captured E2AP indication, as the RAN sent it.
const cucpIndicationMessage = "0c00000003000000000000000200b05252432e436f6e6e4d65616e0120000000a05252432e436f6e6e4d61780120000000f05252432e436f6e6e45737461624174740120000040270f"

func TestDecodeIndicationFromACUCP(t *testing.T) {
	report, err := DecodeIndicationMessage(hexBytes(t, cucpIndicationMessage))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []string{"RRC.ConnMean", "RRC.ConnMax", "RRC.ConnEstabAtt"}
	if len(report.Measurements) != len(want) {
		t.Fatalf("measurements = %v, want %v", report.Measurements, want)
	}
	for i, name := range want {
		if report.Measurements[i] != name {
			t.Errorf("measurement %d = %q, want %q", i, report.Measurements[i], name)
		}
	}
	if report.Period != 10*time.Second {
		t.Errorf("period = %v, want 10s", report.Period)
	}
	if len(report.Rows) == 0 {
		t.Fatal("no rows")
	}
	for _, row := range report.Rows {
		if len(row) != len(want) {
			t.Errorf("row has %d values, want %d", len(row), len(want))
		}
	}
}

func TestLatestPairsNamesWithValues(t *testing.T) {
	report, err := DecodeIndicationMessage(hexBytes(t, cucpIndicationMessage))
	if err != nil {
		t.Fatal(err)
	}
	latest := report.Latest()
	for _, name := range []string{"RRC.ConnMean", "RRC.ConnMax", "RRC.ConnEstabAtt"} {
		rec, ok := latest[name]
		if !ok {
			t.Errorf("%s missing", name)
			continue
		}
		if _, present := rec.Float(); !present && rec.Kind != NoValue {
			t.Errorf("%s reports kind %v but no value", name, rec.Kind)
		}
	}
}

func TestDecodeIndicationRejectsBadInput(t *testing.T) {
	for name, raw := range map[string][]byte{
		"empty":   {},
		"garbage": {0xde, 0xad, 0xbe, 0xef},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeIndicationMessage(raw); err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}
