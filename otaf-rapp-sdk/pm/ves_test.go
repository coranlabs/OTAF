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

package pm

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/retry"
)

const vesEvent7 = `{
  "event": {
    "commonEventHeader": {
      "domain": "perf3gpp",
      "eventId": "evt-1",
      "sourceName": "gnb-001",
      "nfVendorName": "Acme",
      "startEpochMicrosec": 1772100000000000,
      "lastEpochMicrosec": 1772100900000000
    },
    "perf3gppFields": {
      "perf3gppFieldsVersion": "1.0",
      "measDataCollection": {
        "granularityPeriod": 900,
        "measuredEntityDn": "ManagedElement=gnb-001",
        "measuredEntitySoftwareVersion": "R2026A",
        "measInfoList": [
          {
            "measInfoId": {"sMeasInfoId": "NRCellDU"},
            "measTypes": {"sMeasTypesList": ["counterA", "counterB"]},
            "measValuesList": [
              {
                "measObjInstId": "NRCellDU=cell-1",
                "suspectFlag": "false",
                "measResults": [
                  {"p": 1, "sValue": "10"},
                  {"p": 2, "sValue": "20.5"}
                ]
              },
              {
                "measObjInstId": "NRCellDU=cell-2",
                "suspectFlag": true,
                "measResults": [
                  {"p": 1, "sValue": "11"},
                  {"p": 2, "sValue": "21.5"}
                ]
              }
            ]
          }
        ]
      }
    }
  }
}`

func TestVESEvent(t *testing.T) {
	reports, err := ParseVES([]byte(vesEvent7))
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	report := reports[0]

	if report.Format != FormatVES {
		t.Errorf("format = %s", report.Format)
	}
	if report.Element != "ManagedElement=gnb-001" {
		t.Errorf("element = %q", report.Element)
	}
	if report.Granularity != 15*time.Minute {
		t.Errorf("granularity = %v, want 15m", report.Granularity)
	}
	if report.End.IsZero() {
		t.Error("the last epoch should become the report end")
	}
	if len(report.Measurements) != 2 {
		t.Fatalf("measurements = %d, want 2", len(report.Measurements))
	}

	first := report.Measurements[0]
	if first.Group != "NRCellDU" {
		t.Errorf("group = %q", first.Group)
	}
	if got, ok := first.Float("counterB"); !ok || got != 20.5 {
		t.Errorf("counterB = %v (%v), want 20.5", got, ok)
	}
	if first.Suspect {
		t.Error(`suspectFlag "false" must not read as suspect`)
	}
	if !report.Measurements[1].Suspect {
		t.Error("a boolean suspectFlag should be honoured")
	}
}
