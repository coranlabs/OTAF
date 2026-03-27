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

func checkDeploymentItems(r *Report, files map[string][]byte, items []map[string]any) {
	referenced := map[string]bool{}

	for _, item := range items {
		key := str(item["__key"])
		file := str(item["file"])

		if file == "" {
			r.fail("deployment-items", fmt.Sprintf("artifact %q has no file", key), "")
			continue
		}
		if _, ok := files[file]; !ok {
			r.fail("artifact-files",
				fmt.Sprintf("artifact %q points at %s, which is not in the package", key, file),
				"every artifact file is checked for existence during upload")
			continue
		}
		referenced[file] = true

		props, _ := item["properties"].(map[string]any)
		if props == nil {
			r.fail("deployment-items", fmt.Sprintf("artifact %q has no properties block", key), "")
			continue
		}

		if got := str(props["artifact_type"]); got != ArtifactTypeHelm {
			r.fail("deployment-items",
				fmt.Sprintf("artifact %q has artifact_type %q", key, got),
				fmt.Sprintf("only %q is uploaded to the chart repository; anything else is silently skipped", ArtifactTypeHelm))
		}
		if got := str(props["target_server"]); got != TargetServerChart {
			r.warn("deployment-items",
				fmt.Sprintf("artifact %q has target_server %q", key, got),
				fmt.Sprintf("the ASD type definition only allows %q", TargetServerChart))
		}
		if str(props["target_server_uri"]) == "" {
			r.fail("deployment-items",
				fmt.Sprintf("artifact %q has no target_server_uri", key),
				"this is the address the chart is POSTed to; an empty value makes the upload fail during priming")
		}
		if str(props["item_id"]) == "" {
			r.warn("deployment-items", fmt.Sprintf("artifact %q has no item_id", key), "")
		}
	}

	for name := range files {
		if strings.HasPrefix(name, HelmDir+"/") && strings.HasSuffix(name, ".tgz") && !referenced[name] {
			r.warn("artifact-files",
				fmt.Sprintf("%s is in the package but no artifact refers to it", name),
				"an unreferenced chart is never uploaded and never deployed")
		}
	}
}

// checkResourceNames guards the assumption the platform makes when it turns a
// resource path into the identifier an rApp instance refers to: it slices the
// name at the last dot, so a second dot silently truncates the identifier.
func checkResourceNames(r *Report, files map[string][]byte) {
	seen := map[string]string{}

	for name := range files {
		dir := path.Dir(name)
		if !isResourceDir(dir) {
			continue
		}
		base := path.Base(name)
		r.Resources = append(r.Resources, name)

		if strings.Count(base, ".") != 1 {
			r.fail("resource-names",
				fmt.Sprintf("%s must contain exactly one dot", name),
				"the platform derives the resource identifier by cutting at the last dot, so extra dots corrupt it")
			continue
		}

		id := dir + "/" + strings.TrimSuffix(base, path.Ext(base))
		if prev, dup := seen[id]; dup {
			r.fail("resource-names",
				fmt.Sprintf("%s and %s resolve to the same resource identifier", prev, name),
				"rename one of them")
		}
		seen[id] = name
	}
	sort.Strings(r.Resources)
}

func isResourceDir(dir string) bool {
	for _, d := range ResourceDirs {
		if dir == d {
			return true
		}
	}
	return false
}

// checkAcmInstances confirms every chart an automation composition instance
// asks for is actually shipped, since a mismatch only shows up as a failed
// deployment long after onboarding succeeded.
func checkAcmInstances(r *Report, files map[string][]byte) {
	shipped := map[string]bool{}
	for name := range files {
		if strings.HasPrefix(name, HelmDir+"/") && strings.HasSuffix(name, ".tgz") {
			shipped[strings.TrimSuffix(path.Base(name), ".tgz")] = true
		}
	}
	if len(shipped) == 0 {
		return
	}

	for name, data := range files {
		if path.Dir(name) != AcmInstancesDir {
			continue
		}

		var doc struct {
			Elements map[string]struct {
				Properties struct {
					Chart struct {
						ChartID struct {
							Name    string `json:"name"`
							Version string `json:"version"`
						} `json:"chartId"`
					} `json:"chart"`
				} `json:"properties"`
			} `json:"elements"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			r.fail("acm-instance", fmt.Sprintf("%s is not valid JSON: %v", name, err), "")
			continue
		}

		for id, el := range doc.Elements {
			chart := el.Properties.Chart.ChartID
			if chart.Name == "" || chart.Version == "" {
				continue
			}
			want := chart.Name + "-" + chart.Version
			if !shipped[want] {
				r.fail("acm-instance",
					fmt.Sprintf("%s element %s asks for chart %s, which is not in the package", name, id, want),
					"the chart name and version here must match a packaged archive exactly")
			}
		}
	}
}
