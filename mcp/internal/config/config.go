// Copyright 2025 Alby Hernández
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

	"memex-mcp/api"

	"gopkg.in/yaml.v3"
)

// Marshal serialises a Configuration into YAML bytes.
func Marshal(cfg api.Configuration) ([]byte, error) {
	return yaml.Marshal(cfg)
}

// Unmarshal parses YAML bytes into a Configuration.
func Unmarshal(data []byte) (api.Configuration, error) {
	var cfg api.Configuration
	err := yaml.Unmarshal(data, &cfg)
	return cfg, err
}

// ReadFile reads a YAML configuration file, expands any ${ENV_VAR} references
// found in its content, and returns the parsed Configuration.
func ReadFile(filepath string) (api.Configuration, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return api.Configuration{}, err
	}
	expanded := os.ExpandEnv(string(data))
	return Unmarshal([]byte(expanded))
}
