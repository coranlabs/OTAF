package kpm

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func ranFunction(t *testing.T) []byte {
	t.Helper()
	return fixture(t, "ranfunction.hex")
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
			t.Error("measurement has no name")
		}
		if m.ID == nil {
			t.Errorf("measurement %q has no id", m.Name)
		}
	}
}

func TestDecodeCUCPRANFunction(t *testing.T) {
	fn, err := DecodeRANFunction(fixture(t, "ranfunction-cucp.hex"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if fn.OID != "1.3.6.1.4.1.53148.1.2.2.2" {
		t.Errorf("oid = %q", fn.OID)
	}
	style, ok := fn.Style(1)
	if !ok {
		t.Fatal("style 1 missing")
	}
	if style.ActionFormat != 1 {
		t.Errorf("action format = %d, want 1", style.ActionFormat)
	}
	for _, m := range style.Measurements {
		if m.Name == "" {
			t.Error("measurement has no name")
		}
		if m.ID != nil {
			t.Errorf("measurement %q reports id %d, expected none", m.Name, *m.ID)
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

func hexBytes(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
