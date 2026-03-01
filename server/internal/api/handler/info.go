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
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/achetronic/memex/internal/api/middleware"
	"github.com/achetronic/memex/internal/config"
)

// Info handles GET /api/v1/info.
//
// Behaviour depends on whether auth is enabled:
//
//   - Auth disabled: returns all declared namespaces. No credentials required.
//   - Auth enabled:  the Auth middleware already ran and validated the API key.
//     Returns only the namespaces that key is allowed to access.
//     Without a valid key the middleware rejects with 401 before this runs.
type Info struct {
	cfg *config.Config
	log *slog.Logger
}

// NewInfo creates an Info handler.
func NewInfo(cfg *config.Config, log *slog.Logger) *Info {
	return &Info{cfg: cfg, log: log}
}

// infoResponse is the payload returned by GET /api/v1/info.
type infoResponse struct {
	// AuthEnabled indicates whether API key authentication is active.
	AuthEnabled bool `json:"auth_enabled"`
	// Namespaces lists the namespaces the caller is allowed to access.
	// When auth is disabled this is the full list of declared namespaces.
	Namespaces []string `json:"namespaces"`
}

// Get godoc
//
//	@Summary		Get server info
//	@Description	Returns auth status and the namespaces the caller can access.
//	@Description	When auth is disabled, all declared namespaces are returned without credentials.
//	@Description	When auth is enabled, a valid X-Memex-Api-Key header is required.
//	@Tags			info
//	@Produce		json
//	@Success		200	{object}	infoResponse
//	@Failure		401	{object}	map[string]string
//	@Router			/info [get]
func (h *Info) Get(w http.ResponseWriter, r *http.Request) {
	authEnabled := len(h.cfg.Auth.APIKeys) > 0

	var namespaces []string

	if authEnabled {
		// Auth middleware already ran — pull the allowed namespaces it stored in context.
		allowed := middleware.AllowedNamespacesFromContext(r.Context())
		if len(allowed) == 1 && allowed[0] == "*" {
			// Wildcard key: return all declared namespaces.
			for _, ns := range h.cfg.Namespaces {
				namespaces = append(namespaces, ns.Name)
			}
		} else {
			namespaces = allowed
		}
	} else {
		// No auth: return all declared namespaces.
		for _, ns := range h.cfg.Namespaces {
			namespaces = append(namespaces, ns.Name)
		}
	}

	if namespaces == nil {
		namespaces = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(infoResponse{
		AuthEnabled: authEnabled,
		Namespaces:  namespaces,
	})
}
