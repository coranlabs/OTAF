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

// Package csar builds and checks the rApp package that rApp Manager onboards
// over R1. The layout and the checks follow what the platform actually reads,
// so a package that passes here is a package it can prime.
package csar

const (
	ToscaMetaPath   = "TOSCA-Metadata/TOSCA.meta"
	AsdPath         = "Definitions/asd.yaml"
	AsdTypesPath    = "Definitions/asd_types.yaml"
	HelmDir         = "Artifacts/Deployment/HELM"
	AcmDefinition   = "Files/Acm/definition/compositions.json"
	AcmInstancesDir = "Files/Acm/instances"

	SmeProvidersDir   = "Files/Sme/providers"
	SmeServiceApisDir = "Files/Sme/serviceapis"
	SmeInvokersDir    = "Files/Sme/invokers"

	DmeProducerTypesDir = "Files/Dme/producerinfotypes"
	DmeConsumerTypesDir = "Files/Dme/consumerinfotypes"
	DmeProducersDir     = "Files/Dme/infoproducers"
	DmeConsumersDir     = "Files/Dme/infoconsumers"

	entryDefinitionsKey = "Entry-Definitions"

	// The only artifact and target values the ASD type definition allows.
	ArtifactTypeHelm    = "helm_chart"
	TargetServerChart   = "chartmuseum"
	defaultSchemaVersio = "2.0"
)

// ResourceDirs are the directories rApp Manager scans for resources it hands
// to the platform services. Every file placed here is picked up by base name.
var ResourceDirs = []string{
	AcmInstancesDir,
	SmeProvidersDir,
	SmeServiceApisDir,
	SmeInvokersDir,
	DmeProducerTypesDir,
	DmeConsumerTypesDir,
	DmeProducersDir,
	DmeConsumersDir,
}
