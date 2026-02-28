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
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/achetronic/memex/internal/chunker"
	"github.com/achetronic/memex/internal/db"
	"github.com/achetronic/memex/internal/embedder"
	"github.com/achetronic/memex/internal/parser"
	"github.com/google/uuid"
)

const (
	// baseRetryDelay is the initial wait before the first retry.
	baseRetryDelay = 2 * time.Second
)

// Job represents a document ingestion task enqueued by the API.
type Job struct {
	DocumentID uuid.UUID
	Namespace  string
	Filename   string
	Content    io.Reader
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

// run is the main loop for a single worker goroutine. It processes jobs
// sequentially, retrying on failure with exponential backoff.
func (w *Worker) run(id int) {
	w.log.Info("ingestion worker started", "worker_id", id)
	for job := range w.jobs {
		w.processWithRetry(job)
	}
}

// processWithRetry attempts to process a job up to maxRetry times.
// On exhaustion it marks the document as failed and logs the error.
// It always moves on to the next job regardless of outcome.
func (w *Worker) processWithRetry(job Job) {
	var lastErr error

	for attempt := 1; attempt <= w.maxRetry; attempt++ {
		ctx := context.Background()

		if err := w.process(ctx, job); err != nil {
			lastErr = err
			w.log.Warn("ingestion attempt failed",
				"document_id", job.DocumentID,
				"filename", job.Filename,
				"attempt", attempt,
				"max_retries", w.maxRetry,
				"error", err,
			)

			if attempt < w.maxRetry {
				delay := backoff(attempt)
				time.Sleep(delay)
			}
			continue
		}

		w.log.Info("document ingested successfully",
			"document_id", job.DocumentID,
			"filename", job.Filename,
		)
		return
	}

	// All attempts exhausted — mark document as failed.
	errMsg := lastErr.Error()
	w.log.Error("ingestion failed after all retries",
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

// process runs the full ingestion pipeline for a single job:
// parse → chunk → embed → store. It updates document status at each stage.
func (w *Worker) process(ctx context.Context, job Job) error {
	// Mark as processing.
	if err := w.store.UpdateDocumentStatus(ctx, job.DocumentID, db.StatusProcessing, nil); err != nil {
		return fmt.Errorf("marking document as processing: %w", err)
	}

	// 1. Parse: extract plain text from the raw file.
	p, err := parser.ForFile(job.Filename)
	if err != nil {
		return fmt.Errorf("selecting parser: %w", err)
	}

	text, err := p.Parse(job.Content)
	if err != nil {
		return fmt.Errorf("parsing document: %w", err)
	}

	if text == "" {
		return fmt.Errorf("parser returned empty text for %q", job.Filename)
	}

	// 2. Chunk: split text into overlapping segments.
	rawChunks := w.chunker.Split(text)
	if len(rawChunks) == 0 {
		return fmt.Errorf("chunker returned no chunks for %q", job.Filename)
	}

	// 3. Embed: generate a vector for each chunk.
	texts := make([]string, len(rawChunks))
	for i, c := range rawChunks {
		texts[i] = c.Content
	}

	vectors, err := w.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("generating embeddings: %w", err)
	}

	// 4. Store: persist chunks and embeddings in a single transaction.
	dbChunks := make([]db.Chunk, len(rawChunks))
	for i, c := range rawChunks {
		dbChunks[i] = db.Chunk{
			ChunkIndex: c.Index,
			Content:    c.Content,
			Metadata:   map[string]any{"filename": job.Filename},
		}
	}

	if err := w.store.InsertChunks(ctx, job.DocumentID, job.Namespace, dbChunks, vectors); err != nil {
		return fmt.Errorf("storing chunks: %w", err)
	}

	// Update chunk count and mark completed.
	if err := w.store.UpdateDocumentChunkCount(ctx, job.DocumentID, len(dbChunks)); err != nil {
		return fmt.Errorf("updating chunk count: %w", err)
	}

	if err := w.store.UpdateDocumentStatus(ctx, job.DocumentID, db.StatusCompleted, nil); err != nil {
		return fmt.Errorf("marking document as completed: %w", err)
	}

	return nil
}

// backoff returns the wait duration for the given attempt number using
// exponential backoff: baseRetryDelay * 2^(attempt-1).
func backoff(attempt int) time.Duration {
	d := baseRetryDelay
	for i := 1; i < attempt; i++ {
		d *= 2
	}
	return d
}
