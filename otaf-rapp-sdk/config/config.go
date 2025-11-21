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
