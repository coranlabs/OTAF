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

// ApplyEnv walks dst and overwrites every field carrying an `env:"NAME"` tag
// whose variable is set and non-empty.
func ApplyEnv(dst any) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return errs.New(errs.CategoryInternal, "CONFIG_BAD_DESTINATION",
			"config: destination must be a non-nil pointer")
	}
	return walk(v.Elem())
}

func walk(v reflect.Value) error {
	if v.Kind() != reflect.Struct {
		return nil
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field, value := t.Field(i), v.Field(i)
		if !value.CanSet() {
			continue
		}

		switch value.Kind() {
		case reflect.Struct:
			if value.Type() != reflect.TypeOf(time.Duration(0)) {
				if err := walk(value); err != nil {
					return err
				}
				continue
			}
		case reflect.Pointer:
			if !value.IsNil() && value.Elem().Kind() == reflect.Struct {
				if err := walk(value.Elem()); err != nil {
					return err
				}
			}
			continue
		}

		name := field.Tag.Get("env")
		if name == "" || name == "-" {
			continue
		}
		raw, ok := os.LookupEnv(name)
		if !ok || raw == "" {
			continue
		}
		if err := set(value, raw); err != nil {
			return errs.Wrapf(err, errs.CategoryConfig, "CONFIG_BAD_VALUE",
				"%s=%q is not usable", name, raw).
				WithField("variable", name).WithField("value", raw)
		}
	}
	return nil
}

func set(v reflect.Value, raw string) error {
	if v.Type() == reflect.TypeOf(time.Duration(0)) {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return err
		}
		v.SetInt(int64(d))
		return nil
	}

	switch v.Kind() {
	case reflect.String:
		v.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return err
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return err
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return err
		}
		v.SetFloat(f)
	case reflect.Slice:
		if v.Type().Elem().Kind() != reflect.String {
			return fmt.Errorf("unsupported slice element %s", v.Type().Elem())
		}
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		v.Set(reflect.ValueOf(out))
	default:
		return fmt.Errorf("unsupported kind %s", v.Kind())
	}
	return nil
}

func findRapp(v reflect.Value) *Rapp {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	if v.Type() == reflect.TypeOf(Rapp{}) && v.CanAddr() {
		return v.Addr().Interface().(*Rapp)
	}
	for i := 0; i < v.NumField(); i++ {
		if r := findRapp(v.Field(i)); r != nil {
			return r
		}
	}
	return nil
}
