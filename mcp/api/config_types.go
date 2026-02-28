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

package api

import "time"

// ServerTransportHTTPConfig holds the address the HTTP server listens on.
type ServerTransportHTTPConfig struct {
	Host string `yaml:"host"`
}

// ServerTransportConfig selects the transport type and its specific options.
// Supported types: "stdio", "http".
type ServerTransportConfig struct {
	Type string                    `yaml:"type"`
	HTTP ServerTransportHTTPConfig `yaml:"http,omitempty"`
}

// ServerConfig holds top-level MCP server identity and transport settings.
type ServerConfig struct {
	Name      string                `yaml:"name"`
	Version   string                `yaml:"version"`
	Transport ServerTransportConfig `yaml:"transport,omitempty"`
}

// MemexAuthConfig controls how memex-mcp authenticates against the Memex API.
//
// Resolution order for each request (first non-empty value wins):
//  1. X-Memex-Api-Key header forwarded from the agent request (ForwardApiKey)
//  2. Static API key configured for the specific namespace (NamespaceKeys)
//  3. Static API key configured for the "*" wildcard namespace (NamespaceKeys)
//  4. No credential — compatible with Memex instances that have no auth yet.
type MemexAuthConfig struct {
	// ForwardApiKey enables forwarding of the X-Memex-Api-Key header from the
	// agent's incoming request verbatim to the Memex API. Only meaningful in
	// HTTP transport mode — requires JWT auth to be enabled, otherwise any
	// caller could inject an arbitrary key.
	ForwardApiKey bool `yaml:"forward_api_key,omitempty"`

	// NamespaceKeys maps namespace names to their static API keys.
	// Use "*" as the key for a catch-all fallback.
	// Example:
	//   namespace_keys:
	//     facturas: "key-abc123"
	//     "*":      "key-default"
	NamespaceKeys map[string]string `yaml:"namespace_keys,omitempty"`
}

// MemexConfig holds the connection details for the upstream Memex REST API.
type MemexConfig struct {
	// BaseURL is the root URL of the Memex API, e.g. "http://localhost:8080".
	BaseURL string `yaml:"base_url"`

	// DefaultNamespace is sent as X-Memex-Namespace when the caller does not
	// provide one explicitly. Leave empty to send no header by default.
	DefaultNamespace string `yaml:"default_namespace,omitempty"`

	// Auth configures how memex-mcp authenticates against the Memex API.
	// All fields are optional — omit the section entirely for no-auth setups.
	Auth MemexAuthConfig `yaml:"auth,omitempty"`
}

// AccessLogsConfig controls which HTTP headers are logged or redacted.
type AccessLogsConfig struct {
	ExcludedHeaders []string `yaml:"excluded_headers"`
	RedactedHeaders []string `yaml:"redacted_headers"`
}

// JWTValidationAllowCondition is a single CEL expression that must evaluate
// to true for a request to be considered authenticated.
type JWTValidationAllowCondition struct {
	Expression string `yaml:"expression"`
}

// JWTConfig configures the JWT validation middleware.
// When disabled the middleware is a no-op and all requests are allowed through.
type JWTConfig struct {
	Enabled         bool                          `yaml:"enabled"`
	JWKSUri         string                        `yaml:"jwks_uri,omitempty"`
	CacheInterval   time.Duration                 `yaml:"cache_interval,omitempty"`
	AllowConditions []JWTValidationAllowCondition `yaml:"allow_conditions,omitempty"`
}

// MiddlewareConfig groups all middleware configurations.
type MiddlewareConfig struct {
	AccessLogs AccessLogsConfig `yaml:"access_logs"`
	JWT        JWTConfig        `yaml:"jwt,omitempty"`
}

// OAuthAuthorizationServerConfig exposes the OAuth Authorization Server
// discovery document at /.well-known/oauth-authorization-server.
type OAuthAuthorizationServerConfig struct {
	Enabled   bool   `yaml:"enabled"`
	UrlSuffix string `yaml:"url_suffix,omitempty"`
	IssuerUri string `yaml:"issuer_uri"`
}

// OAuthProtectedResourceConfig exposes the OAuth Protected Resource metadata
// document at /.well-known/oauth-protected-resource.
type OAuthProtectedResourceConfig struct {
	Enabled   bool   `yaml:"enabled"`
	UrlSuffix string `yaml:"url_suffix,omitempty"`

	Resource                              string   `yaml:"resource"`
	AuthServers                           []string `yaml:"auth_servers"`
	JWKSUri                               string   `yaml:"jwks_uri"`
	ScopesSupported                       []string `yaml:"scopes_supported"`
	BearerMethodsSupported                []string `yaml:"bearer_methods_supported,omitempty"`
	ResourceSigningAlgValuesSupported     []string `yaml:"resource_signing_alg_values_supported,omitempty"`
	ResourceName                          string   `yaml:"resource_name,omitempty"`
	ResourceDocumentation                 string   `yaml:"resource_documentation,omitempty"`
	ResourcePolicyUri                     string   `yaml:"resource_policy_uri,omitempty"`
	ResourceTosUri                        string   `yaml:"resource_tos_uri,omitempty"`
	TLSClientCertificateBoundAccessTokens bool     `yaml:"tls_client_certificate_bound_access_tokens,omitempty"`
	AuthorizationDetailsTypesSupported    []string `yaml:"authorization_details_types_supported,omitempty"`
	DPoPSigningAlgValuesSupported         []string `yaml:"dpop_signing_alg_values_supported,omitempty"`
	DPoPBoundAccessTokensRequired         bool     `yaml:"dpop_bound_access_tokens_required,omitempty"`
}

// ToolPolicyConfig pairs a CEL expression with the set of tools it unlocks.
// The first policy whose expression evaluates to true wins.
type ToolPolicyConfig struct {
	Expression   string   `yaml:"expression"`
	AllowedTools []string `yaml:"allowed_tools"`
}

// NamespacePolicyConfig pairs a CEL expression with the namespaces it grants
// access to. Use "*" in allowed_namespaces to grant access to all namespaces.
type NamespacePolicyConfig struct {
	Expression         string   `yaml:"expression"`
	AllowedNamespaces  []string `yaml:"allowed_namespaces"`
}

// PoliciesConfig groups tool and namespace access-control policies.
type PoliciesConfig struct {
	Tools      []ToolPolicyConfig      `yaml:"tools"`
	Namespaces []NamespacePolicyConfig `yaml:"namespaces"`
}

// Configuration is the root configuration structure for memex-mcp.
// It is loaded once at startup from a YAML file.
type Configuration struct {
	Server                   ServerConfig                    `yaml:"server,omitempty"`
	Middleware               MiddlewareConfig                `yaml:"middleware,omitempty"`
	Policies                 PoliciesConfig                  `yaml:"policies,omitempty"`
	OAuthAuthorizationServer OAuthAuthorizationServerConfig  `yaml:"oauth_authorization_server,omitempty"`
	OAuthProtectedResource   OAuthProtectedResourceConfig    `yaml:"oauth_protected_resource,omitempty"`
	Memex                    MemexConfig                     `yaml:"memex"`
}
