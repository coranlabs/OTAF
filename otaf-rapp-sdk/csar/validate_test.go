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
	"path/filepath"
	"strings"
	"testing"
)

const (
	fixtureChart = HelmDir + "/demo-0.1.0.tgz"
	fixtureMeta  = "TOSCA-Meta-File-Version: 1.0\nCSAR-Version: 1.0\nCreated-By: test\n" +
		"Entry-Definitions: " + AsdPath + "\n"
)

func fixtureAsd(artifacts string) string {
	return `tosca_definitions_version: tosca_simple_yaml_1_2
description: demo
topology_template:
  node_templates:
    applicationServiceDescriptor:
      type: tosca.nodes.asd
      properties:
        descriptor_id: 0f9a6b2c-1d3e-4f50-8a1b-2c3d4e5f6071
        descriptor_invariant_id: 1a2b3c4d-5e6f-4071-8293-a4b5c6d7e8f9
        descriptor_version: "0.1.0"
        schema_version: "2.0"
        provider: test
        application_name: demo
        application_version: "0.1.0"
` + artifacts
}

const goodArtifacts = `      artifacts:
        demo:
          type: tosca.artifacts.asd.deploymentItem
          file: "` + fixtureChart + `"
          properties:
            artifact_type: "helm_chart"
            target_server: "chartmuseum"
            target_server_uri: "http://chartmuseum.nonrtric:8080/api/charts"
            item_id: "1"
`

const acmInstance = `{
  "name": "demo-instance",
  "elements": {
    "e1": {
      "properties": {
        "chart": { "chartId": { "name": "demo", "version": "0.1.0" } }
      }
    }
  }
}`

func baseline() map[string][]byte {
	return map[string][]byte{
		ToscaMetaPath:                           []byte(fixtureMeta),
		AsdPath:                                 []byte(fixtureAsd(goodArtifacts)),
		AcmDefinition:                           []byte(`{"topology_template":{}}`),
		AcmInstancesDir + "/demo-instance.json": []byte(acmInstance),
		fixtureChart:                            []byte("not really a chart"),
	}
}

// fixture writes a package, applying mutations first. A nil value deletes.
func fixture(t *testing.T, name string, mutate map[string][]byte) string {
	t.Helper()

	contents := baseline()
	for k, v := range mutate {
		if v == nil {
			delete(contents, k)
			continue
		}
		contents[k] = v
	}

	path := filepath.Join(t.TempDir(), name)
	if err := writeZip(path, contents); err != nil {
		t.Fatal(err)
	}
	return path
}

func validate(t *testing.T, path string) *Report {
	t.Helper()
	r, err := Validate(path)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func hasRule(r *Report, rule string, severity Severity) bool {
	for _, f := range r.Findings {
		if f.Rule == rule && f.Severity == severity {
			return true
		}
	}
	return false
}

func summarise(r *Report) string {
	var parts []string
	for _, f := range r.Findings {
		parts = append(parts, string(f.Severity)+":"+f.Rule)
	}
	return strings.Join(parts, ", ")
}

func TestValidPackagePasses(t *testing.T) {
	r := validate(t, fixture(t, "demo-0.1.0.csar", nil))
	if !r.OK() {
		t.Fatalf("a well-formed package should pass, got: %s", summarise(r))
	}
	if r.DeploymentItems != 1 {
		t.Errorf("deployment items = %d, want 1", r.DeploymentItems)
	}
	if r.ApplicationName != "demo" {
		t.Errorf("application name = %q, want demo", r.ApplicationName)
	}
}

func TestExtensionIsChecked(t *testing.T) {
	r := validate(t, fixture(t, "demo-0.1.0.zip", nil))
	if !hasRule(r, "package-name", SeverityError) {
		t.Errorf("expected a package-name error, got: %s", summarise(r))
	}
}

func TestRequiredFilesAreChecked(t *testing.T) {
	for _, missing := range []string{AcmDefinition, ToscaMetaPath} {
		r := validate(t, fixture(t, "demo-0.1.0.csar", map[string][]byte{missing: nil}))
		if !hasRule(r, "required-files", SeverityError) {
			t.Errorf("dropping %s should raise required-files, got: %s", missing, summarise(r))
		}
	}
}

func TestEntryDefinitionsMustResolve(t *testing.T) {
	noEntry := "TOSCA-Meta-File-Version: 1.0\nCSAR-Version: 1.0\nCreated-By: test\n"
	r := validate(t, fixture(t, "demo-0.1.0.csar", map[string][]byte{ToscaMetaPath: []byte(noEntry)}))
	if !hasRule(r, "tosca-entry-definitions", SeverityError) {
		t.Errorf("expected tosca-entry-definitions error, got: %s", summarise(r))
	}

	dangling := fixtureMeta[:strings.Index(fixtureMeta, "Entry-Definitions:")] + "Entry-Definitions: Definitions/absent.yaml\n"
	r = validate(t, fixture(t, "demo-0.1.0.csar", map[string][]byte{ToscaMetaPath: []byte(dangling)}))
	if !hasRule(r, "tosca-entry-definitions", SeverityError) {
		t.Errorf("expected tosca-entry-definitions error for a dangling pointer, got: %s", summarise(r))
	}
}

func TestDescriptorIdentifiersAreRequired(t *testing.T) {
	blank := strings.Replace(fixtureAsd(goodArtifacts),
		"descriptor_id: 0f9a6b2c-1d3e-4f50-8a1b-2c3d4e5f6071", `descriptor_id: ""`, 1)

	r := validate(t, fixture(t, "demo-0.1.0.csar", map[string][]byte{AsdPath: []byte(blank)}))
	if !hasRule(r, "asd-descriptor", SeverityError) {
		t.Errorf("expected asd-descriptor error, got: %s", summarise(r))
	}
}

func TestNonUuidDescriptorWarns(t *testing.T) {
	legacy := strings.Replace(fixtureAsd(goodArtifacts),
		"descriptor_id: 0f9a6b2c-1d3e-4f50-8a1b-2c3d4e5f6071", "descriptor_id: demo-rapp-v1", 1)

	r := validate(t, fixture(t, "demo-0.1.0.csar", map[string][]byte{AsdPath: []byte(legacy)}))
	if !hasRule(r, "asd-descriptor", SeverityWarn) {
		t.Errorf("expected an asd-descriptor warning, got: %s", summarise(r))
	}
	if !r.OK() {
		t.Error("a non-UUID identifier is a warning, not a rejection")
	}
}

// Deployment items declared in the properties block are not read, and the
// resulting empty list stops the package being primed.
func TestDeploymentItemsInPropertiesAreRejected(t *testing.T) {
	legacy := fixtureAsd(`        deploymentItems:
          - artifactId: demo
            artifactType: HELM
            artifactLocation: ` + fixtureChart + `
`)

	r := validate(t, fixture(t, "demo-0.1.0.csar", map[string][]byte{AsdPath: []byte(legacy)}))
	if !hasRule(r, "deployment-items", SeverityError) {
		t.Errorf("expected a deployment-items error, got: %s", summarise(r))
	}
	if r.DeploymentItems != 0 {
		t.Errorf("deployment items = %d, want 0", r.DeploymentItems)
	}
}

func TestMissingArtifactsIsRejected(t *testing.T) {
	r := validate(t, fixture(t, "demo-0.1.0.csar", map[string][]byte{AsdPath: []byte(fixtureAsd(""))}))
	if !hasRule(r, "deployment-items", SeverityError) {
		t.Errorf("expected a deployment-items error, got: %s", summarise(r))
	}
}

func TestArtifactFileMustExist(t *testing.T) {
	r := validate(t, fixture(t, "demo-0.1.0.csar", map[string][]byte{fixtureChart: nil}))
	if !hasRule(r, "artifact-files", SeverityError) {
		t.Errorf("expected an artifact-files error, got: %s", summarise(r))
	}
}

func TestTargetServerUriIsRequired(t *testing.T) {
	blank := strings.Replace(fixtureAsd(goodArtifacts),
		`target_server_uri: "http://chartmuseum.nonrtric:8080/api/charts"`, `target_server_uri: ""`, 1)

	r := validate(t, fixture(t, "demo-0.1.0.csar", map[string][]byte{AsdPath: []byte(blank)}))
	if !hasRule(r, "deployment-items", SeverityError) {
		t.Errorf("expected a deployment-items error, got: %s", summarise(r))
	}
}

// The platform cuts a resource name at its last dot, so a second dot silently
// truncates the identifier an rApp instance has to refer to.
func TestResourceNamesRejectExtraDots(t *testing.T) {
	r := validate(t, fixture(t, "demo-0.1.0.csar", map[string][]byte{
		AcmInstancesDir + "/demo.v2.json": []byte(acmInstance),
	}))
	if !hasRule(r, "resource-names", SeverityError) {
		t.Errorf("expected a resource-names error, got: %s", summarise(r))
	}
}

func TestAcmInstanceChartMustBeShipped(t *testing.T) {
	mismatch := strings.Replace(acmInstance, `"version": "0.1.0"`, `"version": "9.9.9"`, 1)

	r := validate(t, fixture(t, "demo-0.1.0.csar", map[string][]byte{
		AcmInstancesDir + "/demo-instance.json": []byte(mismatch),
	}))
	if !hasRule(r, "acm-instance", SeverityError) {
		t.Errorf("expected an acm-instance error, got: %s", summarise(r))
	}
}

// The mismatch that reports nothing: the platform registers a type under its
// file's base name, and a job asking for anything else simply never starts.
func TestDmeConsumerMustNameAnExistingType(t *testing.T) {
	r := validate(t, fixture(t, "demo-0.1.0.csar", map[string][]byte{
		DmeConsumerTypesDir + "/ves-notify-file-ready.json": []byte(`{"info_job_data_schema":{}}`),
		DmeConsumersDir + "/my-consumer.json": []byte(
			`{"info_type_id":"VES_NOTIFY_FILE_READY","job_owner":"me","job_result_uri":"http://me/data","job_definition":{}}`),
	}))

	if !hasRule(r, "dme-info-types", SeverityError) {
		t.Errorf("expected a dme-info-types error, got: %s", summarise(r))
	}
}

func TestDmeConsumerMatchingTheBasenamePasses(t *testing.T) {
	r := validate(t, fixture(t, "demo-0.1.0.csar", map[string][]byte{
		DmeConsumerTypesDir + "/ves-notify-file-ready.json": []byte(`{"info_job_data_schema":{}}`),
		DmeConsumersDir + "/my-consumer.json": []byte(
			`{"info_type_id":"ves-notify-file-ready","job_owner":"me","job_result_uri":"http://me/data","job_definition":{}}`),
	}))

	if !r.OK() {
		t.Errorf("a matching id should pass, got: %s", summarise(r))
	}
}

func TestDmeProducerMustNameAnExistingType(t *testing.T) {
	r := validate(t, fixture(t, "demo-0.1.0.csar", map[string][]byte{
		DmeProducerTypesDir + "/cell-state.json": []byte(`{"info_job_data_schema":{}}`),
		DmeProducersDir + "/my-producer.json": []byte(
			`{"info_producer_id":"p","supported_info_types":["cell-state","typo-state"]}`),
	}))

	if !hasRule(r, "dme-info-types", SeverityError) {
		t.Errorf("expected a dme-info-types error for the unknown type, got: %s", summarise(r))
	}
}
