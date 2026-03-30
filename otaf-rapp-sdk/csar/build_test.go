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
