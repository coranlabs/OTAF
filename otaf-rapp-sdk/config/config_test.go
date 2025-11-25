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

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type nested struct {
	Endpoint string        `yaml:"endpoint" env:"TEST_ENDPOINT"`
	Timeout  time.Duration `yaml:"timeout" env:"TEST_TIMEOUT"`
	Retries  int           `yaml:"retries" env:"TEST_RETRIES"`
	Enabled  bool          `yaml:"enabled" env:"TEST_ENABLED"`
	Brokers  []string      `yaml:"brokers" env:"TEST_BROKERS"`
}

type sample struct {
	Rapp  Rapp   `yaml:"rapp"`
	Inner nested `yaml:"inner"`
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rapp.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAppliesDefaults(t *testing.T) {
	path := writeConfig(t, "rapp:\n  name: probe\n")

	var cfg sample
	if err := Load(&cfg, path); err != nil {
		t.Fatal(err)
	}

	if cfg.Rapp.Name != "probe" {
		t.Errorf("name = %q, want probe", cfg.Rapp.Name)
	}
	if cfg.Rapp.HTTPPort != "8080" {
		t.Errorf("http port = %q, want 8080", cfg.Rapp.HTTPPort)
	}
	if cfg.Rapp.LogLevel != "info" {
		t.Errorf("log level = %q, want info", cfg.Rapp.LogLevel)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	path := writeConfig(t, `
rapp:
  name: probe
  http_port: "8080"
inner:
  endpoint: http://from-file
  retries: 1
`)

	t.Setenv("TEST_ENDPOINT", "http://from-env")
	t.Setenv("TEST_TIMEOUT", "45s")
	t.Setenv("TEST_RETRIES", "7")
	t.Setenv("TEST_ENABLED", "true")
	t.Setenv("TEST_BROKERS", "a:9092, b:9092 ,")
	t.Setenv("HTTP_PORT", "9999")

	var cfg sample
	if err := Load(&cfg, path); err != nil {
		t.Fatal(err)
	}

	if cfg.Inner.Endpoint != "http://from-env" {
		t.Errorf("endpoint = %q, want the env value", cfg.Inner.Endpoint)
	}
	if cfg.Inner.Timeout != 45*time.Second {
		t.Errorf("timeout = %v, want 45s", cfg.Inner.Timeout)
	}
	if cfg.Inner.Retries != 7 {
		t.Errorf("retries = %d, want 7", cfg.Inner.Retries)
	}
	if !cfg.Inner.Enabled {
		t.Error("enabled = false, want true")
	}
	if len(cfg.Inner.Brokers) != 2 || cfg.Inner.Brokers[0] != "a:9092" || cfg.Inner.Brokers[1] != "b:9092" {
		t.Errorf("brokers = %#v, want two trimmed entries", cfg.Inner.Brokers)
	}
	if cfg.Rapp.HTTPPort != "9999" {
		t.Errorf("http port = %q, want the env value", cfg.Rapp.HTTPPort)
	}
}

func TestEmptyEnvDoesNotOverride(t *testing.T) {
	path := writeConfig(t, "rapp:\n  name: probe\ninner:\n  endpoint: http://from-file\n")
	t.Setenv("TEST_ENDPOINT", "")

	var cfg sample
	if err := Load(&cfg, path); err != nil {
		t.Fatal(err)
	}
	if cfg.Inner.Endpoint != "http://from-file" {
		t.Errorf("endpoint = %q, want the file value kept", cfg.Inner.Endpoint)
	}
}

func TestUnparseableEnvIsAnError(t *testing.T) {
	path := writeConfig(t, "rapp:\n  name: probe\n")
	t.Setenv("TEST_TIMEOUT", "not-a-duration")

	var cfg sample
	if err := Load(&cfg, path); err == nil {
		t.Fatal("expected an error for an unparseable duration")
	}
}

func TestExplicitPathMustExist(t *testing.T) {
	var cfg sample
	if err := Load(&cfg, filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("expected an error when the named config file is missing")
	}
}
