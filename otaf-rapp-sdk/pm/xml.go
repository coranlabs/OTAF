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
	"encoding/xml"
	"strings"
)

type measCollecFile struct {
	XMLName    xml.Name   `xml:"measCollecFile"`
	FileHeader fileHeader `xml:"fileHeader"`
	MeasData   []measData `xml:"measData"`
	FileFooter fileFooter `xml:"fileFooter"`
}

type fileHeader struct {
	FileFormatVersion string     `xml:"fileFormatVersion,attr"`
	VendorName        string     `xml:"vendorName,attr"`
	DNPrefix          string     `xml:"dnPrefix,attr"`
	FileSender        fileSender `xml:"fileSender"`
	MeasCollec        measCollec `xml:"measCollec"`
}

type fileSender struct {
	LocalDn     string `xml:"localDn,attr"`
	ElementType string `xml:"elementType,attr"`
}

type measCollec struct {
	BeginTime string `xml:"beginTime,attr"`
	EndTime   string `xml:"endTime,attr"`
}

type fileFooter struct {
	MeasCollec measCollec `xml:"measCollec"`
}

type measData struct {
	ManagedElement managedElement `xml:"managedElement"`
	MeasInfo       []measInfo     `xml:"measInfo"`
}

type managedElement struct {
	LocalDn   string `xml:"localDn,attr"`
	UserLabel string `xml:"userLabel,attr"`
	SwVersion string `xml:"swVersion,attr"`
}

type measInfo struct {
	MeasInfoID string     `xml:"measInfoId,attr"`
	Job        job        `xml:"job"`
	GranPeriod granPeriod `xml:"granPeriod"`
	RepPeriod  repPeriod  `xml:"repPeriod"`

	// The standard allows the counter names either as repeated measType
	// elements carrying a position, or as one whitespace-separated measTypes
	// string. Elements in the field use both.
	MeasType  []measType `xml:"measType"`
	MeasTypes string     `xml:"measTypes"`

	MeasValue []measValue `xml:"measValue"`
}

type job struct {
	JobID string `xml:"jobId,attr"`
}

type granPeriod struct {
	Duration string `xml:"duration,attr"`
	EndTime  string `xml:"endTime,attr"`
}

type repPeriod struct {
	Duration string `xml:"duration,attr"`
}

type measType struct {
	P     int    `xml:"p,attr"`
	Value string `xml:",chardata"`
}

type measValue struct {
	MeasObjLdn string `xml:"measObjLdn,attr"`

	// Results likewise come either as positioned r elements or as one
	// whitespace-separated measResults string.
	R           []result `xml:"r"`
	MeasResults string   `xml:"measResults"`

	Suspect string `xml:"suspect"`
}

type result struct {
	P     int    `xml:"p,attr"`
	Value string `xml:",chardata"`
}

// ParseXML decodes a 3GPP TS 32.435 measurement collection file.
func ParseXML(data []byte) (*Report, error) {
	var file measCollecFile
	if err := xml.Unmarshal(data, &file); err != nil {
		return nil, wrapBadData(err, CodeDecodeFailed, "could not decode the measurement file")
	}
	if file.XMLName.Local != "measCollecFile" {
		return nil, badData(CodeNotMeasurement,
			"root element is %q, want measCollecFile", file.XMLName.Local)
	}

	report := &Report{
		Format:   FormatXML,
		Element:  file.FileHeader.FileSender.LocalDn,
		DNPrefix: file.FileHeader.DNPrefix,
		Vendor:   file.FileHeader.VendorName,
		Begin:    parseTime(file.FileHeader.MeasCollec.BeginTime),
	}
	report.End = parseTime(firstNonEmpty(
		file.FileFooter.MeasCollec.EndTime,
		file.FileHeader.MeasCollec.EndTime,
	))

	for _, data := range file.MeasData {
		// The sender names itself in the header, but each block names the
		// element the counters actually came from.
		if report.Element == "" {
			report.Element = data.ManagedElement.LocalDn
		}

		for _, info := range data.MeasInfo {
			names := counterNames(info)
			granularity := parseDuration(info.GranPeriod.Duration)
			at := parseTime(info.GranPeriod.EndTime)

			if report.Granularity == 0 {
				report.Granularity = granularity
			}

			for _, value := range info.MeasValue {
				measurement := Measurement{
					Group:       info.MeasInfoID,
					Object:      qualify(data.ManagedElement.LocalDn, value.MeasObjLdn),
					Suspect:     strings.EqualFold(strings.TrimSpace(value.Suspect), "true"),
					At:          at,
					Granularity: granularity,
					Counters:    countersOf(names, value),
				}
				if measurement.At.IsZero() {
					measurement.At = report.End
				}
				report.Measurements = append(report.Measurements, measurement)
			}
		}
	}

	return report, nil
}

// counterNames returns the counter name for each position, one-based, in the
// order the results will arrive.
func counterNames(info measInfo) []string {
	if len(info.MeasType) > 0 {
		highest := 0
		for _, t := range info.MeasType {
			if t.P > highest {
				highest = t.P
			}
		}

		names := make([]string, highest)
		for i, t := range info.MeasType {
			name := strings.TrimSpace(t.Value)
			switch {
			case t.P >= 1 && t.P <= highest:
				names[t.P-1] = name
			case i < highest:
				// A file that omits the position falls back to document order.
				names[i] = name
			}
		}
		return names
	}
	return strings.Fields(info.MeasTypes)
}

func countersOf(names []string, value measValue) map[string]string {
	counters := make(map[string]string, len(names))

	if len(value.R) > 0 {
		for i, r := range value.R {
			position := r.P
			if position < 1 {
				position = i + 1
			}
			if name := nameAt(names, position); name != "" {
				counters[name] = strings.TrimSpace(r.Value)
			}
		}
		return counters
	}

	for i, raw := range strings.Fields(value.MeasResults) {
		if name := nameAt(names, i+1); name != "" {
			counters[name] = raw
		}
	}
	return counters
}

func nameAt(names []string, position int) string {
	if position < 1 || position > len(names) {
		return ""
	}
	return names[position-1]
}
