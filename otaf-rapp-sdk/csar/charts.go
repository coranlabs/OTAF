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
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
)

type PackagedChart struct {
	Name            string
	Version         string
	File            string
	TargetServerURI string
}

// splitChartFile recovers the name and version from an archive named the way
// helm names them, `<name>-<version>.tgz`. Both halves may contain hyphens, so
// the split is at the last hyphen followed by a digit.
func splitChartFile(file string) (name, version string) {
	base := strings.TrimSuffix(file[strings.LastIndex(file, "/")+1:], ".tgz")

	for i := len(base) - 1; i > 0; i-- {
		if base[i] != '-' || i+1 >= len(base) {
			continue
		}
		if unicode.IsDigit(rune(base[i+1])) {
			return base[:i], base[i+1:]
		}
	}
	return base, ""
}

// CheckChartRepository asks the chart repository whether it already holds each
// chart at the version being published.
//
// This matters because priming uploads the packaged charts, and a repository
// that already has that name and version keeps the copy it has. The old chart
// is then what deploys — with the old values, and the old secrets — however
// carefully the new one was rebuilt. Nothing reports it.
//
// It reaches the network, so it is opt-in. A repository that cannot be reached
// produces one warning, not a failure: not being able to check is not the same
// as knowing something is wrong.
func CheckChartRepository(ctx context.Context, charts []PackagedChart, client *http.Client) []Finding {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	var findings []Finding
	for _, chart := range charts {
		if chart.Name == "" || chart.Version == "" || chart.TargetServerURI == "" {
			continue
		}

		url := strings.TrimRight(chart.TargetServerURI, "/") + "/" +
			chart.Name + "/" + chart.Version

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			findings = append(findings, Finding{
				Rule:     "chart-repository",
				Severity: SeverityWarn,
				Message:  fmt.Sprintf("could not reach the chart repository for %s", chart.Name),
				Hint:     "the published chart was not checked; " + err.Error(),
			})
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			findings = append(findings, Finding{
				Rule:     "chart-repository",
				Severity: SeverityWarn,
				Message: fmt.Sprintf("the repository already holds %s %s",
					chart.Name, chart.Version),
				Hint: "priming will not replace it, so the older chart is what deploys, with " +
					"its old values and secrets. Bump the version, or delete the published " +
					"copy first",
			})
		}
	}
	return findings
}
