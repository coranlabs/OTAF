package kpm

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// ranFunction reads the fixture: a RAN function definition captured from a
// gNB's E2 setup, as the platform stores it.
func ranFunction(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/ranfunction.hex")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestDecodeRANFunction(t *testing.T) {
	fn, err := DecodeRANFunction(ranFunction(t))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if fn.ShortName != "ORAN-E2SM-KPM" {
		t.Errorf("short name = %q, want ORAN-E2SM-KPM", fn.ShortName)
	}
	if len(fn.ReportStyles) != 5 {
		t.Fatalf("report styles = %d, want 5", len(fn.ReportStyles))
	}
}

// Each report style demands its own action definition format. A subscription
// built with the wrong one is rejected by the RAN, so the mapping has to come
// from the RAN rather than from an assumption.
func TestReportStyleActionFormats(t *testing.T) {
	fn, err := DecodeRANFunction(ranFunction(t))
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]int{1: 1, 2: 2, 3: 3, 4: 4, 5: 5}
	for styleType, actionFormat := range want {
		style, ok := fn.Style(styleType)
		if !ok {
			t.Errorf("style %d missing", styleType)
			continue
		}
		if style.ActionFormat != actionFormat {
			t.Errorf("style %d action format = %d, want %d", styleType, style.ActionFormat, actionFormat)
		}
	}
}

func TestMeasurementsAreNamed(t *testing.T) {
	fn, err := DecodeRANFunction(ranFunction(t))
	if err != nil {
		t.Fatal(err)
	}
	style, ok := fn.Style(1)
	if !ok {
		t.Fatal("style 1 missing")
	}
	if len(style.Measurements) != 9 {
		t.Fatalf("measurements = %d, want 9", len(style.Measurements))
	}
	for _, m := range style.Measurements {
		if m.Name == "" {
			t.Errorf("measurement %d has no name", m.ID)
		}
		if m.ID <= 0 {
			t.Errorf("measurement %q has id %d", m.Name, m.ID)
		}
	}
}

func TestDecodeRejectsBadInput(t *testing.T) {
	valid := ranFunction(t)
	cases := map[string][]byte{
		"empty":     {},
		"garbage":   {0xde, 0xad, 0xbe, 0xef},
		"truncated": valid[:len(valid)/2],
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeRANFunction(raw); err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}
