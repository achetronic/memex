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
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps a pgxpool.Pool and exposes all database operations.
// Use NewStore to create an instance.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a connection pool using the given DSN and runs
// all pending migrations. Returns an error if the connection or
// migrations fail.
func NewStore(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	s := &Store{pool: pool}
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
// Safe to run on every startup.
func (s *Store) migrate(ctx context.Context) error {
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

		`CREATE TABLE IF NOT EXISTS chunks (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			document_id   UUID        NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			namespace     TEXT        NOT NULL DEFAULT '',
			chunk_index   INTEGER     NOT NULL,
			content       TEXT        NOT NULL,
			embedding     vector,
			metadata      JSONB       NOT NULL DEFAULT '{}',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`ALTER TABLE chunks ADD COLUMN IF NOT EXISTS namespace TEXT NOT NULL DEFAULT ''`,

		// Partial index: only index chunks that have an embedding.
		`CREATE INDEX IF NOT EXISTS chunks_embedding_idx
			ON chunks USING ivfflat (embedding vector_cosine_ops)
			WHERE embedding IS NOT NULL`,

		// Index for fast namespace-filtered queries on documents.
		`CREATE INDEX IF NOT EXISTS documents_namespace_idx ON documents (namespace)`,

		// Index for fast namespace-filtered chunk searches.
		`CREATE INDEX IF NOT EXISTS chunks_namespace_idx ON chunks (namespace)`,
	}

	for _, stmt := range statements {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("executing migration statement: %w\nSQL: %s", err, stmt)
		}
	}

	return nil
}
