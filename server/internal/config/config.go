// Package config loads and validates all application configuration from
// environment variables. It is read once at startup and passed as a dependency
// throughout the application — never read from the environment mid-execution.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all runtime configuration for memex.
// Every field maps to an environment variable documented in AGENTS.md.
type Config struct {
	// HTTP server
	Port int

	// Logging
	LogFormat string // "console" or "json"
	LogLevel  string // "debug", "info", "warn", "error"

	// Database
	DatabaseURL string

	// OpenAI-compatible embeddings provider (works with Ollama, OpenAI, Groq, etc.)
	OpenAIBaseURL      string
	OpenAIAPIKey       string
	OpenAIEmbeddingModel string
	OpenAIEmbeddingDim int

	// Worker
	WorkerPoolSize  int
	WorkerMaxRetries int

	// Chunker
	ChunkSize    int
	ChunkOverlap int

	// Search
	SearchDefaultLimit int

	// Upload
	MaxUploadSizeMB int64
}

// Load reads all configuration from environment variables and returns a
// validated Config. Returns an error if any required variable is missing
// or if any value is out of acceptable range.
func Load() (*Config, error) {
	cfg := &Config{
		Port:               envInt("PORT", 8080),
		LogFormat:          envStr("LOG_FORMAT", "console"),
		LogLevel:           envStr("LOG_LEVEL", "info"),
		DatabaseURL:        envStr("DATABASE_URL", ""),
		OpenAIBaseURL:        envStr("OPENAI_BASE_URL", "http://localhost:11434"),
		OpenAIAPIKey:         envStr("OPENAI_API_KEY", "ollama"),
		OpenAIEmbeddingModel: envStr("OPENAI_EMBEDDING_MODEL", "nomic-embed-text"),
		OpenAIEmbeddingDim:   envInt("OPENAI_EMBEDDING_DIM", 768),
		WorkerPoolSize:     envInt("WORKER_POOL_SIZE", 3),
		WorkerMaxRetries:   envInt("WORKER_MAX_RETRIES", 3),
		ChunkSize:          envInt("CHUNK_SIZE", 512),
		ChunkOverlap:       envInt("CHUNK_OVERLAP", 64),
		SearchDefaultLimit: envInt("SEARCH_DEFAULT_LIMIT", 5),
		MaxUploadSizeMB:    int64(envInt("MAX_UPLOAD_SIZE_MB", 50)),
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

	return cfg, nil
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
