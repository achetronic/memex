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

package handler

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	mw "github.com/achetronic/memex/internal/api/middleware"
	"github.com/achetronic/memex/internal/db"
	"github.com/achetronic/memex/internal/ingestion"
	"github.com/achetronic/memex/internal/parser"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Documents groups handlers that operate on document resources.
type Documents struct {
	store          *db.Store
	worker         *ingestion.Worker
	log            *slog.Logger
	maxUploadBytes int64
}

// NewDocuments constructs a Documents handler group.
func NewDocuments(store *db.Store, worker *ingestion.Worker, log *slog.Logger, maxUploadMB int64) *Documents {
	return &Documents{
		store:          store,
		worker:         worker,
		log:            log,
		maxUploadBytes: maxUploadMB * 1024 * 1024,
	}
}

// Upload godoc
//
//	@Summary		Upload a document
//	@Description	Accepts a multipart/form-data upload, checks for duplicates by SHA-256 hash,
//	@Description	enqueues the document for ingestion, and returns its ID and initial status.
//	@Tags			documents
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			X-Memex-Namespace	header		string	false	"Target namespace"
//	@Param			file				formData	file	true	"Document file to ingest"
//	@Success		202					{object}	db.Document
//	@Failure		400					{object}	errorResponse
//	@Failure		409					{object}	errorResponse	"Duplicate file"
//	@Failure		413					{object}	errorResponse
//	@Failure		422					{object}	errorResponse
//	@Failure		429					{object}	errorResponse
//	@Failure		500					{object}	errorResponse
//	@Router			/documents [post]
func (h *Documents) Upload(w http.ResponseWriter, r *http.Request) {
	namespace := mw.NamespaceFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes)

	if err := r.ParseMultipartForm(h.maxUploadBytes); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file too large: max %d MB", h.maxUploadBytes/1024/1024))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "field 'file' is required")
		return
	}
	defer file.Close()

	filename := filepath.Base(header.Filename)
	ext := strings.ToLower(filepath.Ext(filename))

	if _, err := parser.ForFile(filename); err != nil {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("unsupported format: %q", ext))
		return
	}

	// Compute SHA-256 of the file content for deduplication.
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		h.log.Error("failed to hash file", "error", err)
		writeError(w, http.StatusInternalServerError, "could not read file")
		return
	}
	fileHash := fmt.Sprintf("%x", hasher.Sum(nil))

	// Seek back to the beginning so the ingestion worker can read the file.
	if seeker, ok := file.(io.Seeker); ok {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			writeError(w, http.StatusInternalServerError, "could not process file")
			return
		}
	}

	// Dedup check: reject if the same file (by hash) already exists in this namespace.
	existing, err := h.store.FindDocumentByHash(r.Context(), namespace, fileHash)
	if err != nil {
		h.log.Error("dedup check failed", "error", err)
		writeError(w, http.StatusInternalServerError, "could not check for duplicates")
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, fmt.Sprintf(
			"duplicate file: %q was already uploaded (id: %s)", existing.Filename, existing.ID,
		))
		return
	}

	format := strings.TrimPrefix(ext, ".")

	doc, err := h.store.CreateDocument(r.Context(), namespace, filename, format, fileHash)
	if err != nil {
		h.log.Error("failed to create document record", "error", err)
		writeError(w, http.StatusInternalServerError, "could not create document")
		return
	}
	job := ingestion.Job{
		DocumentID: doc.ID,
		Namespace:  namespace,
		Filename:   filename,
		Content:    file,
	}

	if !h.worker.Enqueue(job) {
		// Queue full — clean up the created record to avoid orphans.
		_ = h.store.DeleteDocument(r.Context(), namespace, doc.ID)
		writeError(w, http.StatusTooManyRequests, "ingestion queue is full, try again later")
		return
	}

	writeJSON(w, http.StatusAccepted, doc)
}

// List godoc
//
//	@Summary		List documents
//	@Description	Returns a paginated, sorted list of documents in the active namespace.
//	@Tags			documents
//	@Produce		json
//	@Param			X-Memex-Namespace	header	string	false	"Target namespace"
//	@Param			status				query	string	false	"Filter by status"	Enums(pending, processing, completed, failed)
//	@Param			sort_by				query	string	false	"Sort field"		Enums(created_at, filename)
//	@Param			sort_order			query	string	false	"Sort direction"	Enums(asc, desc)
//	@Param			limit				query	int		false	"Page size (default 10)"
//	@Param			offset				query	int		false	"Page offset (default 0)"
//	@Success		200					{object}	db.DocumentList
//	@Failure		500					{object}	errorResponse
//	@Router			/documents [get]
func (h *Documents) List(w http.ResponseWriter, r *http.Request) {
	namespace := mw.NamespaceFromContext(r.Context())
	q := r.URL.Query()

	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	sortBy := db.SortField(q.Get("sort_by"))
	sortOrder := db.SortOrder(strings.ToUpper(q.Get("sort_order")))

	result, err := h.store.ListDocuments(r.Context(), namespace, db.ListDocumentsParams{
		Status:    db.DocumentStatus(q.Get("status")),
		SortBy:    sortBy,
		SortOrder: sortOrder,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		h.log.Error("failed to list documents", "error", err)
		writeError(w, http.StatusInternalServerError, "could not list documents")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// Get godoc
//
//	@Summary		Get a document
//	@Description	Returns the detail and current ingestion status of a single document in the active namespace.
//	@Tags			documents
//	@Produce		json
//	@Param			X-Memex-Namespace	header	string	false	"Target namespace"
//	@Param			id					path	string	true	"Document UUID"
//	@Success		200	{object}	db.Document
//	@Failure		400	{object}	errorResponse
//	@Failure		404	{object}	errorResponse
//	@Failure		500	{object}	errorResponse
//	@Router			/documents/{id} [get]
func (h *Documents) Get(w http.ResponseWriter, r *http.Request) {
	namespace := mw.NamespaceFromContext(r.Context())

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document ID")
		return
	}

	doc, err := h.store.GetDocument(r.Context(), namespace, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// Delete godoc
//
//	@Summary		Delete a document
//	@Description	Removes a document and all its chunks from the active namespace.
//	@Tags			documents
//	@Produce		json
//	@Param			X-Memex-Namespace	header	string	false	"Target namespace"
//	@Param			id					path	string	true	"Document UUID"
//	@Success		204	"No Content"
//	@Failure		400	{object}	errorResponse
//	@Failure		404	{object}	errorResponse
//	@Failure		500	{object}	errorResponse
//	@Router			/documents/{id} [delete]
func (h *Documents) Delete(w http.ResponseWriter, r *http.Request) {
	namespace := mw.NamespaceFromContext(r.Context())

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document ID")
		return
	}

	if err := h.store.DeleteDocument(r.Context(), namespace, id); err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
