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
			filename     TEXT        NOT NULL,
			format       TEXT        NOT NULL,
			status       TEXT        NOT NULL DEFAULT 'pending',
			error        TEXT,
			chunk_count  INTEGER     NOT NULL DEFAULT 0,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE TABLE IF NOT EXISTS chunks (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			document_id   UUID        NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			chunk_index   INTEGER     NOT NULL,
			content       TEXT        NOT NULL,
			embedding     vector,
			metadata      JSONB       NOT NULL DEFAULT '{}',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		// Partial index: only index chunks that have an embedding.
		`CREATE INDEX IF NOT EXISTS chunks_embedding_idx
			ON chunks USING ivfflat (embedding vector_cosine_ops)
			WHERE embedding IS NOT NULL`,
	}

	for _, stmt := range statements {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("executing migration statement: %w\nSQL: %s", err, stmt)
		}
	}

	return nil
}
