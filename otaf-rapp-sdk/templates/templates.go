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

// Package templates renders a new rApp repository: source, chart and the rApp
// package descriptors, all consistent with each other from the first commit.
package templates

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	rappsdk "github.com/coranlabs/OTAF/otaf-rapp-sdk"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/csar"
)

//go:embed all:rapp
var files embed.FS

const namePlaceholder = "__name__"

// Scaffold is everything the generated rApp needs to be internally consistent.
type Scaffold struct {
	Name        string
	Module      string
	Provider    string
	Description string
	Version     string
	Namespace   string
	ImageRepo   string
	NodePort    int

	DescriptorID          string
	DescriptorInvariantID string
	ElementID             string

	SDKModule   string
	SDKVersion  string
	ChartMuseum string

	// SDKReplace points the generated module at a local SDK checkout, for
	// working against an unreleased version.
	SDKReplace string
}

func (s *Scaffold) applyDefaults() error {
	if s.Name == "" {
		return fmt.Errorf("rApp name is required")
	}
	if s.Module == "" {
		s.Module = s.Name
	}
	if s.Provider == "" {
		s.Provider = "coRAN Labs"
	}
	if s.Description == "" {
		s.Description = fmt.Sprintf("%s rApp for the Non-RT RIC", s.Name)
	}
	if s.Version == "" {
		s.Version = "0.1.0"
	}
	if s.Namespace == "" {
		s.Namespace = "nonrtric"
	}
	if s.ImageRepo == "" {
		s.ImageRepo = "localhost:5000/" + s.Name
	}
	if s.NodePort == 0 {
		s.NodePort = 30980
	}
	if s.ChartMuseum == "" {
		s.ChartMuseum = csar.DefaultChartMuseum
	}
	s.SDKModule = "github.com/coranlabs/OTAF/otaf-rapp-sdk"
	s.SDKVersion = rappsdk.Version

	var err error
	if s.DescriptorID == "" {
		if s.DescriptorID, err = UUID(); err != nil {
			return err
		}
	}
	if s.DescriptorInvariantID == "" {
		if s.DescriptorInvariantID, err = UUID(); err != nil {
			return err
		}
	}
	if s.ElementID == "" {
		if s.ElementID, err = UUID(); err != nil {
			return err
		}
	}
	return nil
}

// Render writes the scaffold into dir. It refuses to touch an existing file so
// re-running it can never clobber work in progress.
func Render(dir string, s *Scaffold) ([]string, error) {
	if err := s.applyDefaults(); err != nil {
		return nil, err
	}

	var written []string
	err := fs.WalkDir(files, "rapp", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel := strings.TrimPrefix(p, "rapp/")
		rel = strings.TrimSuffix(rel, ".tmpl")
		rel = strings.ReplaceAll(rel, namePlaceholder, s.Name)
		target := filepath.Join(dir, filepath.FromSlash(rel))

		if _, statErr := os.Stat(target); statErr == nil {
			return fmt.Errorf("%s already exists", target)
		}

		body, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		rendered, err := execute(rel, string(body), s)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(rel, ".sh") {
			mode = 0o755
		}
		if err := os.WriteFile(target, rendered, mode); err != nil {
			return err
		}
		written = append(written, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(written)
	return written, nil
}

// Delimiters avoid Go's default braces so Helm templates in the scaffold pass
// through untouched.
func execute(name, body string, s *Scaffold) ([]byte, error) {
	t, err := template.New(name).Delims("[[", "]]").Option("missingkey=error").Parse(body)
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, s); err != nil {
		return nil, fmt.Errorf("render template %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

// UUID returns a random RFC 4122 version 4 identifier.
func UUID() (string, error) { return csar.UUID() }
