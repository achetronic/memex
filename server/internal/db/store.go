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

// Package db manages all interactions with PostgreSQL + pgvector.
// It handles connection pooling, schema migrations on startup,
// and provides typed methods for documents and chunks.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps a pgxpool.Pool and exposes all database operations.
// Use NewStore to create an instance.
type Store struct {
	pool       *pgxpool.Pool
	dimensions int
}

// NewStore creates a connection pool using the given DSN and runs
// all pending migrations. dimensions is the vector size produced by the
// configured embedding model (e.g. 768 for nomic-embed-text).
// Returns an error if the connection or migrations fail.
func NewStore(ctx context.Context, dsn string, dimensions int) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	s := &Store{pool: pool, dimensions: dimensions}
	if err := s.migrate(ctx); err != nil {
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return s, nil
}

// Close releases all connections in the pool. Should be deferred after NewStore.
func (s *Store) Close() {
	s.pool.Close()
}

// migrate applies the schema idempotently. It enables pgvector, creates the
// documents and chunks tables if they don't exist, and adds the vector index.
// Safe to run on every startup — fails fast if the configured dimensions do
// not match the existing column to prevent silent data corruption.
func (s *Store) migrate(ctx context.Context) error {
	// Guard: if the chunks table already exists and the embedding column has
	// explicit dimensions, verify they match the configured value. A mismatch
	// means the user changed embeddings.dimensions without re-ingesting data,
	// which would silently corrupt search results.
	//
	// We read the column type as text (e.g. "vector(768)") and parse N directly
	// to avoid ambiguity with atttypmod encoding conventions.
	var colType string
	err := s.pool.QueryRow(ctx, `
		SELECT pg_catalog.format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relname = 'chunks'
		  AND n.nspname = 'public'
		  AND a.attname = 'embedding'
		  AND a.attnum > 0
		  AND NOT a.attisdropped
	`).Scan(&colType)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Table does not exist yet — nothing to validate.
	case err != nil:
		return fmt.Errorf("checking existing embedding dimensions: %w", err)
	default:
		// colType is e.g. "vector(768)" or "vector" (no dimensions).
		// Only validate when dimensions are explicitly set.
		var current int
		if n, _ := fmt.Sscanf(colType, "vector(%d)", &current); n == 1 && current != s.dimensions {
			return fmt.Errorf(
				"embedding dimension mismatch: database has vector(%d) but config says %d — "+
					"change embeddings.dimensions back to %d or drop the chunks table and re-ingest",
				current, s.dimensions, current,
			)
		}
	}

	statements := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,

		`CREATE TABLE IF NOT EXISTS documents (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			namespace    TEXT        NOT NULL DEFAULT '',
			filename     TEXT        NOT NULL,
			format       TEXT        NOT NULL,
			status       TEXT        NOT NULL DEFAULT 'pending',
			error        TEXT,
			chunk_count  INTEGER     NOT NULL DEFAULT 0,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		// Idempotent migration: add namespace column to pre-existing tables.
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS namespace TEXT NOT NULL DEFAULT ''`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS chunks (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			document_id   UUID        NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			namespace     TEXT        NOT NULL DEFAULT '',
			chunk_index   INTEGER     NOT NULL,
			content       TEXT        NOT NULL,
			embedding     vector(%d),
			metadata      JSONB       NOT NULL DEFAULT '{}',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`, s.dimensions),

		`ALTER TABLE chunks ADD COLUMN IF NOT EXISTS namespace TEXT NOT NULL DEFAULT ''`,

		// Idempotent migration: if the embedding column exists without explicit
		// dimensions (created before v0.2.0), set them now. Safe because the
		// dimension guard above already confirmed there is no mismatch.
		fmt.Sprintf(`ALTER TABLE chunks ALTER COLUMN embedding TYPE vector(%d)`, s.dimensions),

		// Partial index on chunks with a fixed-dimension embedding column.
		// ivfflat requires the column type to have explicit dimensions (vector(N)).
		`CREATE INDEX IF NOT EXISTS chunks_embedding_idx
			ON chunks USING ivfflat (embedding vector_cosine_ops)
			WHERE embedding IS NOT NULL`,

		// Index for fast namespace-filtered queries on documents.
		`CREATE INDEX IF NOT EXISTS documents_namespace_idx ON documents (namespace)`,

		// Index for fast namespace-filtered chunk searches.
		`CREATE INDEX IF NOT EXISTS chunks_namespace_idx ON chunks (namespace)`,

		// Idempotent migration: add file_hash column for deduplication.
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS file_hash TEXT`,

		// Index for fast dedup lookups by hash within a namespace.
		`CREATE INDEX IF NOT EXISTS documents_namespace_hash_idx ON documents (namespace, file_hash) WHERE file_hash IS NOT NULL`,

		// Idempotent migration: add file_path column for on-disk file persistence.
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS file_path TEXT`,

		// Idempotent migration: add instance_id to track which instance owns the file.
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS instance_id TEXT`,
	}

	for _, stmt := range statements {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("executing migration statement: %w\nSQL: %s", err, stmt)
		}
	}

	return nil
}
