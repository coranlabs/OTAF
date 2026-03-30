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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func buildableSpec(t *testing.T) *Spec {
	t.Helper()
	root := t.TempDir()

	chart := filepath.Join(root, "demo-0.1.0.tgz")
	if err := os.WriteFile(chart, []byte("chart archive"), 0o600); err != nil {
		t.Fatal(err)
	}

	acmDir := filepath.Join(root, "rapp-package", "Files", "Acm", "definition")
	instDir := filepath.Join(root, "rapp-package", "Files", "Acm", "instances")
	for _, d := range []string{acmDir, instDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(acmDir, "compositions.json"), []byte(`{"topology_template":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instDir, "demo-instance.json"), []byte(acmInstance), 0o600); err != nil {
		t.Fatal(err)
	}

	return &Spec{
		Name:                  "demo",
		Version:               "0.1.0",
		Provider:              "test",
		Description:           "demo rApp",
		DescriptorID:          "0f9a6b2c-1d3e-4f50-8a1b-2c3d4e5f6071",
		DescriptorInvariantID: "1a2b3c4d-5e6f-4071-8293-a4b5c6d7e8f9",
		SchemaVersion:         "2.0",
		Charts:                []Chart{{Path: chart, TargetServerURI: DefaultChartMuseum}},
		ResourceDir:           filepath.Join(root, "rapp-package"),
		OutputDir:             filepath.Join(root, "dist"),
	}
}

func TestBuildProducesAValidPackage(t *testing.T) {
	result, err := Build(buildableSpec(t))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasSuffix(result.Path, "demo-0.1.0.csar") {
		t.Errorf("package path = %s, want it to end with demo-0.1.0.csar", result.Path)
	}
	if !result.Report.OK() {
		t.Fatalf("a freshly built package must validate, got: %s", summarise(result.Report))
	}

	want := []string{ToscaMetaPath, AsdPath, AsdTypesPath, AcmDefinition, "demo.mf"}
	for _, w := range want {
		found := false
		for _, f := range result.Files {
			if f == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("package is missing %s", w)
		}
	}
}

func TestBuildRequiresACompositionDefinition(t *testing.T) {
	spec := buildableSpec(t)
	if err := os.Remove(filepath.Join(spec.ResourceDir, "Files", "Acm", "definition", "compositions.json")); err != nil {
		t.Fatal(err)
	}

	if _, err := Build(spec); err == nil {
		t.Fatal("expected a build failure when the composition definition is absent")
	}
}

func TestBuildRejectsDuplicateChartNames(t *testing.T) {
	spec := buildableSpec(t)
	spec.Charts = append(spec.Charts, spec.Charts[0])

	if _, err := Build(spec); err == nil {
		t.Fatal("expected a build failure when two charts package to the same name")
	}
}

func TestSpecValidation(t *testing.T) {
	cases := map[string]func(*Spec){
		"no name":            func(s *Spec) { s.Name = "" },
		"no version":         func(s *Spec) { s.Version = "" },
		"no provider":        func(s *Spec) { s.Provider = "" },
		"bad descriptor":     func(s *Spec) { s.DescriptorID = "not-a-uuid" },
		"identical uuids":    func(s *Spec) { s.DescriptorInvariantID = s.DescriptorID },
		"no charts":          func(s *Spec) { s.Charts = nil },
		"chart without uri":  func(s *Spec) { s.Charts[0].TargetServerURI = "" },
		"chart without path": func(s *Spec) { s.Charts[0].Path = "" },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := buildableSpec(t)
			mutate(spec)
			if err := spec.Validate(); err == nil {
				t.Errorf("%s should be rejected", name)
			}
		})
	}
}

// The platform remembers element ids and refuses one it has seen before, so a
// package that shipped the same id every build could only ever be onboarded
// once per environment.
func TestEveryBuildMintsFreshElementIDs(t *testing.T) {
	spec := buildableSpec(t)

	first := elementIDsFromPackage(t, spec)
	if len(first) != 1 {
		t.Fatalf("elements = %d, want 1", len(first))
	}

	second := elementIDsFromPackage(t, spec)
	for id := range first {
		if second[id] {
			t.Errorf("element id %s was reused; the second onboard would be refused", id)
		}
	}
	for id := range second {
		if !isUUID(id) {
			t.Errorf("element id %q is not a UUID", id)
		}
	}
}

// Only the identifiers move: the element still has to point at its definition.
func TestRefreshingIDsKeepsEverythingElse(t *testing.T) {
	spec := buildableSpec(t)
	if _, err := Build(spec); err != nil {
		t.Fatal(err)
	}

	body := fileFromPackage(t, filepath.Join(spec.OutputDir, spec.CsarName()),
		AcmInstancesDir+"/demo-instance.json")

	var doc struct {
		Name     string `json:"name"`
		Elements map[string]struct {
			ID         string `json:"id"`
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
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}

	if doc.Name != "demo-instance" {
		t.Errorf("instance name = %q, want it untouched", doc.Name)
	}
	for key, element := range doc.Elements {
		// The key and the inner id are the same identifier written twice, and
		// the platform reads both.
		if element.ID != key {
			t.Errorf("element key %s carries id %s; they must agree", key, element.ID)
		}
		chart := element.Properties.Chart.ChartID
		if chart.Name != "demo" || chart.Version != "0.1.0" {
			t.Errorf("chart reference was disturbed: %+v", chart)
		}
	}
}

func TestInstanceWithoutElementsIsLeftAlone(t *testing.T) {
	original := []byte(`{"name":"x","elements":{}}`)

	got, err := refreshElementIDs(original)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Errorf("got %s, want the file unchanged", got)
	}
}

func elementIDsFromPackage(t *testing.T, spec *Spec) map[string]bool {
	t.Helper()

	if _, err := Build(spec); err != nil {
		t.Fatal(err)
	}
	body := fileFromPackage(t, filepath.Join(spec.OutputDir, spec.CsarName()),
		AcmInstancesDir+"/demo-instance.json")

	var doc struct {
		Elements map[string]json.RawMessage `json:"elements"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}

	out := map[string]bool{}
	for id := range doc.Elements {
		out[id] = true
	}
	return out
}

func fileFromPackage(t *testing.T, csarPath, want string) []byte {
	t.Helper()

	zr, err := zip.OpenReader(csarPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name != want {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		body, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	t.Fatalf("%s is not in the package", want)
	return nil
}

func TestSplitChartFile(t *testing.T) {
	cases := map[string][2]string{
		"demo-0.1.0.tgz":                    {"demo", "0.1.0"},
		"cell-watch-1.10.tgz":               {"cell-watch", "1.10"},
		"a-b-c-2.0.0-rc1.tgz":               {"a-b-c", "2.0.0-rc1"},
		"Artifacts/Deployment/HELM/x-1.tgz": {"x", "1"},
	}
	for file, want := range cases {
		name, version := splitChartFile(file)
		if name != want[0] || version != want[1] {
			t.Errorf("split(%q) = %q, %q; want %q, %q", file, name, version, want[0], want[1])
		}
	}
}

func TestGeneratedAsdKeepsDeploymentItemsInArtifacts(t *testing.T) {
	spec := buildableSpec(t)
	asd := renderAsd(spec, []deployItem{{
		Key:             "demo",
		File:            HelmDir + "/demo-0.1.0.tgz",
		ItemID:          1,
		TargetServerURI: DefaultChartMuseum,
	}})

	if !strings.Contains(asd, "      artifacts:") {
		t.Error("generated ASD must declare an artifacts block")
	}
	if strings.Contains(asd, "deploymentItems") {
		t.Error("generated ASD must not use the properties.deploymentItems form, which is never read")
	}
}

// Left unquoted, YAML reads a version such as 1.10 as the number 1.1 and the
// package ends up declaring a version nobody wrote.
func TestVersionsSurviveYamlRoundTrip(t *testing.T) {
	spec := buildableSpec(t)
	spec.Version = "1.10"

	var doc map[string]any
	if err := yaml.Unmarshal([]byte(renderAsd(spec, nil)), &doc); err != nil {
		t.Fatal(err)
	}

	props, _ := dig(doc, "topology_template", "node_templates", "applicationServiceDescriptor", "properties").(map[string]any)
	if props == nil {
		t.Fatal("generated ASD has no properties block")
	}

	for _, key := range []string{"descriptor_version", "application_version", "schema_version"} {
		got, ok := props[key].(string)
		if !ok {
			t.Errorf("%s came back as %T, want a string", key, props[key])
			continue
		}
		if key != "schema_version" && got != "1.10" {
			t.Errorf("%s = %q, want 1.10", key, got)
		}
	}
}
