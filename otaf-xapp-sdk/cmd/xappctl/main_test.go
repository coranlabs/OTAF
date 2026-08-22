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
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coranlabs/OTAF/otaf-xapp-sdk/appmgr"
)

func scaffoldInto(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := newXapp([]string{"demo-xapp", "-out", dir}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestScaffoldWritesTheExpectedFiles(t *testing.T) {
	dir := scaffoldInto(t)
	for _, f := range []string{"go.mod", "Dockerfile", "cmd/demo-xapp/main.go", "deploy/demo-xapp.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

// The routing manager builds routes from names derived from the xApp name. If
// the manifest and the SDK ever disagree on those names, traffic is dropped
// with nothing reported, so they are compared here.
func TestManifestServicesMatchTheDerivedEndpoints(t *testing.T) {
	manifest := read(t, scaffoldInto(t), "deploy/demo-xapp.yaml")
	desc := appmgr.Descriptor{
		Name: "demo-xapp", Version: "1.0.0",
		HTTPPort: 8080, RMRDataPort: 4560, RMRRoutePort: 4561,
	}
	for _, endpoint := range []string{desc.RMREndpoint(), desc.HTTPEndpoint()} {
		service := strings.TrimSuffix(strings.SplitN(endpoint, ":", 2)[0], ".ricxapp")
		if !strings.Contains(manifest, "name: "+service) {
			t.Errorf("manifest has no service %q", service)
		}
	}
	if !strings.Contains(manifest, "value: service-ricxapp-demo-xapp-rmr.ricxapp") {
		t.Error("RMR_SRC_ID does not carry the name the routing manager uses")
	}
}

// The platform's modules declare paths their repositories do not serve. Without
// these the scaffold does not resolve, and the error names a transitive
// dependency rather than the cause.
func TestGoModCarriesThePlatformReplacements(t *testing.T) {
	gomod := read(t, scaffoldInto(t), "go.mod")
	for _, want := range []string{
		"replace gerrit.o-ran-sc.org/r/ric-plt/xapp-frame => gerrit.o-ran-sc.org/r/ric-plt/xapp-frame.git",
		"replace gerrit.o-ran-sc.org/r/ric-plt/sdlgo => gerrit.o-ran-sc.org/r/ric-plt/sdlgo.git",
		"replace gerrit.o-ran-sc.org/r/com/golog => gerrit.o-ran-sc.org/r/com/golog.git",
		"gerrit.o-ran-sc.org/r/ric-plt/nodeb-rnib.git/reader",
	} {
		if !strings.Contains(gomod, want) {
			t.Errorf("go.mod is missing %q", want)
		}
	}
}

func TestScaffoldedSourceParses(t *testing.T) {
	dir := scaffoldInto(t)
	path := filepath.Join(dir, "cmd/demo-xapp/main.go")
	if _, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.AllErrors); err != nil {
		t.Errorf("generated source does not parse: %v", err)
	}
}

func TestScaffoldRejectsABadName(t *testing.T) {
	if err := newXapp([]string{"Demo_xApp", "-out", t.TempDir()}); err == nil {
		t.Error("want error for a name the platform would reject")
	}
}

func TestSplitPositional(t *testing.T) {
	cases := []struct {
		args []string
		name string
		rest int
	}{
		{[]string{"demo", "-out", "/tmp"}, "demo", 2},
		{[]string{"demo"}, "demo", 0},
		{[]string{"-out", "/tmp"}, "", 2},
		{nil, "", 0},
	}
	for _, c := range cases {
		name, rest := splitPositional(c.args)
		if name != c.name || len(rest) != c.rest {
			t.Errorf("splitPositional(%v) = %q, %v", c.args, name, rest)
		}
	}
}

// The subscription manager posts its response to the same name the routing
// manager routes to, so that service has to carry the http port as well. It is
// otherwise unreachable and the subscription never completes.
func TestRMRServiceAlsoServesHTTP(t *testing.T) {
	manifest := read(t, scaffoldInto(t), "deploy/demo-xapp.yaml")
	rmr := manifest[strings.Index(manifest, "name: service-ricxapp-demo-xapp-rmr"):]
	if end := strings.Index(rmr, "\n---"); end > 0 {
		rmr = rmr[:end]
	}
	if !strings.Contains(rmr, "port: 8080") {
		t.Error("the rmr service does not serve the http port")
	}
}

// The SDK is not on the module proxy until it is released, so a scaffold has
// to be able to resolve it from a checkout.
func TestSDKPathAddsAReplacement(t *testing.T) {
	dir := t.TempDir()
	if err := newXapp([]string{"demo-xapp", "-out", dir, "-sdk-path", "/somewhere/otaf-xapp-sdk"}); err != nil {
		t.Fatal(err)
	}
	gomod := read(t, dir, "go.mod")
	if !strings.Contains(gomod, "replace github.com/coranlabs/OTAF/otaf-xapp-sdk => /somewhere/otaf-xapp-sdk") {
		t.Error("go.mod does not resolve the SDK from the given path")
	}
}

func TestWithoutSDKPathThereIsNoReplacement(t *testing.T) {
	gomod := read(t, scaffoldInto(t), "go.mod")
	if strings.Contains(gomod, "replace github.com/coranlabs/OTAF/otaf-xapp-sdk") {
		t.Error("go.mod replaces the SDK when it was not asked to")
	}
}
