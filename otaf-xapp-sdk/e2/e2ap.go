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
#include "wrapper.h"
*/
import "C"

import (
	"github.com/coranlabs/OTAF/otaf-xapp-sdk/errs"
	"unsafe"
)

// ErrNotAnIndication is returned when a payload is not a RIC indication.
var ErrNotAnIndication = errs.New(errs.CategoryData, "E2_NOT_AN_INDICATION", "not a RIC indication")

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
