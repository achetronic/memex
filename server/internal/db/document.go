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
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/google/uuid"
)

// DocumentStatus represents the lifecycle state of a document in the ingestion pipeline.
type DocumentStatus string

const (
	StatusPending    DocumentStatus = "pending"
	StatusProcessing DocumentStatus = "processing"
	StatusCompleted  DocumentStatus = "completed"
	StatusFailed     DocumentStatus = "failed"
)

// SortField represents the column to sort documents by.
type SortField string

const (
	SortByDate SortField = "created_at"
	SortByName SortField = "filename"
)

// SortOrder represents sort direction.
type SortOrder string

const (
	SortAsc  SortOrder = "ASC"
	SortDesc SortOrder = "DESC"
)

// ListDocumentsParams holds optional filters, sorting and pagination for ListDocuments.
type ListDocumentsParams struct {
	Status    DocumentStatus
	SortBy    SortField
	SortOrder SortOrder
	Limit     int
	Offset    int
}

// Document represents a row in the documents table.
type Document struct {
	ID         uuid.UUID      `json:"id"`
	Namespace  string         `json:"namespace"`
	Filename   string         `json:"filename"`
	Format     string         `json:"format"`
	Status     DocumentStatus `json:"status"`
	Error      *string        `json:"error,omitempty"`
	ChunkCount int            `json:"chunk_count"`
	FileHash   *string        `json:"file_hash,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// DocumentList wraps a page of documents with total count for pagination.
type DocumentList struct {
	Documents  []*Document `json:"documents"`
	Total      int         `json:"total"`
	Limit      int         `json:"limit"`
	Offset     int         `json:"offset"`
}

// CreateDocument inserts a new document row with status "pending" and an optional
// file hash for deduplication. Returns the inserted document.
func (s *Store) CreateDocument(ctx context.Context, namespace, filename, format, fileHash string) (*Document, error) {
	doc := &Document{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO documents (namespace, filename, format, file_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id, namespace, filename, format, status, error, chunk_count, file_hash, created_at, updated_at
	`, namespace, filename, format, fileHash).Scan(
		&doc.ID, &doc.Namespace, &doc.Filename, &doc.Format, &doc.Status,
		&doc.Error, &doc.ChunkCount, &doc.FileHash, &doc.CreatedAt, &doc.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting document: %w", err)
	}
	return doc, nil
}

// FindDocumentByHash looks up an existing document in the given namespace by its
// file hash. Returns (nil, nil) if no match is found.
func (s *Store) FindDocumentByHash(ctx context.Context, namespace, fileHash string) (*Document, error) {
	doc := &Document{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, namespace, filename, format, status, error, chunk_count, file_hash, created_at, updated_at
		FROM documents
		WHERE namespace = $1 AND file_hash = $2
		LIMIT 1
	`, namespace, fileHash).Scan(
		&doc.ID, &doc.Namespace, &doc.Filename, &doc.Format, &doc.Status,
		&doc.Error, &doc.ChunkCount, &doc.FileHash, &doc.CreatedAt, &doc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding document by hash: %w", err)
	}
	return doc, nil
}

// GetDocument retrieves a single document by its UUID, scoped to the given namespace.
// Returns an error if the document is not found or belongs to a different namespace.
func (s *Store) GetDocument(ctx context.Context, namespace string, id uuid.UUID) (*Document, error) {
	doc := &Document{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, namespace, filename, format, status, error, chunk_count, file_hash, created_at, updated_at
		FROM documents WHERE id = $1 AND namespace = $2
	`, id, namespace).Scan(
		&doc.ID, &doc.Namespace, &doc.Filename, &doc.Format, &doc.Status,
		&doc.Error, &doc.ChunkCount, &doc.FileHash, &doc.CreatedAt, &doc.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("getting document %s: %w", id, err)
	}
	return doc, nil
}

// ListDocuments returns a paginated, sorted page of documents in the given namespace.
// p.Limit defaults to 10 if zero. p.SortBy defaults to created_at. p.SortOrder defaults to DESC.
// Returns the page plus the total count matching the filters.
func (s *Store) ListDocuments(ctx context.Context, namespace string, p ListDocumentsParams) (*DocumentList, error) {
	// Apply defaults.
	if p.Limit <= 0 {
		p.Limit = 10
	}
	sortBy := p.SortBy
	if sortBy != SortByName && sortBy != SortByDate {
		sortBy = SortByDate
	}
	sortOrder := p.SortOrder
	if sortOrder != SortAsc && sortOrder != SortDesc {
		sortOrder = SortDesc
	}

	// Count query.
	countQuery := `SELECT COUNT(*) FROM documents WHERE namespace = $1`
	countArgs := []any{namespace}
	if p.Status != "" {
		countQuery += " AND status = $2"
		countArgs = append(countArgs, p.Status)
	}
	var total int
	if err := s.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting documents: %w", err)
	}

	// Data query — sort column and order are trusted constants, safe to interpolate.
	dataQuery := fmt.Sprintf(`
		SELECT id, namespace, filename, format, status, error, chunk_count, file_hash, created_at, updated_at
		FROM documents
		WHERE namespace = $1
	`)
	dataArgs := []any{namespace}
	if p.Status != "" {
		dataQuery += " AND status = $2"
		dataArgs = append(dataArgs, p.Status)
		dataQuery += fmt.Sprintf(" ORDER BY %s %s LIMIT $3 OFFSET $4", sortBy, sortOrder)
		dataArgs = append(dataArgs, p.Limit, p.Offset)
	} else {
		dataQuery += fmt.Sprintf(" ORDER BY %s %s LIMIT $2 OFFSET $3", sortBy, sortOrder)
		dataArgs = append(dataArgs, p.Limit, p.Offset)
	}

	rows, err := s.pool.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, fmt.Errorf("listing documents: %w", err)
	}
	defer rows.Close()

	docs := make([]*Document, 0)
	for rows.Next() {
		doc := &Document{}
		if err := rows.Scan(
			&doc.ID, &doc.Namespace, &doc.Filename, &doc.Format, &doc.Status,
			&doc.Error, &doc.ChunkCount, &doc.FileHash, &doc.CreatedAt, &doc.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning document row: %w", err)
		}
		docs = append(docs, doc)
	}

	return &DocumentList{
		Documents: docs,
		Total:     total,
		Limit:     p.Limit,
		Offset:    p.Offset,
	}, nil
}

// UpdateDocumentStatus updates the status and optional error message of a document,
// and refreshes its updated_at timestamp.
func (s *Store) UpdateDocumentStatus(ctx context.Context, id uuid.UUID, status DocumentStatus, errMsg *string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE documents
		SET status = $1, error = $2, updated_at = NOW()
		WHERE id = $3
	`, status, errMsg, id)
	if err != nil {
		return fmt.Errorf("updating document status: %w", err)
	}
	return nil
}

// UpdateDocumentChunkCount sets the final chunk count after ingestion completes.
func (s *Store) UpdateDocumentChunkCount(ctx context.Context, id uuid.UUID, count int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE documents
		SET chunk_count = $1, updated_at = NOW()
		WHERE id = $2
	`, count, id)
	if err != nil {
		return fmt.Errorf("updating chunk count: %w", err)
	}
	return nil
}

// DeleteDocument removes a document and all its chunks (via ON DELETE CASCADE),
// scoped to the given namespace. Returns an error if the document is not found
// or belongs to a different namespace.
func (s *Store) DeleteDocument(ctx context.Context, namespace string, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM documents WHERE id = $1 AND namespace = $2`, id, namespace)
	if err != nil {
		return fmt.Errorf("deleting document %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("document %s not found", id)
	}
	return nil
}
