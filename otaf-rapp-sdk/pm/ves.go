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

type measResultJSON struct {
	P      int             `json:"p"`
	SValue string          `json:"sValue"`
	IValue json.RawMessage `json:"iValue"`
}

// ParseVES decodes a VES event carrying the perf3gpp domain. Events in other
// domains are skipped rather than rejected, since a topic commonly carries
// more than one.
func ParseVES(data []byte) ([]*Report, error) {
	events, err := vesEvents(data)
	if err != nil {
		return nil, err
	}

	var reports []*Report
	for i := range events {
		event := events[i]
		if !strings.EqualFold(event.CommonEventHeader.Domain, "perf3gpp") &&
			event.CommonEventHeader.Domain != "" {
			continue
		}
		reports = append(reports, reportFromVES(event))
	}

	if len(reports) == 0 {
		return nil, badData(CodeNoPerfEvents, "no perf3gpp events in payload")
	}
	return reports, nil
}

func vesEvents(data []byte) ([]vesEvent, error) {
	trimmed := strings.TrimSpace(string(data))

	if strings.HasPrefix(trimmed, "[") {
		var batch []vesEnvelope
		if err := json.Unmarshal(data, &batch); err != nil {
			return nil, wrapBadData(err, CodeDecodeFailed, "could not decode the VES batch")
		}
		var events []vesEvent
		for _, item := range batch {
			events = append(events, item.events()...)
		}
		return events, nil
	}

	var envelope vesEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, wrapBadData(err, CodeDecodeFailed, "could not decode the VES event")
	}
	events := envelope.events()
	if len(events) == 0 {
		return nil, badData(CodeNoPerfEvents, "payload carries no VES event")
	}
	return events, nil
}

func (e vesEnvelope) events() []vesEvent {
	if len(e.EventList) > 0 {
		return e.EventList
	}
	if e.Event != nil {
		return []vesEvent{*e.Event}
	}
	return nil
}

func reportFromVES(event vesEvent) *Report {
	header := event.CommonEventHeader
	collection := event.Perf3gppFields.MeasDataCollection

	report := &Report{
		Format:      FormatVES,
		Element:     firstNonEmpty(collection.MeasuredEntityDn, header.SourceName),
		Vendor:      header.NfVendorName,
		Begin:       epochMicros(header.StartEpochMicrosec),
		End:         epochMicros(header.LastEpochMicrosec),
		Granularity: granularityOf(collection.GranularityPeriod),
	}

	for _, info := range collection.MeasInfoList {
		names := vesCounterNames(info.MeasTypes)
		group := vesGroup(info.MeasInfoID)

		for _, value := range info.MeasValues {
			counters := make(map[string]string, len(value.MeasResults))
			for i, r := range value.MeasResults {
				position := r.P
				if position < 1 {
					position = i + 1
				}
				name := nameAt(names, position)
				if name == "" {
					continue
				}
				counters[name] = vesValue(r)
			}

			report.Measurements = append(report.Measurements, Measurement{
				Group:       group,
				Object:      qualify(report.Element, value.MeasObjInstID),
				Suspect:     suspect(value.SuspectFlag),
				At:          report.End,
				Granularity: report.Granularity,
				Counters:    counters,
			})
		}
	}
	return report
}

func vesCounterNames(types measTypesJSON) []string {
	if len(types.SMeasTypesList) > 0 {
		return types.SMeasTypesList
	}
	names := make([]string, len(types.IMeasTypesList))
	for i, id := range types.IMeasTypesList {
		names[i] = strconv.Itoa(id)
	}
	return names
}

func vesValue(r measResultJSON) string {
	if r.SValue != "" {
		return r.SValue
	}
	if len(r.IValue) > 0 {
		return strings.Trim(strings.TrimSpace(string(r.IValue)), `"`)
	}
	return ""
}

func vesGroup(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	var wrapped struct {
		SMeasInfoID string `json:"sMeasInfoId"`
		IMeasInfoID int    `json:"iMeasInfoId"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		if wrapped.SMeasInfoID != "" {
			return wrapped.SMeasInfoID
		}
		if wrapped.IMeasInfoID != 0 {
			return strconv.Itoa(wrapped.IMeasInfoID)
		}
	}
	return ""
}

func granularityOf(raw json.RawMessage) time.Duration {
	if len(raw) == 0 {
		return 0
	}

	var seconds int64
	if err := json.Unmarshal(raw, &seconds); err == nil {
		return time.Duration(seconds) * time.Second
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return parseDuration(text)
	}
	return 0
}
