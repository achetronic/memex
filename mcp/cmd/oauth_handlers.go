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

package main

import (
	"encoding/json"
	"io"
	"net/http"

	"memex-mcp/internal/globals"
)

// handleOAuthAuthorizationServer returns an http.HandlerFunc that proxies the
// /.well-known/openid-configuration document from the configured issuer URI.
// This is required by the MCP OAuth Authorization Server metadata spec.
func handleOAuthAuthorizationServer(appCtx *globals.ApplicationContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		remoteURL := appCtx.Config.OAuthAuthorizationServer.IssuerUri + "/.well-known/openid-configuration"
		resp, err := http.Get(remoteURL) //nolint:noctx
		if err != nil {
			appCtx.Logger.Error("error fetching oauth-authorization-server metadata", "error", err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			appCtx.Logger.Error("error reading oauth-authorization-server response", "error", err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		_, _ = w.Write(body)
	}
}

// handleOAuthProtectedResource returns an http.HandlerFunc that serves the
// /.well-known/oauth-protected-resource metadata document built from the
// configuration. This is required by the MCP OAuth Protected Resource spec.
func handleOAuthProtectedResource(appCtx *globals.ApplicationContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := appCtx.Config.OAuthProtectedResource

		metadata := map[string]interface{}{
			"resource":                cfg.Resource,
			"authorization_servers":   cfg.AuthServers,
			"jwks_uri":                cfg.JWKSUri,
			"scopes_supported":        cfg.ScopesSupported,
			"bearer_methods_supported": cfg.BearerMethodsSupported,
		}

		if cfg.ResourceName != "" {
			metadata["resource_name"] = cfg.ResourceName
		}
		if cfg.ResourceDocumentation != "" {
			metadata["resource_documentation"] = cfg.ResourceDocumentation
		}
		if len(cfg.ResourceSigningAlgValuesSupported) > 0 {
			metadata["resource_signing_alg_values_supported"] = cfg.ResourceSigningAlgValuesSupported
		}

		body, err := json.Marshal(metadata)
		if err != nil {
			appCtx.Logger.Error("error marshalling oauth-protected-resource metadata", "error", err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		_, _ = w.Write(body)
	}
}
