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
	"flag"
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
	// 1. Parse flags.
	configPath := flag.String("config", "", "path to YAML config file (default: config.yaml in working directory)")
	flag.Parse()

	// 2. Load configuration from YAML file.
	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// 3. Initialise logger.
	log := logger.New(cfg.Log.Format, cfg.Log.Level)
	log.Info("memex starting",
		"port", cfg.Server.Port,
		"log_format", cfg.Log.Format,
		"embeddings_base_url", cfg.Embeddings.BaseURL,
		"embeddings_model", cfg.Embeddings.Model,
		"auth_enabled", cfg.IsAuthEnabled(),
	)

	// 4. Connect to PostgreSQL and run migrations.
	ctx := context.Background()
	store, err := db.NewStore(ctx, cfg.Database.URL, cfg.Embeddings.Dimensions)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer store.Close()
	log.Info("database connected and migrations applied")

	// 5. Initialise the OpenAI-compatible embedder.
	emb := embedder.New(cfg.Embeddings.BaseURL, cfg.Embeddings.APIKey, cfg.Embeddings.Model)
	if err := emb.Ping(ctx); err != nil {
		return fmt.Errorf("embeddings API not reachable: %w", err)
	}
	log.Info("embeddings API reachable",
		"base_url", cfg.Embeddings.BaseURL,
		"model", cfg.Embeddings.Model,
	)

	// 6. Initialise chunker.
	chk := chunker.New(cfg.Chunker.Size, cfg.Chunker.Overlap)

	// 7. Start ingestion worker pool.
	queueSize := cfg.Worker.QueueSize
	if queueSize <= 0 {
		queueSize = cfg.Worker.PoolSize * 10
	}
	worker := ingestion.NewWorker(store, emb, chk, log, cfg.Worker.PoolSize, cfg.Worker.MaxRetries, queueSize)
	log.Info("ingestion worker pool started", "pool_size", cfg.Worker.PoolSize, "queue_size", queueSize)

	// 8. Build HTTP router.
	routerCfg := api.RouterConfig{
		Store:        store,
		Embedder:     emb,
		Worker:       worker,
		Log:          log,
		Config:       cfg,
		MaxUploadMB:  cfg.Upload.MaxSizeMB,
		DefaultLimit: cfg.Search.DefaultLimit,
		FrontendFS:   frontend,
	}
	router := api.NewRouter(routerCfg)

	// 9. Start HTTP server.
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("HTTP server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// 10. Wait for shutdown signal or server error.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("HTTP server error: %w", err)
	case sig := <-quit:
		log.Info("shutdown signal received", "signal", sig)
	}

	// 11. Graceful shutdown: give in-flight requests up to 30s to complete.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	log.Info("memex stopped cleanly")
	return nil
}
