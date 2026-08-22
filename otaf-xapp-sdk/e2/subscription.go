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

package e2

/*
#include <stdlib.h>
#include "wrapper.h"
*/
import "C"

import (
	"github.com/coranlabs/OTAF/otaf-xapp-sdk/errs"
	"time"
	"unsafe"
)

// Cell identifies the cell a report style measures.
type Cell struct {
	PLMN [3]byte
	NRCI [5]byte
}

// Subscription is the pair of encoded payloads an E2 subscription carries.
type Subscription struct {
	EventTrigger []byte
	ActionDef    []byte
	Style        ReportStyle
}

// Request asks a RAN to report measurements on one of its report styles.
type Request struct {
	Style        int
	Measurements []string
	Period       time.Duration
	Cell         *Cell
}

// ErrEncode is returned when a subscription cannot be encoded.
var ErrEncode = errs.New(errs.CategoryInternal, "E2_ENCODE_FAILED", "encode failed")

const encodeBuf = 8192

// Subscribe builds the payloads for a subscription against this RAN function.
// The action definition format comes from the report style the RAN advertised.
func (f *RANFunction) Subscribe(req Request) (*Subscription, error) {
	style, ok := f.Style(req.Style)
	if !ok {
		return nil, errs.Newf(errs.CategoryConfig, "E2_NO_SUCH_STYLE", "RAN does not offer report style %d", req.Style)
	}
	if len(req.Measurements) == 0 {
		return nil, errs.New(errs.CategoryConfig, "E2_NO_MEASUREMENTS", "no measurements requested")
	}
	if req.Period <= 0 {
		return nil, errs.New(errs.CategoryConfig, "E2_BAD_PERIOD", "reporting period must be positive")
	}
	for _, name := range req.Measurements {
		if !style.reports(name) {
			return nil, errs.Newf(errs.CategoryConfig, "E2_NO_SUCH_MEASUREMENT", "report style %d does not measure %q", style.Type, name)
		}
	}

	period := req.Period.Milliseconds()
	trigger, err := encodeEventTrigger(period)
	if err != nil {
		return nil, err
	}

	var action []byte
	switch style.ActionFormat {
	case 1:
		if req.Cell == nil {
			return nil, errs.Newf(errs.CategoryConfig, "E2_CELL_REQUIRED", "report style %d measures a cell, so one must be given", style.Type)
		}
		action, err = encodeActionFormat1(req.Measurements, style.Type, period, *req.Cell)
	case 3:
		action, err = encodeActionFormat3(req.Measurements, style.Type, period)
	default:
		return nil, errs.Newf(errs.CategoryConfig, "E2_UNSUPPORTED_FORMAT",
			"report style %d needs action definition format %d, which is not supported",
			style.Type, style.ActionFormat)
	}
	if err != nil {
		return nil, err
	}
	return &Subscription{EventTrigger: trigger, ActionDef: action, Style: style}, nil
}

func (s ReportStyle) reports(name string) bool {
	for _, m := range s.Measurements {
		if m.Name == name {
			return true
		}
	}
	return false
}

func encodeEventTrigger(periodMs int64) ([]byte, error) {
	buf := make([]byte, encodeBuf)
	period := C.long(periodMs)
	n := C.e2sm_encode_ric_event_trigger_definition(
		unsafe.Pointer(&buf[0]), C.size_t(len(buf)), 1, &period)
	if n <= 0 {
		return nil, ErrEncode
	}
	return buf[:n], nil
}

func encodeActionFormat1(names []string, style int, periodMs int64, cell Cell) ([]byte, error) {
	cNames, free := cStrings(names)
	defer free()

	buf := make([]byte, encodeBuf)
	size := C.size_t(len(buf))
	plmn := cell.PLMN
	nrci := cell.NRCI
	n := C.e2sm_encode_ric_action_definition_format1_by_name(
		(*C.uchar)(&buf[0]), &size,
		(**C.char)(unsafe.Pointer(&cNames[0])), C.size_t(len(names)),
		C.long(style), C.ulong(periodMs),
		(*C.uchar)(&plmn[0]), (*C.uchar)(&nrci[0]))
	if n <= 0 {
		return nil, ErrEncode
	}
	return buf[:n], nil
}

func encodeActionFormat3(names []string, style int, periodMs int64) ([]byte, error) {
	cNames, free := cStrings(names)
	defer free()

	buf := make([]byte, encodeBuf)
	size := C.size_t(len(buf))
	n := C.e2sm_encode_ric_action_definition_format3_by_name(
		(*C.uchar)(&buf[0]), &size,
		(**C.char)(unsafe.Pointer(&cNames[0])), C.size_t(len(names)),
		C.long(style), C.ulong(periodMs))
	if n <= 0 {
		return nil, ErrEncode
	}
	return buf[:n], nil
}

func decodeEventTrigger(raw []byte) (periodMs uint64, ok bool) {
	if len(raw) == 0 {
		return 0, false
	}
	etd := C.e2sm_decode_ric_event_trigger_definition(unsafe.Pointer(&raw[0]), C.size_t(len(raw)))
	if etd == nil {
		return 0, false
	}
	defer C.e2sm_free_ric_event_trigger_definition(etd)
	return uint64(C.e2sm_etd_reporting_period(etd)), true
}

func decodeActionDefinition(raw []byte) (style, format int, ok bool) {
	if len(raw) == 0 {
		return 0, 0, false
	}
	ad := C.e2sm_decode_ric_action_definition(unsafe.Pointer(&raw[0]), C.size_t(len(raw)))
	if ad == nil {
		return 0, 0, false
	}
	defer C.e2sm_free_ric_action_definition(ad)
	return int(C.e2sm_ad_style_type(ad)), int(C.e2sm_ad_format(ad)), true
}

func cStrings(names []string) ([]*C.char, func()) {
	out := make([]*C.char, len(names))
	for i, name := range names {
		out[i] = C.CString(name)
	}
	return out, func() {
		for _, p := range out {
			C.free(unsafe.Pointer(p))
		}
	}
}
