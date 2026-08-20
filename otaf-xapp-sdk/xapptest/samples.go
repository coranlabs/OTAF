package xapptest

import (
	_ "embed"
	"encoding/hex"
	"strings"
	"testing"
)

//go:embed testdata/indication-cucp.hex
var indicationCUCP string

//go:embed testdata/ranfunction-cucp.hex
var ranFunctionCUCP string

func decode(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Indication returns a report a gNB CU-CP sent, as it arrived over RMR. It
// carries RRC and UE context counters on a ten second period.
func Indication(t testing.TB) []byte {
	t.Helper()
	return decode(t, indicationCUCP)
}

// RANFunction returns the E2SM-KPM definition a gNB CU-CP published at E2
// setup, as the platform stores it.
func RANFunction(t testing.TB) []byte {
	t.Helper()
	return decode(t, ranFunctionCUCP)
}
