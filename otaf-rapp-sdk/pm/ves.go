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
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type vesEnvelope struct {
	Event     *vesEvent  `json:"event"`
	EventList []vesEvent `json:"eventList"`
}

type vesEvent struct {
	CommonEventHeader commonEventHeader `json:"commonEventHeader"`
	Perf3gppFields    perf3gppFields    `json:"perf3gppFields"`
}

type commonEventHeader struct {
	Domain              string `json:"domain"`
	EventID             string `json:"eventId"`
	SourceName          string `json:"sourceName"`
	ReportingEntityName string `json:"reportingEntityName"`
	StartEpochMicrosec  int64  `json:"startEpochMicrosec"`
	LastEpochMicrosec   int64  `json:"lastEpochMicrosec"`
	NfVendorName        string `json:"nfVendorName"`
}

type perf3gppFields struct {
	Version            string             `json:"perf3gppFieldsVersion"`
	MeasDataCollection measDataCollection `json:"measDataCollection"`
}

type measDataCollection struct {
	// Sent as a number of seconds, or as an ISO 8601 period by some senders.
	GranularityPeriod json.RawMessage `json:"granularityPeriod"`

	MeasuredEntityUserName string         `json:"measuredEntityUserName"`
	MeasuredEntityDn       string         `json:"measuredEntityDn"`
	MeasuredEntitySoftware string         `json:"measuredEntitySoftwareVersion"`
	MeasInfoList           []measInfoJSON `json:"measInfoList"`
}

type measInfoJSON struct {
	// Sent as {"sMeasInfoId": "..."} or as a bare string.
	MeasInfoID json.RawMessage `json:"measInfoId"`
	MeasTypes  measTypesJSON   `json:"measTypes"`
	MeasValues []measValueJSON `json:"measValuesList"`
}

type measTypesJSON struct {
	SMeasTypesList []string `json:"sMeasTypesList"`
	IMeasTypesList []int    `json:"iMeasTypesList"`
}

type measValueJSON struct {
	MeasObjInstID string `json:"measObjInstId"`

	// Sent as a boolean or as the strings "true" and "false".
	SuspectFlag json.RawMessage `json:"suspectFlag"`

	MeasResults []measResultJSON `json:"measResults"`
}
