package kpm

import (
	"testing"
	"time"
)

var testCell = Cell{
	PLMN: [3]byte{0x02, 0xf8, 0x29},
	NRCI: [5]byte{0x00, 0x00, 0x00, 0x00, 0x10},
}

// A subscription is only useful if the RAN can read it back, so the encoded
// action definition is decoded again rather than only checked for length.
func TestSubscribeStyle1RoundTrips(t *testing.T) {
	fn, err := DecodeRANFunction(ranFunction(t))
	if err != nil {
		t.Fatal(err)
	}
	sub, err := fn.Subscribe(Request{
		Style:        1,
		Measurements: []string{"DRB.RlcSduTransmittedVolumeDL", "L1M.DL-SS-RSRP.SSB"},
		Period:       time.Second,
		Cell:         &testCell,
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if len(sub.EventTrigger) == 0 {
		t.Error("no event trigger encoded")
	}
	style, format, ok := decodeActionDefinition(sub.ActionDef)
	if !ok {
		t.Fatal("action definition does not decode")
	}
	if style != 1 {
		t.Errorf("style = %d, want 1", style)
	}
	if format != 1 {
		t.Errorf("action format = %d, want 1", format)
	}
}

func TestSubscribeStyle3UsesFormat3(t *testing.T) {
	fn, err := DecodeRANFunction(ranFunction(t))
	if err != nil {
		t.Fatal(err)
	}
	sub, err := fn.Subscribe(Request{
		Style:        3,
		Measurements: []string{"DRB.RlcSduTransmittedVolumeDL"},
		Period:       time.Second,
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	style, format, ok := decodeActionDefinition(sub.ActionDef)
	if !ok {
		t.Fatal("action definition does not decode")
	}
	if style != 3 {
		t.Errorf("style = %d, want 3", style)
	}
	if format != 3 {
		t.Errorf("action format = %d, want 3", format)
	}
}

func TestSubscribeRejectsBadRequests(t *testing.T) {
	fn, err := DecodeRANFunction(ranFunction(t))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]Request{
		"unknown style": {
			Style: 99, Measurements: []string{"DRB.RlcSduTransmittedVolumeDL"},
			Period: time.Second, Cell: &testCell,
		},
		"unmeasured counter": {
			Style: 1, Measurements: []string{"DRB.NotAThing"},
			Period: time.Second, Cell: &testCell,
		},
		"no measurements": {
			Style: 1, Period: time.Second, Cell: &testCell,
		},
		"no period": {
			Style: 1, Measurements: []string{"DRB.RlcSduTransmittedVolumeDL"},
			Cell: &testCell,
		},
		"format 1 without a cell": {
			Style: 1, Measurements: []string{"DRB.RlcSduTransmittedVolumeDL"},
			Period: time.Second,
		},
		"unsupported action format": {
			Style: 2, Measurements: []string{"DRB.RlcSduTransmittedVolumeDL"},
			Period: time.Second, Cell: &testCell,
		},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := fn.Subscribe(req); err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}
