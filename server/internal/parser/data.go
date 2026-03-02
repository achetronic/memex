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

package parser

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// JSONParser converts JSON into indented text. This preserves keys and values
// in a human-readable form that embeds well.
type JSONParser struct{}

func (p *JSONParser) Extensions() []string { return []string{".json"} }

// Parse reads JSON from r and returns a pretty-printed representation.
func (p *JSONParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading JSON file: %w", err)
	}

	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return "", fmt.Errorf("parsing JSON: %w", err)
	}

	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("re-marshalling JSON: %w", err)
	}
	return string(out), nil
}

// YAMLParser converts YAML into JSON text for consistent representation.
type YAMLParser struct{}

func (p *YAMLParser) Extensions() []string { return []string{".yaml", ".yml"} }

// Parse reads YAML from r, unmarshals it, and returns a JSON representation.
func (p *YAMLParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading YAML file: %w", err)
	}

	var v any
	if err := yaml.Unmarshal(b, &v); err != nil {
		return "", fmt.Errorf("parsing YAML: %w", err)
	}

	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("converting YAML to JSON: %w", err)
	}
	return string(out), nil
}

// TOMLParser converts TOML into JSON text for consistent representation.
type TOMLParser struct{}

func (p *TOMLParser) Extensions() []string { return []string{".toml"} }

// Parse reads TOML from r, decodes it, and returns a JSON representation.
func (p *TOMLParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading TOML file: %w", err)
	}

	var v any
	if _, err := toml.Decode(string(b), &v); err != nil {
		return "", fmt.Errorf("parsing TOML: %w", err)
	}

	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("converting TOML to JSON: %w", err)
	}
	return string(out), nil
}
