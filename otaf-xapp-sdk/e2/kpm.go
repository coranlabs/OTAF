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

// Package e2 speaks the E2 interface: E2AP messages and the E2SM-KPM service
// model a RAN reports measurements with.
package e2

/*
#cgo CFLAGS: -I${SRCDIR} -w -DASN_DISABLE_OER_SUPPORT
#cgo LDFLAGS: -lm
#include "wrapper.h"
*/
import "C"

import (
	"github.com/coranlabs/OTAF/otaf-xapp-sdk/errs"
	"unsafe"
)

// ErrDecode is returned when a payload is not a valid E2SM-KPM structure.
var ErrDecode = errs.New(errs.CategoryData, "E2_DECODE_FAILED", "decode failed")

// Measurement is one counter a report style can carry. ID is nil when the RAN
// names its measurements without identifying them.
type Measurement struct {
	ID   *int
	Name string
}

// ReportStyle is one way a RAN will report measurements. ActionFormat is the
// action definition format a subscription for this style must use.
type ReportStyle struct {
	Type         int
	Name         string
	ActionFormat int
	IndMsgFormat int
	Measurements []Measurement
}

// RANFunction is what a RAN advertises it can do for E2SM-KPM.
type RANFunction struct {
	ShortName    string
	OID          string
	Description  string
	ReportStyles []ReportStyle
}

// Style returns the report style with the given type.
func (f *RANFunction) Style(styleType int) (ReportStyle, bool) {
	for _, s := range f.ReportStyles {
		if s.Type == styleType {
			return s, true
		}
	}
	return ReportStyle{}, false
}

// DecodeRANFunction decodes a RAN function definition published at E2 setup.
func DecodeRANFunction(raw []byte) (*RANFunction, error) {
	if len(raw) == 0 {
		return nil, ErrDecode
	}
	rfd := C.e2sm_decode_ran_function_description(unsafe.Pointer(&raw[0]), C.size_t(len(raw)))
	if rfd == nil {
		return nil, ErrDecode
	}
	defer C.e2sm_free_ran_function_description(rfd)

	name := rfd.ranFunction_Name
	fn := &RANFunction{
		ShortName:   str(name.ranFunction_ShortName.buf, name.ranFunction_ShortName.size),
		OID:         str(name.ranFunction_E2SM_OID.buf, name.ranFunction_E2SM_OID.size),
		Description: str(name.ranFunction_Description.buf, name.ranFunction_Description.size),
	}
	for i := 0; i < int(C.e2sm_rfd_report_style_count(rfd)); i++ {
		s := C.e2sm_rfd_report_style(rfd, C.int(i))
		style := ReportStyle{
			Type:         int(s.ric_ReportStyle_Type),
			Name:         str(s.ric_ReportStyle_Name.buf, s.ric_ReportStyle_Name.size),
			ActionFormat: int(s.ric_ActionFormat_Type),
			IndMsgFormat: int(s.ric_IndicationMessageFormat_Type),
		}
		for j := 0; j < int(C.e2sm_style_meas_count(s)); j++ {
			m := C.e2sm_style_meas(s, C.int(j))
			meas := Measurement{Name: str(m.measName.buf, m.measName.size)}
			if id := int(C.e2sm_meas_id(m)); id >= 0 {
				meas.ID = &id
			}
			style.Measurements = append(style.Measurements, meas)
		}
		fn.ReportStyles = append(fn.ReportStyles, style)
	}
	return fn, nil
}

func str(b *C.uint8_t, n C.size_t) string {
	if b == nil || n == 0 {
		return ""
	}
	return C.GoStringN((*C.char)(unsafe.Pointer(b)), C.int(n))
}
