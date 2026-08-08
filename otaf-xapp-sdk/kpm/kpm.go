// Package kpm decodes E2SM-KPM, the service model a RAN uses to report
// performance measurements over E2.
package kpm

/*
#cgo CFLAGS: -I${SRCDIR} -w
#cgo LDFLAGS: -lm
#include "wrapper.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// ErrDecode is returned when a payload is not a valid E2SM-KPM structure.
var ErrDecode = errors.New("kpm: decode failed")

// Measurement is one counter a report style can carry.
type Measurement struct {
	ID   int
	Name string
}

// ReportStyle is one way a RAN will report measurements. ActionFormat is the
// action definition format a subscription must use for this style; sending any
// other format is rejected by the RAN.
type ReportStyle struct {
	Type         int
	Name         string
	ActionFormat int
	IndMsgFormat int
	Measurements []Measurement
}

// RANFunction is what a RAN advertises it can do for E2SM-KPM. It is carried in
// the RAN function definition that E2 setup publishes and the platform stores.
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

// DecodeRANFunction decodes a RAN function definition as published at E2 setup.
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
			style.Measurements = append(style.Measurements, Measurement{
				ID:   int(C.e2sm_meas_id(m)),
				Name: str(m.measName.buf, m.measName.size),
			})
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
