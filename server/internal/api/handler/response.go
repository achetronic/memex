// Package handler contains all HTTP handlers for the memex API.
// Handlers are intentionally thin: they parse input, delegate to domain
// packages, and write JSON responses. No business logic lives here.
package handler

import (
	"encoding/json"
	"net/http"
)

// writeJSON serialises v as JSON and writes it to w with the given status code.
// On marshalling failure it falls back to a plain 500 response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

// errorResponse is the standard error payload returned by all endpoints.
type errorResponse struct {
	Error string `json:"error"`
}

// writeError writes a JSON error response with the given status and message.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
