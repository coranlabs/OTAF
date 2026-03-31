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
