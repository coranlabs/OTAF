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

func (r *Report) fail(rule, msg, hint string) {
	r.Findings = append(r.Findings, Finding{Rule: rule, Severity: SeverityError, Message: msg, Hint: hint})
}

func (r *Report) warn(rule, msg, hint string) {
	r.Findings = append(r.Findings, Finding{Rule: rule, Severity: SeverityWarn, Message: msg, Hint: hint})
}

// Validate checks a built package the way the platform does when it is
// uploaded and primed. Every rule mirrors a check the platform performs, so
// passing here means the package gets past onboarding.
func Validate(csarPath string) (*Report, error) {
	report := &Report{Package: path.Base(csarPath)}

	if !strings.HasSuffix(csarPath, ".csar") {
		report.fail("package-name",
			"package file name must end with .csar",
			"rename the file; the platform rejects the upload on the extension alone")
	}

	zr, err := zip.OpenReader(csarPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", csarPath, err)
	}
	defer zr.Close()

	files := map[string][]byte{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("read %s from package: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s from package: %w", f.Name, err)
		}
		files[f.Name] = data
	}

	checkRequiredFiles(report, files)
	asdPath := checkToscaMeta(report, files)
	items := checkAsd(report, files, asdPath)
	checkDeploymentItems(report, files, items)
	checkResourceNames(report, files)
	checkAcmInstances(report, files)
	checkDmeInfoTypes(report, files)
	checkSmeProviders(report, files)
	checkSmeServiceApis(report, files)

	return report, nil
}

func checkRequiredFiles(r *Report, files map[string][]byte) {
	for _, required := range []string{ToscaMetaPath, AcmDefinition} {
		if _, ok := files[required]; !ok {
			r.fail("required-files",
				fmt.Sprintf("package is missing %s", required),
				"the platform checks for this path before it reads anything else")
		}
	}
}
