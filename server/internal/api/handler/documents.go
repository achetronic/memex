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
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

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
//	@Description	Accepts a multipart/form-data upload, enqueues the document for ingestion, and returns its ID and initial status.
//	@Tags			documents
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			file	formData	file	true	"Document file to ingest"
//	@Success		202		{object}	db.Document
//	@Failure		400		{object}	errorResponse
//	@Failure		413		{object}	errorResponse
//	@Failure		422		{object}	errorResponse
//	@Failure		429		{object}	errorResponse
//	@Failure		500		{object}	errorResponse
//	@Router			/documents [post]
func (h *Documents) Upload(w http.ResponseWriter, r *http.Request) {
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

	format := strings.TrimPrefix(ext, ".")

	doc, err := h.store.CreateDocument(r.Context(), filename, format)
	if err != nil {
		h.log.Error("failed to create document record", "error", err)
		writeError(w, http.StatusInternalServerError, "could not create document")
		return
	}

	job := ingestion.Job{
		DocumentID: doc.ID,
		Filename:   filename,
		Content:    file,
	}

	if !h.worker.Enqueue(job) {
		// Queue full — clean up the created record to avoid orphans.
		_ = h.store.DeleteDocument(r.Context(), doc.ID)
		writeError(w, http.StatusTooManyRequests, "ingestion queue is full, try again later")
		return
	}

	writeJSON(w, http.StatusAccepted, doc)
}

// List godoc
//
//	@Summary		List documents
//	@Description	Returns all documents, optionally filtered by status (pending, processing, completed, failed).
//	@Tags			documents
//	@Produce		json
//	@Param			status	query		string	false	"Filter by status"	Enums(pending, processing, completed, failed)
//	@Success		200		{array}		db.Document
//	@Failure		500		{object}	errorResponse
//	@Router			/documents [get]
func (h *Documents) List(w http.ResponseWriter, r *http.Request) {
	status := db.DocumentStatus(r.URL.Query().Get("status"))
	docs, err := h.store.ListDocuments(r.Context(), status)
	if err != nil {
		h.log.Error("failed to list documents", "error", err)
		writeError(w, http.StatusInternalServerError, "could not list documents")
		return
	}
	writeJSON(w, http.StatusOK, docs)
}

// Get godoc
//
//	@Summary		Get a document
//	@Description	Returns the detail and current ingestion status of a single document.
//	@Tags			documents
//	@Produce		json
//	@Param			id	path		string	true	"Document UUID"
//	@Success		200	{object}	db.Document
//	@Failure		400	{object}	errorResponse
//	@Failure		404	{object}	errorResponse
//	@Failure		500	{object}	errorResponse
//	@Router			/documents/{id} [get]
func (h *Documents) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document ID")
		return
	}

	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// Delete godoc
//
//	@Summary		Delete a document
//	@Description	Removes a document and all its chunks from the database.
//	@Tags			documents
//	@Produce		json
//	@Param			id	path		string	true	"Document UUID"
//	@Success		204	"No Content"
//	@Failure		400	{object}	errorResponse
//	@Failure		404	{object}	errorResponse
//	@Failure		500	{object}	errorResponse
//	@Router			/documents/{id} [delete]
func (h *Documents) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document ID")
		return
	}

	if err := h.store.DeleteDocument(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
