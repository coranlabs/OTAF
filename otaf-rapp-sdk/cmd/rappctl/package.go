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
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/csar"
)

func runPackage(args []string) error {
	fs := flag.NewFlagSet("package", flag.ExitOnError)
	spec := fs.String("spec", csar.SpecFile, "package descriptor to build from")
	quiet := fs.Bool("quiet", false, "print only the resulting package path")
	checkCharts := fs.Bool("check-charts", false,
		"ask the chart repository whether it already publishes these versions")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: rappctl package [flags]\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	s, err := csar.LoadSpec(*spec)
	if err != nil {
		return err
	}

	result, err := csar.Build(s)
	if err != nil {
		return err
	}

	if *checkCharts {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result.Report.Findings = append(result.Report.Findings,
			csar.CheckChartRepository(ctx, result.Charts, nil)...)
	}

	if *quiet {
		fmt.Println(result.Path)
	} else {
		fmt.Printf("Built %s\n\n", result.Path)
		for _, f := range result.Files {
			fmt.Println("  ", f)
		}
		fmt.Println()
	}

	printReport(result.Report, false)
	if !result.Report.OK() {
		return fmt.Errorf("package would be rejected during onboarding")
	}
	return nil
}
