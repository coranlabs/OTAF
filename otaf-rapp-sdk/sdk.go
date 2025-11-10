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

// Package rappsdk is the OTAF rApp SDK: it builds rApps for the O-RAN Non-RT
// RIC.
//
// An rApp assembled with this SDK is onboarded to rApp Manager as a CSAR over
// R1 and lifecycle-managed from there; it is never started by hand.
package rappsdk

const (
	Name = "OTAF rApp SDK"

	// Version follows semantic versioning. From 1.0.0 on, anything exported
	// keeps working within the major version.
	Version = "1.0.0"

	// Identifies SDK-built rApps on every outbound call to a platform service.
	UserAgent = "OTAF-rApp-SDK/" + Version
)
