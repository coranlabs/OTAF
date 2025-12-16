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
	"testing"
	"time"
)

const xmlPositional = `<?xml version="1.0" encoding="UTF-8"?>
<measCollecFile xmlns="http://www.3gpp.org/ftp/specs/archive/32_series/32.435#measCollec">
  <fileHeader fileFormatVersion="32.435 V10.0" vendorName="Acme" dnPrefix="SubNetwork=Europe">
    <fileSender localDn="ManagedElement=gnb-001" elementType="gNB"/>
    <measCollec beginTime="2026-03-01T10:00:00+00:00"/>
  </fileHeader>
  <measData>
    <managedElement localDn="ManagedElement=gnb-001" swVersion="R2026A"/>
    <measInfo measInfoId="NRCellDU">
      <job jobId="job-1"/>
      <granPeriod duration="PT900S" endTime="2026-03-01T10:15:00+00:00"/>
      <repPeriod duration="PT900S"/>
      <measType p="1">counterA</measType>
      <measType p="2">counterB</measType>
      <measType p="3">counterC</measType>
      <measValue measObjLdn="NRCellDU=cell-1">
        <r p="1">10</r>
        <r p="2">20.5</r>
        <r p="3">30</r>
      </measValue>
      <measValue measObjLdn="NRCellDU=cell-2">
        <r p="1">11</r>
        <r p="2">21.5</r>
        <r p="3">31</r>
        <suspect>true</suspect>
      </measValue>
    </measInfo>
  </measData>
  <fileFooter>
    <measCollec endTime="2026-03-01T10:15:00+00:00"/>
  </fileFooter>
</measCollecFile>`

// The form with one whitespace-separated measTypes string, no namespace.
const xmlWhitespace = `<measCollecFile>
  <fileHeader>
    <fileSender localDn="ManagedElement=gnb-002"/>
    <measCollec beginTime="2026-03-01T10:00:00Z"/>
  </fileHeader>
  <measData>
    <managedElement localDn="ManagedElement=gnb-002"/>
    <measInfo measInfoId="NRCellCU">
      <granPeriod duration="PT15M" endTime="2026-03-01T10:15:00Z"/>
      <measTypes>counterA counterB</measTypes>
      <measValue measObjLdn="NRCellCU=cell-9">
        <measResults>7 8</measResults>
      </measValue>
    </measInfo>
  </measData>
</measCollecFile>`
