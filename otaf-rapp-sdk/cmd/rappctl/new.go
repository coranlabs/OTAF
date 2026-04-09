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

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/templates"
)

var nameRule = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)

func runNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	out := fs.String("out", "", "directory to create (default: ./<name>)")
	module := fs.String("module", "", "Go module path (default: the rApp name)")
	provider := fs.String("provider", "", "organisation shown as the rApp provider")
	description := fs.String("description", "", "one-line description")
	version := fs.String("version", "0.1.0", "initial rApp version")
	namespace := fs.String("namespace", "nonrtric", "Kubernetes namespace the rApp deploys into")
	image := fs.String("image", "", "container image repository")
	nodePort := fs.Int("node-port", 30980, "NodePort for reaching the rApp during bring-up")
	sdkPath := fs.String("sdk-path", "", "resolve the SDK from a local checkout instead of the module proxy")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: rappctl new <name> [flags]\n\n")
		fs.PrintDefaults()
	}
	name, rest := splitPositional(args)
	if err := fs.Parse(rest); err != nil {
		return err
	}

	if name == "" {
		name = fs.Arg(0)
	}
	if name == "" {
		fs.Usage()
		return fmt.Errorf("rApp name is required")
	}
	if !nameRule.MatchString(name) {
		return fmt.Errorf("rApp name %q must be lower-case letters, digits and hyphens, starting and ending with an alphanumeric", name)
	}

	dir := *out
	if dir == "" {
		dir = name
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	scaffold := &templates.Scaffold{
		Name:        name,
		Module:      *module,
		Provider:    *provider,
		Description: *description,
		Version:     *version,
		Namespace:   *namespace,
		ImageRepo:   *image,
		NodePort:    *nodePort,
		SDKReplace:  *sdkPath,
	}

	written, err := templates.Render(abs, scaffold)
	if err != nil {
		return err
	}

	fmt.Printf("Created %s (%d files)\n", abs, len(written))
	for _, f := range written {
		fmt.Println("  ", f)
	}
	fmt.Printf(`
Descriptor id           %s
Descriptor invariant id %s

Both identifiers are recorded in rapp-package.yaml. The platform refuses a
package whose descriptor id it has onboarded before, so keep them stable.

Next:
  cd %s
  go mod tidy
  # put your analysis in internal/logic/logic.go
  rappctl package
`, scaffold.DescriptorID, scaffold.DescriptorInvariantID, dir)
	return nil
}
