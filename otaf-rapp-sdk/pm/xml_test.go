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

func TestXMLPositionalForm(t *testing.T) {
	report, err := ParseXML([]byte(xmlPositional))
	if err != nil {
		t.Fatal(err)
	}

	if report.Format != FormatXML {
		t.Errorf("format = %s, want %s", report.Format, FormatXML)
	}
	if report.Element != "ManagedElement=gnb-001" {
		t.Errorf("element = %q", report.Element)
	}
	if report.DNPrefix != "SubNetwork=Europe" {
		t.Errorf("dn prefix = %q", report.DNPrefix)
	}
	if report.Vendor != "Acme" {
		t.Errorf("vendor = %q", report.Vendor)
	}
	if report.Granularity != 15*time.Minute {
		t.Errorf("granularity = %v, want 15m", report.Granularity)
	}
	if len(report.Measurements) != 2 {
		t.Fatalf("measurements = %d, want 2", len(report.Measurements))
	}

	first := report.Measurements[0]
	if first.Group != "NRCellDU" {
		t.Errorf("group = %q, want NRCellDU", first.Group)
	}
	if got, ok := first.Float("counterB"); !ok || got != 20.5 {
		t.Errorf("counterB = %v (%v), want 20.5", got, ok)
	}
	if first.At.IsZero() {
		t.Error("the granularity period end should become the sample time")
	}
	if first.Suspect {
		t.Error("the first measurement is not suspect")
	}
}

// Data the element itself flagged as unreliable has to stay distinguishable,
// or an rApp treats a disturbed collection period as fact.
func TestSuspectFlagIsCarried(t *testing.T) {
	report, err := ParseXML([]byte(xmlPositional))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Measurements[1].Suspect {
		t.Error("a measurement marked suspect should say so")
	}
}

func TestXMLWhitespaceForm(t *testing.T) {
	report, err := ParseXML([]byte(xmlWhitespace))
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Measurements) != 1 {
		t.Fatalf("measurements = %d, want 1", len(report.Measurements))
	}
	m := report.Measurements[0]

	if got, ok := m.Int("counterA"); !ok || got != 7 {
		t.Errorf("counterA = %v (%v), want 7", got, ok)
	}
	if got, ok := m.Int("counterB"); !ok || got != 8 {
		t.Errorf("counterB = %v (%v), want 8", got, ok)
	}
	if report.Granularity != 15*time.Minute {
		t.Errorf("granularity = %v, want 15m from PT15M", report.Granularity)
	}
}

// The same cell must not look like two different objects depending on which
// file it arrived in.
func TestObjectNamesAreQualified(t *testing.T) {
	report, err := ParseXML([]byte(xmlPositional))
	if err != nil {
		t.Fatal(err)
	}

	want := "ManagedElement=gnb-001,NRCellDU=cell-1"
	if got := report.Measurements[0].Object; got != want {
		t.Errorf("object = %q, want %q", got, want)
	}
}

func TestAlreadyQualifiedObjectIsLeftAlone(t *testing.T) {
	const doc = `<measCollecFile><fileHeader><fileSender localDn="ManagedElement=gnb-1"/></fileHeader>
	<measData><managedElement localDn="ManagedElement=gnb-1"/>
	<measInfo measInfoId="g"><measTypes>a</measTypes>
	<measValue measObjLdn="ManagedElement=gnb-1,NRCellDU=cell-1"><r p="1">1</r></measValue>
	</measInfo></measData></measCollecFile>`

	report, err := ParseXML([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Measurements[0].Object; got != "ManagedElement=gnb-1,NRCellDU=cell-1" {
		t.Errorf("object = %q, want it unchanged", got)
	}
}

// The standard allows several measData blocks in one file, one per element.
func TestMultipleMeasDataBlocks(t *testing.T) {
	const doc = `<measCollecFile>
	  <fileHeader><fileSender localDn="ManagedElement=gnb-1"/></fileHeader>
	  <measData><managedElement localDn="ManagedElement=gnb-1"/>
	    <measInfo measInfoId="g1"><measTypes>a</measTypes>
	      <measValue measObjLdn="NRCellDU=c1"><r p="1">1</r></measValue></measInfo></measData>
	  <measData><managedElement localDn="ManagedElement=gnb-2"/>
	    <measInfo measInfoId="g2"><measTypes>a</measTypes>
	      <measValue measObjLdn="NRCellDU=c2"><r p="1">2</r></measValue></measInfo></measData>
	</measCollecFile>`

	report, err := ParseXML([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Measurements) != 2 {
		t.Fatalf("measurements = %d, want 2", len(report.Measurements))
	}
	if report.Measurements[1].Object != "ManagedElement=gnb-2,NRCellDU=c2" {
		t.Errorf("second block object = %q, want it named after its own element",
			report.Measurements[1].Object)
	}
}
