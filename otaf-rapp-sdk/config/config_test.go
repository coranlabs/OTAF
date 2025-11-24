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
