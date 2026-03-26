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

package csar

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Severity string

const (
	SeverityError Severity = "error"
	SeverityWarn  Severity = "warning"
)

type Finding struct {
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Hint     string   `json:"hint,omitempty"`
}

type Report struct {
	Package               string    `json:"package"`
	ApplicationName       string    `json:"application_name,omitempty"`
	ApplicationVersion    string    `json:"application_version,omitempty"`
	DescriptorID          string    `json:"descriptor_id,omitempty"`
	DescriptorInvariantID string    `json:"descriptor_invariant_id,omitempty"`
	DeploymentItems       int       `json:"deployment_items"`
	Resources             []string  `json:"resources,omitempty"`
	Findings              []Finding `json:"findings"`
}

func (r *Report) OK() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			return false
		}
	}
	return true
}

func (r *Report) Errors() []Finding   { return r.filter(SeverityError) }

func (r *Report) Warnings() []Finding { return r.filter(SeverityWarn) }

func (r *Report) filter(s Severity) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Severity == s {
			out = append(out, f)
		}
	}
	return out
}
