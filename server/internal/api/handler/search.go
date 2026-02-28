package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/achetronic/memex/internal/db"
	"github.com/achetronic/memex/internal/embedder"
)

// searchRequest is the expected JSON body for a semantic search request.
type searchRequest struct {
	// Query is the natural language question to search for.
	Query string `json:"query" example:"maximum aflatoxin levels in cereals"`
	// Limit is the maximum number of chunks to return. Defaults to server config.
	Limit int `json:"limit" example:"5"`
}

// searchResponse wraps the list of search results.
type searchResponse struct {
	Results []*db.SearchResult `json:"results"`
}

// Search groups handlers for the semantic search endpoint.
type Search struct {
	store        *db.Store
	embedder     *embedder.Embedder
	log          *slog.Logger
	defaultLimit int
}

// NewSearch constructs a Search handler.
func NewSearch(store *db.Store, emb *embedder.Embedder, log *slog.Logger, defaultLimit int) *Search {
	return &Search{
		store:        store,
		embedder:     emb,
		log:          log,
		defaultLimit: defaultLimit,
	}
}

// Query godoc
//
//	@Summary		Semantic search
//	@Description	Converts the query to an embedding and returns the most semantically similar chunks from all indexed documents.
//	@Tags			search
//	@Accept			json
//	@Produce		json
//	@Param			request	body		searchRequest	true	"Search parameters"
//	@Success		200		{object}	searchResponse
//	@Failure		400		{object}	errorResponse
//	@Failure		500		{object}	errorResponse
//	@Router			/search [post]
func (h *Search) Query(w http.ResponseWriter, r *http.Request) {
	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "'query' field is required")
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = h.defaultLimit
	}

	// Generate embedding for the query using the same model as ingestion.
	vec, err := h.embedder.Embed(r.Context(), req.Query)
	if err != nil {
		h.log.Error("failed to embed search query", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate query embedding")
		return
	}

	results, err := h.store.Search(r.Context(), vec, limit)
	if err != nil {
		h.log.Error("search query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	// Return an empty array rather than null when there are no results.
	if results == nil {
		results = []*db.SearchResult{}
	}

	writeJSON(w, http.StatusOK, searchResponse{Results: results})
}
