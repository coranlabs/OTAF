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
