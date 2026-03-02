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

// Package ingestion implements the document ingestion pipeline.
// It orchestrates parsing, chunking, embedding generation, and database
// persistence. A worker pool processes jobs concurrently with exponential
// backoff retries and graceful failure handling.
package ingestion

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/achetronic/memex/internal/chunker"
	"github.com/achetronic/memex/internal/db"
	"github.com/achetronic/memex/internal/embedder"
	"github.com/achetronic/memex/internal/parser"
	"github.com/google/uuid"
)

const (
	baseRetryDelay = 2 * time.Second
)

// Job represents a document ingestion task enqueued by the API.
type Job struct {
	DocumentID uuid.UUID
	Namespace  string
	Filename   string
	FilePath   string
}

// Worker is the ingestion pipeline orchestrator. It reads jobs from a channel
// and processes them with retry logic. Create with NewWorker.
type Worker struct {
	store    *db.Store
	embedder *embedder.Embedder
	chunker  *chunker.Chunker
	log      *slog.Logger
	jobs     chan Job
	maxRetry int
}

// NewWorker creates a Worker and starts `poolSize` goroutines that process
// jobs from the returned channel. The caller must enqueue jobs via Enqueue.
func NewWorker(
	store *db.Store,
	emb *embedder.Embedder,
	chk *chunker.Chunker,
	log *slog.Logger,
	poolSize int,
	maxRetry int,
	queueSize int,
) *Worker {
	w := &Worker{
		store:    store,
		embedder: emb,
		chunker:  chk,
		log:      log,
		jobs:     make(chan Job, queueSize),
		maxRetry: maxRetry,
	}

	for i := 0; i < poolSize; i++ {
		go w.run(i)
	}

	return w
}

// Enqueue adds a job to the processing queue. Returns false if the queue is
// full (caller should respond with 429 Too Many Requests).
func (w *Worker) Enqueue(job Job) bool {
	select {
	case w.jobs <- job:
		return true
	default:
		return false
	}
}

func (w *Worker) run(id int) {
	w.log.Info("ingestion worker started", "worker_id", id)
	for job := range w.jobs {
		w.processWithRetry(job)
	}
}

// processWithRetry separates the pipeline into two phases:
//
//  1. Parse & chunk (deterministic, CPU-only) — executed once. If this
//     fails the error is permanent and retrying is pointless.
//  2. Embed & store (network/IO) — retried with exponential backoff
//     because these can fail transiently.
func (w *Worker) processWithRetry(job Job) {
	ctx := context.Background()

	if err := w.store.UpdateDocumentStatus(ctx, job.DocumentID, db.StatusProcessing, nil); err != nil {
		w.fail(job, fmt.Errorf("marking document as processing: %w", err))
		return
	}

	chunks, err := w.parseAndChunk(job)
	if err != nil {
		w.fail(job, err)
		return
	}

	var lastErr error
	for attempt := 1; attempt <= w.maxRetry; attempt++ {
		if err := w.embedAndStore(ctx, job, chunks); err != nil {
			lastErr = err
			w.log.Warn("ingestion attempt failed",
				"document_id", job.DocumentID,
				"filename", job.Filename,
				"attempt", attempt,
				"max_retries", w.maxRetry,
				"error", err,
			)
			if attempt < w.maxRetry {
				time.Sleep(backoff(attempt))
			}
			continue
		}

		w.log.Info("document ingested successfully",
			"document_id", job.DocumentID,
			"filename", job.Filename,
		)
		return
	}

	w.fail(job, lastErr)
}

// parseAndChunk runs the deterministic part of the pipeline: select parser,
// extract text, normalise, and split into chunks. Errors here are permanent.
func (w *Worker) parseAndChunk(job Job) ([]chunker.Chunk, error) {
	p, err := parser.ForFile(job.Filename)
	if err != nil {
		return nil, fmt.Errorf("selecting parser: %w", err)
	}

	data, err := os.ReadFile(job.FilePath)
	if err != nil {
		return nil, fmt.Errorf("reading file from disk: %w", err)
	}

	text, err := p.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parsing document: %w", err)
	}

	text = normaliseText(text)

	if text == "" {
		return nil, fmt.Errorf("parser returned empty text for %q", job.Filename)
	}

	chunks := w.chunker.Split(text)
	if len(chunks) == 0 {
		return nil, fmt.Errorf("chunker returned no chunks for %q", job.Filename)
	}

	return chunks, nil
}

// embedAndStore runs the IO-bound part of the pipeline: generate embeddings
// and persist chunks. It deletes any previously inserted chunks for this
// document before inserting, making retries idempotent.
func (w *Worker) embedAndStore(ctx context.Context, job Job, chunks []chunker.Chunk) error {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}

	vectors, err := w.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("generating embeddings: %w", err)
	}

	if err := w.store.DeleteChunksByDocument(ctx, job.DocumentID); err != nil {
		return fmt.Errorf("cleaning up previous chunks: %w", err)
	}

	dbChunks := make([]db.Chunk, len(chunks))
	for i, c := range chunks {
		dbChunks[i] = db.Chunk{
			ChunkIndex: c.Index,
			Content:    c.Content,
			Metadata:   map[string]any{"filename": job.Filename},
		}
	}

	if err := w.store.InsertChunks(ctx, job.DocumentID, job.Namespace, dbChunks, vectors); err != nil {
		return fmt.Errorf("storing chunks: %w", err)
	}

	if err := w.store.UpdateDocumentChunkCount(ctx, job.DocumentID, len(dbChunks)); err != nil {
		return fmt.Errorf("updating chunk count: %w", err)
	}

	if err := w.store.UpdateDocumentStatus(ctx, job.DocumentID, db.StatusCompleted, nil); err != nil {
		return fmt.Errorf("marking document as completed: %w", err)
	}

	if err := w.store.ClearFilePath(ctx, job.DocumentID); err != nil {
		w.log.Warn("failed to clear file_path in DB", "document_id", job.DocumentID, "error", err)
	}
	if err := os.Remove(job.FilePath); err != nil && !os.IsNotExist(err) {
		w.log.Warn("failed to remove file from disk", "path", job.FilePath, "error", err)
	}

	return nil
}

// fail marks a document as failed and logs the error.
func (w *Worker) fail(job Job, err error) {
	errMsg := err.Error()
	w.log.Error("ingestion failed",
		"document_id", job.DocumentID,
		"filename", job.Filename,
		"error", errMsg,
	)

	ctx := context.Background()
	if updateErr := w.store.UpdateDocumentStatus(ctx, job.DocumentID, db.StatusFailed, &errMsg); updateErr != nil {
		w.log.Error("failed to update document status to failed",
			"document_id", job.DocumentID,
			"error", updateErr,
		)
	}
}

// normaliseText cleans up parser output before chunking: strips NUL bytes,
// collapses runs of whitespace, and trims the result.
func normaliseText(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.Map(func(r rune) rune {
		if r != '\n' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)

	var buf strings.Builder
	buf.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		isSpace := r != '\n' && unicode.IsSpace(r)
		if isSpace {
			if !prevSpace {
				buf.WriteRune(' ')
			}
			prevSpace = true
			continue
		}
		prevSpace = false
		buf.WriteRune(r)
	}

	return strings.TrimSpace(buf.String())
}

func backoff(attempt int) time.Duration {
	d := baseRetryDelay
	for i := 1; i < attempt; i++ {
		d *= 2
	}
	return d
}
