package db

import (
	"context"
	"fmt"
	"time"

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

// Document represents a row in the documents table.
type Document struct {
	ID         uuid.UUID      `json:"id"`
	Filename   string         `json:"filename"`
	Format     string         `json:"format"`
	Status     DocumentStatus `json:"status"`
	Error      *string        `json:"error,omitempty"`
	ChunkCount int            `json:"chunk_count"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// CreateDocument inserts a new document row with status "pending" and returns
// the generated UUID.
func (s *Store) CreateDocument(ctx context.Context, filename, format string) (*Document, error) {
	doc := &Document{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO documents (filename, format)
		VALUES ($1, $2)
		RETURNING id, filename, format, status, error, chunk_count, created_at, updated_at
	`, filename, format).Scan(
		&doc.ID, &doc.Filename, &doc.Format, &doc.Status,
		&doc.Error, &doc.ChunkCount, &doc.CreatedAt, &doc.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting document: %w", err)
	}
	return doc, nil
}

// GetDocument retrieves a single document by its UUID. Returns an error if not found.
func (s *Store) GetDocument(ctx context.Context, id uuid.UUID) (*Document, error) {
	doc := &Document{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, filename, format, status, error, chunk_count, created_at, updated_at
		FROM documents WHERE id = $1
	`, id).Scan(
		&doc.ID, &doc.Filename, &doc.Format, &doc.Status,
		&doc.Error, &doc.ChunkCount, &doc.CreatedAt, &doc.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("getting document %s: %w", id, err)
	}
	return doc, nil
}

// ListDocuments returns all documents, optionally filtered by status.
// Pass an empty string to return all statuses.
func (s *Store) ListDocuments(ctx context.Context, status DocumentStatus) ([]*Document, error) {
	query := `
		SELECT id, filename, format, status, error, chunk_count, created_at, updated_at
		FROM documents
	`
	args := []any{}

	if status != "" {
		query += " WHERE status = $1"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing documents: %w", err)
	}
	defer rows.Close()

	var docs []*Document
	for rows.Next() {
		doc := &Document{}
		if err := rows.Scan(
			&doc.ID, &doc.Filename, &doc.Format, &doc.Status,
			&doc.Error, &doc.ChunkCount, &doc.CreatedAt, &doc.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning document row: %w", err)
		}
		docs = append(docs, doc)
	}
	return docs, nil
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

// DeleteDocument removes a document and all its chunks (via ON DELETE CASCADE).
func (s *Store) DeleteDocument(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM documents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting document %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("document %s not found", id)
	}
	return nil
}
