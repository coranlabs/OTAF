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
