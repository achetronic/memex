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

// Package config loads and validates all application configuration.
// Runtime tunables (ports, pool sizes, etc.) come from environment variables.
// Namespaces and API key auth come from an optional YAML file passed via -config.
// Environment variables in the YAML are expanded with os.ExpandEnv at load time.
package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// NamespaceConfig declares a single namespace that Memex is aware of.
// Requests for undeclared namespaces are rejected with 400.
type NamespaceConfig struct {
	Name string `yaml:"name"`
}

// APIKeyConfig binds an API key to one or more namespaces.
// Use "*" in Namespaces to grant access to all declared namespaces.
type APIKeyConfig struct {
	// Key is the raw API key value. Use ${ENV_VAR} for secret injection.
	Key        string   `yaml:"key"`
	Namespaces []string `yaml:"namespaces"`
}

// AuthConfig holds the API key authentication configuration.
// When the section is absent or api_keys is empty, auth is disabled and all
// requests are allowed through (useful for local / no-auth deployments).
type AuthConfig struct {
	APIKeys []APIKeyConfig `yaml:"api_keys"`
}

// FileConfig is the structure of the optional YAML config file.
// All string values support ${ENV_VAR} expansion.
type FileConfig struct {
	Namespaces []NamespaceConfig `yaml:"namespaces"`
	Auth       AuthConfig        `yaml:"auth"`
}

// Config holds all runtime configuration for memex.
type Config struct {
	// HTTP server
	Port int

	// Logging
	LogFormat string // "console" or "json"
	LogLevel  string // "debug", "info", "warn", "error"

	// Database
	DatabaseURL string

	// OpenAI-compatible embeddings provider (works with Ollama, OpenAI, Groq, etc.)
	OpenAIBaseURL        string
	OpenAIAPIKey         string
	OpenAIEmbeddingModel string
	OpenAIEmbeddingDim   int

	// Worker
	WorkerPoolSize   int
	WorkerMaxRetries int

	// Chunker
	ChunkSize    int
	ChunkOverlap int

	// Search
	SearchDefaultLimit int

	// Upload
	MaxUploadSizeMB int64

	// Namespaces and auth — loaded from optional YAML file.
	// Empty means auth is disabled and no namespace validation is performed.
	File FileConfig
}

// Load reads all configuration. Runtime values come from environment variables.
// If configPath is non-empty, the YAML file is loaded for namespace and auth config.
// Returns an error if any required variable is missing or any value is invalid.
func Load(configPath string) (*Config, error) {
	cfg := &Config{
		Port:                 envInt("PORT", 8080),
		LogFormat:            envStr("LOG_FORMAT", "console"),
		LogLevel:             envStr("LOG_LEVEL", "info"),
		DatabaseURL:          envStr("DATABASE_URL", ""),
		OpenAIBaseURL:        envStr("OPENAI_BASE_URL", "http://localhost:11434"),
		OpenAIAPIKey:         envStr("OPENAI_API_KEY", "ollama"),
		OpenAIEmbeddingModel: envStr("OPENAI_EMBEDDING_MODEL", "nomic-embed-text"),
		OpenAIEmbeddingDim:   envInt("OPENAI_EMBEDDING_DIM", 768),
		WorkerPoolSize:       envInt("WORKER_POOL_SIZE", 3),
		WorkerMaxRetries:     envInt("WORKER_MAX_RETRIES", 3),
		ChunkSize:            envInt("CHUNK_SIZE", 512),
		ChunkOverlap:         envInt("CHUNK_OVERLAP", 64),
		SearchDefaultLimit:   envInt("SEARCH_DEFAULT_LIMIT", 5),
		MaxUploadSizeMB:      int64(envInt("MAX_UPLOAD_SIZE_MB", 50)),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	if cfg.LogFormat != "console" && cfg.LogFormat != "json" {
		return nil, fmt.Errorf("LOG_FORMAT must be 'console' or 'json', got %q", cfg.LogFormat)
	}

	if cfg.ChunkOverlap >= cfg.ChunkSize {
		return nil, fmt.Errorf("CHUNK_OVERLAP (%d) must be less than CHUNK_SIZE (%d)", cfg.ChunkOverlap, cfg.ChunkSize)
	}

	if configPath != "" {
		fc, err := loadFileConfig(configPath)
		if err != nil {
			return nil, fmt.Errorf("loading config file %q: %w", configPath, err)
		}
		cfg.File = *fc
	}

	return cfg, nil
}

// loadFileConfig reads and parses the YAML config file, expanding environment
// variables in all string values before unmarshalling.
func loadFileConfig(path string) (*FileConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	expanded := os.ExpandEnv(string(raw))

	var fc FileConfig
	if err := yaml.Unmarshal([]byte(expanded), &fc); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	return &fc, nil
}

// IsAuthEnabled reports whether API key authentication is active.
// Auth is considered enabled when at least one API key is configured.
func (c *Config) IsAuthEnabled() bool {
	return len(c.File.Auth.APIKeys) > 0
}

// IsNamespaceDeclared reports whether the given namespace name is declared in
// the config. Always returns true when no namespaces are declared (open mode).
func (c *Config) IsNamespaceDeclared(ns string) bool {
	if len(c.File.Namespaces) == 0 {
		return true
	}
	for _, n := range c.File.Namespaces {
		if n.Name == ns {
			return true
		}
	}
	return false
}

// KeyHasNamespaceAccess reports whether the given API key exists and has access
// to the given namespace. Returns false if the key is not found.
func (c *Config) KeyHasNamespaceAccess(key, namespace string) bool {
	for _, k := range c.File.Auth.APIKeys {
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

// envStr returns the value of the environment variable named by key,
// or fallback if the variable is not set or is empty.
func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envInt returns the integer value of the environment variable named by key,
// or fallback if the variable is not set, empty, or not a valid integer.
func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
