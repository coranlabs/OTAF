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
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/csar"
)

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emit the report as JSON")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: rappctl validate <file.csar> [flags]\n\n")
		fs.PrintDefaults()
	}
	target, rest := splitPositional(args)
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if target == "" {
		target = fs.Arg(0)
	}
	if target == "" {
		fs.Usage()
		return fmt.Errorf("a .csar path is required")
	}

	report, err := csar.Validate(target)
	if err != nil {
		return err
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		printReport(report, true)
	}

	if !report.OK() {
		return fmt.Errorf("package would be rejected during onboarding")
	}
	return nil
}

func printReport(r *csar.Report, verbose bool) {
	if verbose {
		fmt.Printf("Package          %s\n", r.Package)
		if r.ApplicationName != "" {
			fmt.Printf("Application      %s %s\n", r.ApplicationName, r.ApplicationVersion)
			fmt.Printf("Descriptor       %s\n", r.DescriptorID)
			fmt.Printf("Invariant        %s\n", r.DescriptorInvariantID)
		}
		fmt.Printf("Deployment items %d\n", r.DeploymentItems)
		if len(r.Resources) > 0 {
			fmt.Println("Resources")
			for _, res := range r.Resources {
				fmt.Println("  ", res)
			}
		}
		fmt.Println()
	}

	errors, warnings := r.Errors(), r.Warnings()

	for _, f := range errors {
		fmt.Printf("ERROR  [%s] %s\n", f.Rule, f.Message)
		if f.Hint != "" {
			fmt.Printf("       %s\n", f.Hint)
		}
	}
	for _, f := range warnings {
		fmt.Printf("WARN   [%s] %s\n", f.Rule, f.Message)
		if f.Hint != "" {
			fmt.Printf("       %s\n", f.Hint)
		}
	}

	switch {
	case len(errors) > 0:
		fmt.Printf("\n%d error(s), %d warning(s)\n", len(errors), len(warnings))
	case len(warnings) > 0:
		fmt.Printf("\nReady to onboard, with %d warning(s)\n", len(warnings))
	default:
		fmt.Println("Ready to onboard")
	}
}
