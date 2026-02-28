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

package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

// Chunk represents a text segment extracted from a document, with its embedding.
type Chunk struct {
	ID         uuid.UUID      `json:"id"`
	DocumentID uuid.UUID      `json:"document_id"`
	Namespace  string         `json:"namespace"`
	ChunkIndex int            `json:"chunk_index"`
	Content    string         `json:"content"`
	Metadata   map[string]any `json:"metadata"`
}

// SearchResult is a chunk augmented with similarity score and source filename,
// returned by semantic search queries.
type SearchResult struct {
	ChunkID    uuid.UUID      `json:"chunk_id"`
	DocumentID uuid.UUID      `json:"document_id"`
	Namespace  string         `json:"namespace"`
	Filename   string         `json:"filename"`
	ChunkIndex int            `json:"chunk_index"`
	Content    string         `json:"content"`
	Score      float64        `json:"score"`
	Metadata   map[string]any `json:"metadata"`
}

// InsertChunks inserts a batch of chunks with their embeddings into the database.
// All chunks belong to the same document and namespace. The operation is
// performed in a single transaction for atomicity.
func (s *Store) InsertChunks(ctx context.Context, documentID uuid.UUID, namespace string, chunks []Chunk, embeddings [][]float32) error {
	if len(chunks) != len(embeddings) {
		return fmt.Errorf("chunks (%d) and embeddings (%d) length mismatch", len(chunks), len(embeddings))
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for i, chunk := range chunks {
		vec := pgvector.NewVector(embeddings[i])
		_, err := tx.Exec(ctx, `
			INSERT INTO chunks (document_id, namespace, chunk_index, content, embedding, metadata)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, documentID, namespace, chunk.ChunkIndex, chunk.Content, vec, chunk.Metadata)
		if err != nil {
			return fmt.Errorf("inserting chunk %d: %w", i, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing chunk transaction: %w", err)
	}
	return nil
}

// Search performs cosine similarity search over chunk embeddings, scoped to the
// given namespace. Pass an empty string to search across all namespaces.
// Returns the top `limit` most similar chunks joined with document filename.
func (s *Store) Search(ctx context.Context, queryEmbedding []float32, namespace string, limit int) ([]*SearchResult, error) {
	vec := pgvector.NewVector(queryEmbedding)

	var (
		rows interface{ Close() }
		err  error
	)

	if namespace != "" {
		rows, err = s.pool.Query(ctx, `
			SELECT
				c.id,
				c.document_id,
				c.namespace,
				d.filename,
				c.chunk_index,
				c.content,
				1 - (c.embedding <=> $1) AS score,
				c.metadata
			FROM chunks c
			JOIN documents d ON d.id = c.document_id
			WHERE c.embedding IS NOT NULL AND c.namespace = $2
			ORDER BY c.embedding <=> $1
			LIMIT $3
		`, vec, namespace, limit)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT
				c.id,
				c.document_id,
				c.namespace,
				d.filename,
				c.chunk_index,
				c.content,
				1 - (c.embedding <=> $1) AS score,
				c.metadata
			FROM chunks c
			JOIN documents d ON d.id = c.document_id
			WHERE c.embedding IS NOT NULL
			ORDER BY c.embedding <=> $1
			LIMIT $2
		`, vec, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("executing search query: %w", err)
	}

	// rows is *pgxpool.Rows — use type assertion to call Next/Scan/Close/Err.
	type scanner interface {
		Next() bool
		Scan(dest ...any) error
		Close()
		Err() error
	}
	sr := rows.(scanner)
	defer sr.Close()

	var results []*SearchResult
	for sr.Next() {
		r := &SearchResult{}
		if err := sr.Scan(
			&r.ChunkID, &r.DocumentID, &r.Namespace, &r.Filename,
			&r.ChunkIndex, &r.Content, &r.Score, &r.Metadata,
		); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}
		results = append(results, r)
	}
	if err := sr.Err(); err != nil {
		return nil, fmt.Errorf("iterating search results: %w", err)
	}
	return results, nil
}
