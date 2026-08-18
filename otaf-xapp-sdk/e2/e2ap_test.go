package e2

import (
	"testing"
	"time"
)

// An indication arrives wrapped in E2AP with the service model payload inside.
// Both layers are decoded here, which is what an xApp has to do.
func TestIndicationFromAgNBDecodesThrough(t *testing.T) {
	ind, err := DecodeIndication(fixture(t, "indication-cucp.hex"))
	if err != nil {
		t.Fatalf("e2ap: %v", err)
	}
	if ind.RANFunctionID != 2 {
		t.Errorf("ran function = %d, want 2", ind.RANFunctionID)
	}
	if len(ind.Header) == 0 || len(ind.Message) == 0 {
		t.Fatalf("header %d bytes, message %d bytes", len(ind.Header), len(ind.Message))
	}

	report, err := DecodeIndicationMessage(ind.Message)
	if err != nil {
		t.Fatalf("kpm: %v", err)
	}
	if report.Period != 10*time.Second {
		t.Errorf("period = %v, want 10s", report.Period)
	}
	latest := report.Latest()
	for _, name := range []string{"RRC.ConnMean", "RRC.ConnMax", "RRC.ConnEstabAtt"} {
		if _, ok := latest[name]; !ok {
			t.Errorf("%s missing from %v", name, report.Measurements)
		}
	}
}

func TestDecodeIndicationRejectsNonIndications(t *testing.T) {
	for name, raw := range map[string][]byte{
		"empty":   {},
		"garbage": {0xde, 0xad, 0xbe, 0xef},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeIndication(raw); err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}
