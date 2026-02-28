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

package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/achetronic/memex/internal/config"
)

// Auth returns a middleware that enforces API key + namespace authentication.
//
// When auth is disabled (no api_keys configured), all requests pass through and
// the namespace header is stored in context as-is (empty string if absent).
//
// When auth is enabled, the middleware validates in this order:
//  1. X-Memex-Api-Key header present → 401 if missing.
//  2. X-Memex-Namespace header present → 400 if missing (namespace is required
//     when auth is active so we always know the access scope).
//  3. Namespace is declared in config → 400 if unknown.
//  4. Key has access to the namespace → 403 if denied.
//
// On success the validated namespace is stored in the request context.
func Auth(cfg *config.Config, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			namespace := r.Header.Get("X-Memex-Namespace")

			if !cfg.IsAuthEnabled() {
				// Auth disabled: pass through, carrying whatever namespace was sent.
				next.ServeHTTP(w, r.WithContext(WithNamespace(r.Context(), namespace)))
				return
			}

			apiKey := r.Header.Get("X-Memex-Api-Key")
			if apiKey == "" {
				log.Warn("auth: missing API key", "path", r.URL.Path)
				writeAuthError(w, http.StatusUnauthorized, "missing X-Memex-Api-Key header")
				return
			}

			if namespace == "" {
				log.Warn("auth: missing namespace", "path", r.URL.Path)
				writeAuthError(w, http.StatusBadRequest, "missing X-Memex-Namespace header")
				return
			}

			if !cfg.IsNamespaceDeclared(namespace) {
				log.Warn("auth: unknown namespace", "namespace", namespace, "path", r.URL.Path)
				writeAuthError(w, http.StatusBadRequest, "unknown namespace: "+namespace)
				return
			}

			if !cfg.KeyHasNamespaceAccess(apiKey, namespace) {
				log.Warn("auth: access denied", "namespace", namespace, "path", r.URL.Path)
				writeAuthError(w, http.StatusForbidden, "access denied for this namespace")
				return
			}

			next.ServeHTTP(w, r.WithContext(WithNamespace(r.Context(), namespace)))
		})
	}
}

// writeAuthError writes a JSON error response for auth failures.
func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
