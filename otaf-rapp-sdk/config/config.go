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

// Package config loads rApp configuration from a YAML file mounted by the
// deployment chart, then lets the environment override individual fields.
package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/coranlabs/OTAF/otaf-rapp-sdk/errs"
)

type Rapp struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`

	HTTPPort  string `yaml:"http_port" env:"HTTP_PORT"`
	LogLevel  string `yaml:"log_level" env:"LOG_LEVEL"`
	LogFormat string `yaml:"log_format" env:"LOG_FORMAT"`
}

func (r *Rapp) applyDefaults() {
	if r.Name == "" {
		r.Name = "rapp"
	}
	if r.Version == "" {
		r.Version = "0.1.0"
	}
	if r.HTTPPort == "" {
		r.HTTPPort = "8080"
	}
	if r.LogLevel == "" {
		r.LogLevel = "info"
	}
	if r.LogFormat == "" {
		r.LogFormat = "text"
	}
}

// SearchPaths are tried in order when no explicit path is given. CONFIG_PATH
// wins over all of them; the chart normally sets it.
var SearchPaths = []string{
	"config/rapp.yaml",
	"/app/config/rapp.yaml",
	"/etc/rapp/rapp.yaml",
}

// Load fills dst from the first readable YAML file, then applies env overrides.
// A missing file is not fatal as long as the environment supplies what the
// rApp needs; an unreadable or malformed file always is.
func Load(dst any, paths ...string) error {
	if len(paths) == 0 {
		if p := os.Getenv("CONFIG_PATH"); p != "" {
			paths = []string{p}
		} else {
			paths = SearchPaths
		}
	}

	var loaded string
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := yaml.Unmarshal(data, dst); err != nil {
			return errs.Wrapf(err, errs.CategoryConfig, "CONFIG_UNPARSEABLE",
				"config file %s is not valid YAML", p).WithField("path", p)
		}
		loaded = p
		break
	}
	if loaded == "" && len(paths) == 1 {
		return errs.Newf(errs.CategoryConfig, "CONFIG_MISSING",
			"config file %s is not readable", paths[0]).WithField("path", paths[0])
	}

	if err := ApplyEnv(dst); err != nil {
		return err
	}
	if r := findRapp(reflect.ValueOf(dst)); r != nil {
		r.applyDefaults()
	}
	return nil
}

// Path reports which file Load would read, or "" when none is readable.
func Path(paths ...string) string {
	if len(paths) == 0 {
		if p := os.Getenv("CONFIG_PATH"); p != "" {
			paths = []string{p}
		} else {
			paths = SearchPaths
		}
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
