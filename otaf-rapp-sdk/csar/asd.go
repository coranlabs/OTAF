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
	"fmt"
	"sort"
	"strings"
	"time"
)

type deployItem struct {
	Key             string
	File            string
	ItemID          int
	TargetServerURI string
}

func renderAsd(s *Spec, items []deployItem) string {
	var b strings.Builder

	b.WriteString("tosca_definitions_version: tosca_simple_yaml_1_2\n")
	b.WriteString(fmt.Sprintf("description: %s\n\n", yamlScalar(descriptionOf(s))))
	b.WriteString("imports:\n  - asd_types.yaml\n\n")
	b.WriteString("topology_template:\n")
	b.WriteString("  node_templates:\n")
	b.WriteString("    applicationServiceDescriptor:\n")
	b.WriteString("      type: tosca.nodes.asd\n")
	b.WriteString(fmt.Sprintf("      description: %s\n", yamlScalar(s.Name)))
	b.WriteString("      properties:\n")
	b.WriteString(fmt.Sprintf("        descriptor_id: %s\n", s.DescriptorID))
	b.WriteString(fmt.Sprintf("        descriptor_invariant_id: %s\n", s.DescriptorInvariantID))
	b.WriteString(fmt.Sprintf("        descriptor_version: %s\n", yamlString(s.Version)))
	b.WriteString(fmt.Sprintf("        schema_version: %s\n", yamlString(s.SchemaVersion)))
	b.WriteString(fmt.Sprintf("        function_description: %s\n", yamlScalar(descriptionOf(s))))
	b.WriteString(fmt.Sprintf("        provider: %s\n", yamlScalar(s.Provider)))
	b.WriteString(fmt.Sprintf("        application_name: %s\n", yamlScalar(s.Name)))
	b.WriteString(fmt.Sprintf("        application_version: %s\n", yamlString(s.Version)))
	b.WriteString("      artifacts:\n")

	for _, it := range items {
		b.WriteString(fmt.Sprintf("        %s:\n", it.Key))
		b.WriteString("          type: tosca.artifacts.asd.deploymentItem\n")
		b.WriteString(fmt.Sprintf("          file: %s\n", yamlScalar(it.File)))
		b.WriteString("          properties:\n")
		b.WriteString(fmt.Sprintf("            artifact_type: %s\n", yamlString(ArtifactTypeHelm)))
		b.WriteString(fmt.Sprintf("            target_server: %s\n", yamlString(TargetServerChart)))
		b.WriteString(fmt.Sprintf("            target_server_uri: %s\n", yamlString(it.TargetServerURI)))
		b.WriteString(fmt.Sprintf("            item_id: %s\n", yamlString(fmt.Sprint(it.ItemID))))
	}

	return b.String()
}

func descriptionOf(s *Spec) string {
	if strings.TrimSpace(s.Description) != "" {
		return s.Description
	}
	return s.Name
}

const asdTypes = `tosca_definitions_version: tosca_simple_yaml_1_2
description: ASD types definitions version 0.1
node_types:
  tosca.nodes.asd:
    derived_from: tosca.nodes.Root
    description: "The ASD node type"
    version: 0.1
    properties:
      descriptor_id:
        type: string
        required: true
        description: Identifier of this ASD, in UUID format as specified in RFC 4122
      descriptor_invariant_id:
        type: string
        required: true
        description: >
          Identifier of this descriptor in a version independent manner, invariant
          across versions of the ASD, in UUID format as specified in RFC 4122
      descriptor_version:
        type: string
        required: true
        description: Identifies the version of the ASD
      schema_version:
        type: string
        required: true
        description: Identifies the version of this ASD's schema
      function_description:
        type: string
        required: false
        description: Description of the application service described by this ASD
      provider:
        type: string
        required: true
        description: Identifies the provider of the ASD
      application_name:
        type: string
        required: true
        description: Name identifying the application service described by this ASD
      application_version:
        type: string
        required: true
        description: Identifies the version of the application service described by this ASD

artifact_types:
  tosca.artifacts.asd.deploymentItem:
    version: 0.1
    derived_from: tosca.artifacts.Root
    description: "Describes the artifact type of an ASD deployment item"
    file: "Relative path of the artifact in the package"
    properties:
      item_id:
        description: "The identifier of this ASD deployment item"
        required: true
        type: string
      artifact_type:
        description: "Artifact type of the deployment item"
        required: true
        type: string
        constraints:
          - valid_values: ["helm_chart"]
      target_server:
        description: "Target server the artifact is uploaded to"
        required: true
        type: string
        constraints:
          - valid_values: ["chartmuseum"]
      target_server_uri:
        description: "URI of the target server"
        required: true
        type: string
`
