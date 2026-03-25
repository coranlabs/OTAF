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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const SpecFile = "rapp-package.yaml"

// Spec declares everything needed to assemble the rApp package. It is written
// once by the scaffolder and then edited by hand as the rApp grows.
type Spec struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Provider    string `yaml:"provider"`
	Description string `yaml:"description"`

	// Both identifiers must stay stable for the life of the rApp: rApp
	// Manager refuses a package whose descriptor id it has already onboarded,
	// and the invariant id is what ties versions of the same rApp together.
	DescriptorID          string `yaml:"descriptor_id"`
	DescriptorInvariantID string `yaml:"descriptor_invariant_id"`

	SchemaVersion string `yaml:"schema_version"`

	Charts []Chart `yaml:"charts"`

	ResourceDir string `yaml:"resource_dir"`
	OutputDir   string `yaml:"output_dir"`
}

// Chart is one Helm artifact the platform uploads to the chart repository
// before the automation composition deploys it.
type Chart struct {
	// Path is a chart directory to package, or a prebuilt .tgz.
	Path string `yaml:"path"`
	// TargetServerURI is the chart repository the platform pushes to.
	TargetServerURI string `yaml:"target_server_uri"`
}

const DefaultChartMuseum = "http://chartmuseum.nonrtric:8080/api/charts"

func LoadSpec(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var s Spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	s.applyDefaults(filepath.Dir(path))
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}
