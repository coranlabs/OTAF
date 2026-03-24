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
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type BuildResult struct {
	Path   string
	Files  []string
	Charts []PackagedChart
	Report *Report
}

// Build assembles the rApp package and checks the result before returning it,
// so a package that builds is a package the platform will accept.
func Build(s *Spec) (*BuildResult, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	contents := map[string][]byte{}

	charts, err := collectCharts(s)
	if err != nil {
		return nil, err
	}
	items := make([]deployItem, 0, len(charts))
	packaged := make([]PackagedChart, 0, len(charts))
	for i, c := range charts {
		archived := path.Join(HelmDir, c.name)
		contents[archived] = c.data
		items = append(items, deployItem{
			Key:             artifactKey(c.name),
			File:            archived,
			ItemID:          i + 1,
			TargetServerURI: c.targetServerURI,
		})

		name, version := splitChartFile(c.name)
		packaged = append(packaged, PackagedChart{
			Name: name, Version: version,
			File: archived, TargetServerURI: c.targetServerURI,
		})
	}

	if err := collectResources(s, contents); err != nil {
		return nil, err
	}
	if _, ok := contents[AcmDefinition]; !ok {
		return nil, fmt.Errorf("%s is missing: the platform will not onboard a package without an automation composition definition", AcmDefinition)
	}

	// Every build gets fresh element ids, so a package can be onboarded again
	// after a failed deploy without tripping the platform's duplicate check.
	for name, body := range contents {
		if path.Dir(name) != AcmInstancesDir {
			continue
		}
		refreshed, err := refreshElementIDs(body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		contents[name] = refreshed
	}

	contents[AsdPath] = []byte(renderAsd(s, items))
	contents[AsdTypesPath] = []byte(asdTypes)
	contents[ToscaMetaPath] = []byte(renderToscaMeta(s))

	sources := make([]string, 0, len(contents)+1)
	for name := range contents {
		sources = append(sources, name)
	}
	sources = append(sources, manifestName(s))
	contents[manifestName(s)] = []byte(renderManifest(s, sources))

	if err := os.MkdirAll(s.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	out := filepath.Join(s.OutputDir, s.CsarName())
	if err := writeZip(out, contents); err != nil {
		return nil, err
	}

	report, err := Validate(out)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(contents))
	for name := range contents {
		names = append(names, name)
	}
	sort.Strings(names)

	return &BuildResult{Path: out, Files: names, Charts: packaged, Report: report}, nil
}

type packagedChart struct {
	name            string
	data            []byte
	targetServerURI string
}

func collectCharts(s *Spec) ([]packagedChart, error) {
	var out []packagedChart
	seen := map[string]bool{}

	for _, c := range s.Charts {
		info, err := os.Stat(c.Path)
		if err != nil {
			return nil, fmt.Errorf("chart %s: %w", c.Path, err)
		}

		var name string
		var data []byte
		if info.IsDir() {
			name, data, err = packageChart(c.Path)
		} else {
			name = filepath.Base(c.Path)
			data, err = os.ReadFile(c.Path)
		}
		if err != nil {
			return nil, err
		}
		if !strings.HasSuffix(name, ".tgz") {
			return nil, fmt.Errorf("chart %s did not produce a .tgz archive", c.Path)
		}
		if seen[name] {
			return nil, fmt.Errorf("two charts both package as %s: give them distinct names or versions", name)
		}
		seen[name] = true

		out = append(out, packagedChart{name: name, data: data, targetServerURI: c.TargetServerURI})
	}
	return out, nil
}

func packageChart(dir string) (string, []byte, error) {
	if _, err := exec.LookPath("helm"); err != nil {
		return "", nil, fmt.Errorf("helm is required to package the chart at %s (or point the spec at a prebuilt .tgz)", dir)
	}

	tmp, err := os.MkdirTemp("", "rapp-chart-")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(tmp)

	cmd := exec.Command("helm", "package", dir, "-d", tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", nil, fmt.Errorf("helm package %s: %w: %s", dir, err, strings.TrimSpace(string(out)))
	}

	entries, err := os.ReadDir(tmp)
	if err != nil {
		return "", nil, err
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tgz") {
			data, err := os.ReadFile(filepath.Join(tmp, e.Name()))
			return e.Name(), data, err
		}
	}
	return "", nil, fmt.Errorf("helm package %s produced no archive", dir)
}
