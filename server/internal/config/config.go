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

// Package config loads and validates all application configuration from a
// YAML file. The path is passed via -config (default: config.yaml in the
// working directory). All string values support ${ENV_VAR} expansion so
// secrets can be injected from the environment without writing them in plain
// text.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port int `yaml:"port"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Format string `yaml:"format"` // "console" or "json"
	Level  string `yaml:"level"`  // "debug", "info", "warn", "error"
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	URL string `yaml:"url"`
}

// EmbeddingsConfig holds settings for the OpenAI-compatible embeddings provider.
type EmbeddingsConfig struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
	Dim     int    `yaml:"dim"`
}

// WorkerConfig holds ingestion worker pool settings.
type WorkerConfig struct {
	PoolSize   int `yaml:"pool_size"`
	MaxRetries int `yaml:"max_retries"`
}

// ChunkerConfig holds text chunking settings.
type ChunkerConfig struct {
	Size    int `yaml:"size"`
	Overlap int `yaml:"overlap"`
}

// SearchConfig holds search defaults.
type SearchConfig struct {
	DefaultLimit int `yaml:"default_limit"`
}

// UploadConfig holds upload limits.
type UploadConfig struct {
	MaxSizeMB int64 `yaml:"max_size_mb"`
}

// NamespaceConfig declares a single namespace that Memex is aware of.
// Requests for undeclared namespaces are rejected with 400.
type NamespaceConfig struct {
	Name string `yaml:"name"`
}

// APIKeyConfig binds an API key to one or more namespaces.
// Use "*" in namespaces to grant access to all declared namespaces.
type APIKeyConfig struct {
	Key        string   `yaml:"key"`
	Namespaces []string `yaml:"namespaces"`
}

// AuthConfig holds API key authentication settings.
// When api_keys is empty, auth is disabled and all requests are allowed through.
type AuthConfig struct {
	APIKeys []APIKeyConfig `yaml:"api_keys"`
}

// Config is the root configuration structure loaded from the YAML file.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Log        LogConfig        `yaml:"log"`
	Database   DatabaseConfig   `yaml:"database"`
	Embeddings EmbeddingsConfig `yaml:"embeddings"`
	Worker     WorkerConfig     `yaml:"worker"`
	Chunker    ChunkerConfig    `yaml:"chunker"`
	Search     SearchConfig     `yaml:"search"`
	Upload     UploadConfig     `yaml:"upload"`
	Namespaces []NamespaceConfig `yaml:"namespaces"`
	Auth       AuthConfig       `yaml:"auth"`
}

// defaults returns a Config pre-populated with sensible default values.
func defaults() Config {
	return Config{
		Server: ServerConfig{
			Port: 8080,
		},
		Log: LogConfig{
			Format: "console",
			Level:  "info",
		},
		Embeddings: EmbeddingsConfig{
			BaseURL: "http://localhost:11434",
			APIKey:  "ollama",
			Model:   "nomic-embed-text",
			Dim:     768,
		},
		Worker: WorkerConfig{
			PoolSize:   3,
			MaxRetries: 3,
		},
		Chunker: ChunkerConfig{
			Size:    512,
			Overlap: 64,
		},
		Search: SearchConfig{
			DefaultLimit: 5,
		},
		Upload: UploadConfig{
			MaxSizeMB: 50,
		},
	}
}

// Load reads and validates the config file at the given path.
// If path is empty, it looks for "config.yaml" in the working directory.
// Returns an error if the file is missing, malformed, or fails validation.
func Load(path string) (*Config, error) {
	if path == "" {
		path = "config.yaml"
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	expanded := os.ExpandEnv(string(raw))

	cfg := defaults()
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// validate checks that all required fields are present and values are in range.
func (c *Config) validate() error {
	if c.Database.URL == "" {
		return fmt.Errorf("database.url is required")
	}
	if c.Log.Format != "console" && c.Log.Format != "json" {
		return fmt.Errorf("log.format must be 'console' or 'json', got %q", c.Log.Format)
	}
	if c.Chunker.Overlap >= c.Chunker.Size {
		return fmt.Errorf("chunker.overlap (%d) must be less than chunker.size (%d)", c.Chunker.Overlap, c.Chunker.Size)
	}
	return nil
}

// IsAuthEnabled reports whether API key authentication is active.
func (c *Config) IsAuthEnabled() bool {
	return len(c.Auth.APIKeys) > 0
}

// IsNamespaceDeclared reports whether the given namespace is declared in the
// config. Always returns true when no namespaces are declared (open mode).
func (c *Config) IsNamespaceDeclared(ns string) bool {
	if len(c.Namespaces) == 0 {
		return true
	}
	for _, n := range c.Namespaces {
		if n.Name == ns {
			return true
		}
	}
	return false
}

// KeyHasNamespaceAccess reports whether the given API key exists and has
// access to the given namespace.
func (c *Config) KeyHasNamespaceAccess(key, namespace string) bool {
	for _, k := range c.Auth.APIKeys {
		if k.Key != key {
			continue
		}
		for _, ns := range k.Namespaces {
			if ns == "*" || ns == namespace {
				return true
			}
		}
	}
	return false
}
