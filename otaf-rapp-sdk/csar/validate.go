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

func checkToscaMeta(r *Report, files map[string][]byte) string {
	raw, ok := files[ToscaMetaPath]
	if !ok {
		return ""
	}

	var entry string
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		if strings.TrimSpace(key) == entryDefinitionsKey {
			entry = strings.TrimSpace(value)
			break
		}
	}

	if entry == "" {
		r.fail("tosca-entry-definitions",
			fmt.Sprintf("%s has no %s entry", ToscaMetaPath, entryDefinitionsKey),
			"this is the only way the platform locates the ASD inside the package")
		return ""
	}
	if _, ok := files[entry]; !ok {
		r.fail("tosca-entry-definitions",
			fmt.Sprintf("%s points at %s, which is not in the package", entryDefinitionsKey, entry),
			"")
		return ""
	}
	return entry
}

func checkAsd(r *Report, files map[string][]byte, asdPath string) []map[string]any {
	if asdPath == "" {
		return nil
	}

	var doc map[string]any
	if err := yaml.Unmarshal(files[asdPath], &doc); err != nil {
		r.fail("asd-parse", fmt.Sprintf("%s is not valid YAML: %v", asdPath, err), "")
		return nil
	}

	node := dig(doc, "topology_template", "node_templates", "applicationServiceDescriptor")
	if node == nil {
		r.fail("asd-structure",
			"ASD has no topology_template.node_templates.applicationServiceDescriptor",
			"the platform reads the descriptor from exactly this path")
		return nil
	}

	props, _ := dig(node, "properties").(map[string]any)
	if props == nil {
		r.fail("asd-descriptor", "ASD node has no properties block", "")
		return nil
	}

	r.ApplicationName = str(props["application_name"])
	r.ApplicationVersion = str(props["application_version"])
	r.DescriptorID = str(props["descriptor_id"])
	r.DescriptorInvariantID = str(props["descriptor_invariant_id"])

	for key, value := range map[string]string{
		"descriptor_id":           r.DescriptorID,
		"descriptor_invariant_id": r.DescriptorInvariantID,
	} {
		if value == "" {
			r.fail("asd-descriptor",
				fmt.Sprintf("%s is empty", key),
				"onboarding is refused outright when either identifier is missing")
			continue
		}
		if !isUUID(value) {
			r.warn("asd-descriptor",
				fmt.Sprintf("%s %q is not a UUID", key, value),
				"the ASD schema calls for RFC 4122 UUIDs")
		}
	}
	if r.ApplicationName == "" {
		r.fail("asd-descriptor", "application_name is empty",
			"the platform treats a descriptor without an application name as unparseable")
	}

	if _, hasLegacy := props["deploymentItems"]; hasLegacy {
		r.fail("deployment-items",
			"deployment items are declared under properties.deploymentItems",
			"move them to the artifacts block; the platform reads deployment items only from artifacts and overwrites anything found in properties")
	}

	artifacts, _ := dig(node, "artifacts").(map[string]any)
	if len(artifacts) == 0 {
		r.fail("deployment-items",
			"ASD declares no artifacts",
			"priming fails with \"No deployment items found in ASD metadata\" when this list is empty")
		return nil
	}

	keys := make([]string, 0, len(artifacts))
	for k := range artifacts {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var items []map[string]any
	for _, k := range keys {
		entry, ok := artifacts[k].(map[string]any)
		if !ok {
			r.fail("deployment-items", fmt.Sprintf("artifact %q is not a mapping", k), "")
			continue
		}
		entry["__key"] = k
		items = append(items, entry)
	}
	r.DeploymentItems = len(items)
	return items
}
