package kpm

/*
#include "wrapper.h"
*/
import "C"

import (
	"time"
	"unsafe"
)

// Kind says which of a record's values a RAN filled in.
type Kind int

const (
	NoValue Kind = iota
	Integer
	Real
)

// Record is one counter value from one reporting interval.
type Record struct {
	Kind    Kind
	Integer uint64
	Real    float64
}

// Float returns the value as a float, and whether the RAN reported one.
func (r Record) Float() (float64, bool) {
	switch r.Kind {
	case Integer:
		return float64(r.Integer), true
	case Real:
		return r.Real, true
	}
	return 0, false
}

// Report is what a RAN sent in one indication. Measurements names the counters
// it carries; each row of Records holds one value per name, in the same order.
type Report struct {
	Measurements []string
	Rows         [][]Record
	Period       time.Duration
}

// Latest pairs the names with the values from the last row a RAN sent.
func (r *Report) Latest() map[string]Record {
	out := make(map[string]Record, len(r.Measurements))
	if len(r.Rows) == 0 {
		return out
	}
	row := r.Rows[len(r.Rows)-1]
	for i, name := range r.Measurements {
		if i < len(row) {
			out[name] = row[i]
		}
	}
	return out
}

// DecodeIndicationMessage decodes the message a RIC indication carries. The
// message travels inside an E2AP indication, so it has to be taken out of that
// before it reaches here.
func DecodeIndicationMessage(raw []byte) (*Report, error) {
	if len(raw) == 0 {
		return nil, ErrDecode
	}
	im := C.e2sm_decode_ric_indication_message(unsafe.Pointer(&raw[0]), C.size_t(len(raw)))
	if im == nil {
		return nil, ErrDecode
	}
	defer C.e2sm_free_ric_indication_message(im)

	if C.e2sm_im_format(im) != 1 {
		return nil, ErrDecode
	}

	report := &Report{Period: time.Duration(C.e2sm_im_period(im)) * time.Millisecond}
	for i := 0; i < int(C.e2sm_im_name_count(im)); i++ {
		mi := C.e2sm_im_name(im, C.int(i))
		var n C.size_t
		buf := C.e2sm_mi_name(mi, &n)
		if buf == nil {
			report.Measurements = append(report.Measurements, "")
			continue
		}
		report.Measurements = append(report.Measurements, C.GoStringN(buf, C.int(n)))
	}
	for row := 0; row < int(C.e2sm_im_row_count(im)); row++ {
		var records []Record
		for i := 0; i < int(C.e2sm_im_value_count(im, C.int(row))); i++ {
			v := C.e2sm_im_value(im, C.int(row), C.int(i))
			switch C.e2sm_rec_kind(v) {
			case 1:
				records = append(records, Record{Kind: Integer, Integer: uint64(C.e2sm_rec_integer(v))})
			case 2:
				records = append(records, Record{Kind: Real, Real: float64(C.e2sm_rec_real(v))})
			default:
				records = append(records, Record{Kind: NoValue})
			}
		}
		report.Rows = append(report.Rows, records)
	}
	return report, nil
}
