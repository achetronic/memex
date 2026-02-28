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
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/achetronic/memex/internal/db"
	"github.com/achetronic/memex/internal/embedder"
)

// healthResponse is the payload returned by the health endpoint.
type healthResponse struct {
	Status   string            `json:"status"`
	Services map[string]string `json:"services"`
}

// Health groups handlers for the health check endpoint.
type Health struct {
	store    *db.Store
	embedder *embedder.Embedder
	log      *slog.Logger
}

// NewHealth constructs a Health handler.
func NewHealth(store *db.Store, emb *embedder.Embedder, log *slog.Logger) *Health {
	return &Health{store: store, embedder: emb, log: log}
}

// Check godoc
//
//	@Summary		Health check
//	@Description	Returns the overall system status and the connectivity status of each dependency (database, Ollama).
//	@Tags			health
//	@Produce		json
//	@Success		200	{object}	healthResponse
//	@Failure		503	{object}	healthResponse
//	@Router			/health [get]
func (h *Health) Check(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	services := map[string]string{}
	healthy := true

	// Check database connectivity.
	if _, err := h.store.ListDocuments(ctx, ""); err != nil {
		services["database"] = "unhealthy: " + err.Error()
		healthy = false
	} else {
		services["database"] = "healthy"
	}

	// Check Ollama connectivity.
	if err := h.embedder.Ping(ctx); err != nil {
		services["ollama"] = "unhealthy: " + err.Error()
		healthy = false
	} else {
		services["ollama"] = "healthy"
	}

	status := "ok"
	code := http.StatusOK
	if !healthy {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, healthResponse{
		Status:   status,
		Services: services,
	})
}
