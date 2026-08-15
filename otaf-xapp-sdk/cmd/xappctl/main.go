// Command xappctl scaffolds an xApp.
package main

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/coranlabs/OTAF/otaf-xapp-sdk/appmgr"
)

//go:embed all:templates
var templates embed.FS

type scaffold struct {
	Name         string
	Module       string
	Version      string
	HTTPPort     int
	RMRDataPort  int
	RMRRoutePort int
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "new":
		if err := newXapp(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "xappctl:", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: xappctl new <name> [flags]

  -out     directory to write to (default: the name)
  -module  Go module path (default: the name)
`)
}

func newXapp(args []string) error {
	name, rest := splitPositional(args)
	if name == "" {
		return fmt.Errorf("a name is required")
	}

	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	out := fs.String("out", "", "directory to write to")
	module := fs.String("module", "", "Go module path")
	if err := fs.Parse(rest); err != nil {
		return err
	}

	s := scaffold{
		Name:         name,
		Module:       *module,
		Version:      "1.0.0",
		HTTPPort:     8080,
		RMRDataPort:  4560,
		RMRRoutePort: 4561,
	}
	if s.Module == "" {
		s.Module = name
	}
	dir := *out
	if dir == "" {
		dir = name
	}

	desc := appmgr.Descriptor{
		Name: s.Name, Version: s.Version,
		HTTPPort: s.HTTPPort, RMRDataPort: s.RMRDataPort, RMRRoutePort: s.RMRRoutePort,
	}
	if err := desc.Validate(); err != nil {
		return err
	}

	files := map[string]string{
		"go.mod.tmpl":      "go.mod",
		"main.go.tmpl":     filepath.Join("cmd", name, "main.go"),
		"Dockerfile.tmpl":  "Dockerfile",
		"deploy.yaml.tmpl": filepath.Join("deploy", name+".yaml"),
	}
	for tmpl, target := range files {
		if err := render(dir, tmpl, target, s); err != nil {
			return err
		}
	}

	fmt.Printf("%s\n\n", dir)
	fmt.Printf("  rmr  %s\n", desc.RMREndpoint())
	fmt.Printf("  http %s\n\n", desc.HTTPEndpoint())
	fmt.Print("The services in deploy/ carry those names. The routing manager\n" +
		"builds routes from them, so renaming one without the other loses\n" +
		"traffic with nothing reported.\n")
	return nil
}

func render(dir, tmpl, target string, s scaffold) error {
	body, err := templates.ReadFile("templates/" + tmpl)
	if err != nil {
		return err
	}
	t, err := template.New(tmpl).Parse(string(body))
	if err != nil {
		return err
	}
	path := filepath.Join(dir, target)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, s)
}

// splitPositional separates leading arguments from flags, because the flag
// package stops at the first argument that is not a flag.
func splitPositional(args []string) (string, []string) {
	for i, a := range args {
		if len(a) > 0 && a[0] == '-' {
			if i == 0 {
				return "", args
			}
			return args[0], args[i:]
		}
	}
	if len(args) == 0 {
		return "", nil
	}
	return args[0], nil
}
