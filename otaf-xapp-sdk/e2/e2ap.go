package e2

/*
#include "wrapper.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// ErrNotAnIndication is returned when a payload is not a RIC indication.
var ErrNotAnIndication = errors.New("e2: not a RIC indication")

// Indication is a report a RAN sent. Header and Message are the service model
// payloads it carries, still encoded: what is in them depends on RANFunctionID.
type Indication struct {
	RequestorID   int
	InstanceID    int
	RANFunctionID int
	ActionID      int
	Header        []byte
	Message       []byte
}

// DecodeIndication takes apart the E2AP message a RAN sends when reporting.
func DecodeIndication(raw []byte) (*Indication, error) {
	if len(raw) == 0 {
		return nil, ErrNotAnIndication
	}
	var out C.e2_indication_t
	if C.e2ap_decode_indication(unsafe.Pointer(&raw[0]), C.size_t(len(raw)), &out) != 0 {
		return nil, ErrNotAnIndication
	}
	defer C.e2ap_free_indication(&out)
	return &Indication{
		RequestorID:   int(out.requestor_id),
		InstanceID:    int(out.instance_id),
		RANFunctionID: int(out.ran_function_id),
		ActionID:      int(out.action_id),
		Header:        C.GoBytes(unsafe.Pointer(out.header), C.int(out.header_size)),
		Message:       C.GoBytes(unsafe.Pointer(out.message), C.int(out.message_size)),
	}, nil
}
