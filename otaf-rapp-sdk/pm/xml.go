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
