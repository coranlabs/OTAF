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

package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/csar"
)

func render(t *testing.T) (string, *Scaffold) {
	t.Helper()
	dir := t.TempDir()
	s := &Scaffold{Name: "demo-rapp", Module: "github.com/example/demo-rapp"}
	if _, err := Render(dir, s); err != nil {
		t.Fatal(err)
	}
	return dir, s
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestRenderProducesACompletePackageLayout(t *testing.T) {
	dir, _ := render(t)

	want := []string{
		"go.mod",
		"Dockerfile",
		"README.md",
		"cmd/demo-rapp/main.go",
		"internal/logic/logic.go",
		"config/rapp.yaml",
		csar.SpecFile,
		"deploy/helm/demo-rapp/Chart.yaml",
		"deploy/helm/demo-rapp/values.yaml",
		"rapp-package/Files/Acm/definition/compositions.json",
		"rapp-package/Files/Acm/instances/demo-rapp-instance.json",
		"rapp-package/Files/Dme/infoproducers/demo-rapp-producer.json",
		"rapp-package/Files/Dme/producerinfotypes/demo-rapp-state.json",
		"rapp-package/Files/Sme/serviceapis/demo-rapp-api.json",
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("scaffold is missing %s", rel)
		}
	}
}

func TestIdentifiersAreGeneratedAndDistinct(t *testing.T) {
	_, s := render(t)

	if !isUUIDish(s.DescriptorID) || !isUUIDish(s.DescriptorInvariantID) {
		t.Fatalf("identifiers should be UUIDs, got %q and %q", s.DescriptorID, s.DescriptorInvariantID)
	}
	if s.DescriptorID == s.DescriptorInvariantID {
		t.Error("descriptor id and invariant id must differ")
	}
}

func TestGeneratedSpecIsUsable(t *testing.T) {
	dir, _ := render(t)

	spec, err := csar.LoadSpec(filepath.Join(dir, csar.SpecFile))
	if err != nil {
		t.Fatalf("the generated package descriptor should load cleanly: %v", err)
	}
	if spec.Name != "demo-rapp" {
		t.Errorf("name = %q, want demo-rapp", spec.Name)
	}
}

// The chart the composition instance asks for has to be the chart the repo
// actually builds, or deployment fails long after onboarding succeeded.
func TestChartVersionMatchesCompositionInstance(t *testing.T) {
	dir, s := render(t)

	chart := read(t, dir, "deploy/helm/demo-rapp/Chart.yaml")
	instance := read(t, dir, "rapp-package/Files/Acm/instances/demo-rapp-instance.json")

	if !strings.Contains(chart, `version: "`+s.Version+`"`) {
		t.Errorf("chart does not declare version %s", s.Version)
	}
	if !strings.Contains(instance, `"version": "`+s.Version+`"`) {
		t.Errorf("composition instance does not ask for chart version %s", s.Version)
	}
	if !strings.Contains(instance, `"name": "demo-rapp"`) {
		t.Error("composition instance does not ask for the chart this repo builds")
	}
}

// A version left unquoted in YAML is read as a number, so 1.10 becomes 1.1 and
// Helm then packages a chart nobody asked for.
func TestVersionsAreQuotedEverywhere(t *testing.T) {
	dir := t.TempDir()
	if _, err := Render(dir, &Scaffold{Name: "demo-rapp", Version: "1.10"}); err != nil {
		t.Fatal(err)
	}

	for _, file := range []string{
		"deploy/helm/demo-rapp/Chart.yaml",
		"rapp-package.yaml",
		"config/rapp.yaml",
	} {
		body := read(t, dir, file)
		if strings.Contains(body, "version: 1.10\n") {
			t.Errorf("%s leaves a version unquoted", file)
		}
		if !strings.Contains(body, `"1.10"`) {
			t.Errorf("%s does not carry the version as a quoted string", file)
		}
	}
}
