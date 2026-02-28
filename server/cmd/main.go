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

// memex — self-hosted RAG backend powered by PostgreSQL + pgvector and Ollama.
//
//	@title			memex API
//	@version		1.0
//	@description	Generic RAG (Retrieval-Augmented Generation) backend. Upload documents, index them via Ollama embeddings stored in pgvector, and query them semantically via REST API.
//	@contact.name	Alby Hernández
//	@contact.url	https://github.com/achetronic/memex
//	@license.name	MIT
//	@BasePath		/api/v1
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/achetronic/memex/internal/api"
	"github.com/achetronic/memex/internal/chunker"
	"github.com/achetronic/memex/internal/config"
	"github.com/achetronic/memex/internal/db"
	"github.com/achetronic/memex/internal/embedder"
	"github.com/achetronic/memex/internal/ingestion"
	"github.com/achetronic/memex/internal/logger"
)

// frontend holds the embedded Vue dist/ directory.
// It is populated at build time via go:embed in the embed.go file.
var frontend fs.FS

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

// run is the real entry point. It initialises all dependencies, starts the
// HTTP server with graceful shutdown, and blocks until a signal is received.
func run() error {
	// 1. Load configuration from environment.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// 2. Initialise logger.
	log := logger.New(cfg.LogFormat, cfg.LogLevel)
	log.Info("memex starting",
		"port", cfg.Port,
		"log_format", cfg.LogFormat,
		"openai_base_url", cfg.OpenAIBaseURL,
		"openai_embedding_model", cfg.OpenAIEmbeddingModel,
	)

	// 3. Connect to PostgreSQL and run migrations.
	ctx := context.Background()
	store, err := db.NewStore(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer store.Close()
	log.Info("database connected and migrations applied")

	// 4. Initialise the OpenAI-compatible embedder.
	emb := embedder.New(cfg.OpenAIBaseURL, cfg.OpenAIAPIKey, cfg.OpenAIEmbeddingModel)
	if err := emb.Ping(ctx); err != nil {
		return fmt.Errorf("embeddings API not reachable: %w", err)
	}
	log.Info("embeddings API reachable",
		"base_url", cfg.OpenAIBaseURL,
		"model", cfg.OpenAIEmbeddingModel,
	)

	// 5. Initialise chunker.
	chk := chunker.New(cfg.ChunkSize, cfg.ChunkOverlap)

	// 6. Start ingestion worker pool.
	// Queue size is 10× the pool size to allow burst uploads.
	queueSize := cfg.WorkerPoolSize * 10
	worker := ingestion.NewWorker(store, emb, chk, log, cfg.WorkerPoolSize, cfg.WorkerMaxRetries, queueSize)
	log.Info("ingestion worker pool started", "pool_size", cfg.WorkerPoolSize)

	// 7. Build HTTP router.
	routerCfg := api.RouterConfig{
		Store:        store,
		Embedder:     emb,
		Worker:       worker,
		Log:          log,
		MaxUploadMB:  cfg.MaxUploadSizeMB,
		DefaultLimit: cfg.SearchDefaultLimit,
		FrontendFS:   frontend,
	}
	router := api.NewRouter(routerCfg)

	// 8. Start HTTP server.
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	// Start server in background.
	serverErr := make(chan error, 1)
	go func() {
		log.Info("HTTP server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// 9. Wait for shutdown signal or server error.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("HTTP server error: %w", err)
	case sig := <-quit:
		log.Info("shutdown signal received", "signal", sig)
	}

	// 10. Graceful shutdown: give in-flight requests up to 30s to complete.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	log.Info("memex stopped cleanly")
	return nil
}
