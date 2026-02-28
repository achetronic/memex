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

// Package memex provides a thin HTTP client for the Memex REST API.
// It handles namespace routing via X-Memex-Namespace and API key resolution
// via X-Memex-Api-Key, keeping tool handlers free of HTTP concerns.
package memex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"memex-mcp/api"
)

const (
	// NamespaceHeader scopes every Memex API request to a logical namespace.
	NamespaceHeader = "X-Memex-Namespace"

	// ApiKeyHeader is the header name used to authenticate against the Memex API.
	ApiKeyHeader = "X-Memex-Api-Key"
)

// Client is a thin wrapper around http.Client that speaks to the Memex API.
type Client struct {
	baseURL          string
	defaultNamespace string
	auth             api.MemexAuthConfig
	http             *http.Client
}

// NewClient creates a Memex API client.
//
// baseURL is the root of the API (e.g. "http://localhost:8080").
// defaultNamespace is used when the caller does not provide one explicitly.
// auth holds the API key resolution rules.
func NewClient(baseURL, defaultNamespace string, auth api.MemexAuthConfig) *Client {
	return &Client{
		baseURL:          baseURL,
		defaultNamespace: defaultNamespace,
		auth:             auth,
		http:             &http.Client{Timeout: 60 * time.Second},
	}
}

// ResolveApiKey determines the API key to send to Memex for a given namespace
// and forwarded header value, following this precedence:
//
//  1. forwardedKey — the value the agent passed in the configured forward header
//  2. Exact namespace match in NamespaceKeys config
//  3. Wildcard "*" entry in NamespaceKeys config
//  4. Empty string — no credential sent (no-auth Memex instances)
func (c *Client) ResolveApiKey(namespace, forwardedKey string) string {
	if forwardedKey != "" {
		return forwardedKey
	}
	if len(c.auth.NamespaceKeys) == 0 {
		return ""
	}
	if key, ok := c.auth.NamespaceKeys[namespace]; ok {
		return key
	}
	return c.auth.NamespaceKeys["*"]
}

// ForwardHeader returns the configured header name that agents use to pass an
// API key through to the Memex API. Returns an empty string if not configured.
func (c *Client) ForwardHeader() string {
	return c.auth.ForwardHeader
}

// do executes an HTTP request against the Memex API, setting the namespace and
// API key headers according to the resolved values provided by the caller.
func (c *Client) do(method, path, namespace, apiKey string, body interface{}) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshalling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Namespace: caller > default > none
	ns := namespace
	if ns == "" {
		ns = c.defaultNamespace
	}
	if ns != "" {
		req.Header.Set(NamespaceHeader, ns)
	}

	// API key: resolved by caller via ResolveApiKey
	if apiKey != "" {
		req.Header.Set(ApiKeyHeader, apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response body: %w", err)
	}

	return data, resp.StatusCode, nil
}

// ─── Documents ───────────────────────────────────────────────────────────────

// DocumentStatus represents the ingestion lifecycle of a document.
type DocumentStatus string

const (
	StatusPending    DocumentStatus = "pending"
	StatusProcessing DocumentStatus = "processing"
	StatusCompleted  DocumentStatus = "completed"
	StatusFailed     DocumentStatus = "failed"
)

// Document is the representation returned by the Memex API.
type Document struct {
	ID        string         `json:"id"`
	Filename  string         `json:"filename"`
	Status    DocumentStatus `json:"status"`
	Error     string         `json:"error,omitempty"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
}

// ListDocumentsResponse wraps the list endpoint response.
type ListDocumentsResponse struct {
	Documents []Document `json:"documents"`
}

// ListDocuments returns all documents in the given namespace, optionally
// filtered by status. Pass an empty status to return all.
func (c *Client) ListDocuments(namespace, apiKey, status string) ([]Document, error) {
	path := "/api/v1/documents"
	if status != "" {
		path += "?status=" + status
	}

	data, code, err := c.do(http.MethodGet, path, namespace, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", code, string(data))
	}

	var resp ListDocumentsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing list response: %w", err)
	}
	return resp.Documents, nil
}

// GetDocument returns the detail and current ingestion status of a single document.
func (c *Client) GetDocument(namespace, apiKey, id string) (*Document, error) {
	data, code, err := c.do(http.MethodGet, "/api/v1/documents/"+id, namespace, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", code, string(data))
	}

	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing document response: %w", err)
	}
	return &doc, nil
}

// UploadDocument uploads a file to Memex using multipart/form-data.
func (c *Client) UploadDocument(namespace, apiKey, filename string, content []byte) (*Document, error) {
	var buf bytes.Buffer
	boundary := "memex-mcp-boundary"

	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString(fmt.Sprintf(`Content-Disposition: form-data; name="file"; filename="%s"`, filename) + "\r\n")
	buf.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	buf.Write(content)
	buf.WriteString("\r\n--" + boundary + "--\r\n")

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/v1/documents", &buf)
	if err != nil {
		return nil, fmt.Errorf("creating upload request: %w", err)
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	ns := namespace
	if ns == "" {
		ns = c.defaultNamespace
	}
	if ns != "" {
		req.Header.Set(NamespaceHeader, ns)
	}
	if apiKey != "" {
		req.Header.Set(ApiKeyHeader, apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("uploading document: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(data))
	}

	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing upload response: %w", err)
	}
	return &doc, nil
}

// DeleteDocument removes a document and all its chunks from the given namespace.
func (c *Client) DeleteDocument(namespace, apiKey, id string) error {
	data, code, err := c.do(http.MethodDelete, "/api/v1/documents/"+id, namespace, apiKey, nil)
	if err != nil {
		return err
	}
	if code != http.StatusOK && code != http.StatusNoContent {
		return fmt.Errorf("unexpected status %d: %s", code, string(data))
	}
	return nil
}

// ─── Search ──────────────────────────────────────────────────────────────────

// SearchRequest is the payload for the semantic search endpoint.
type SearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

// SearchResult represents a single chunk returned by a semantic search.
type SearchResult struct {
	DocumentID string  `json:"document_id"`
	Filename   string  `json:"filename"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
}

// SearchResponse wraps the search endpoint response.
type SearchResponse struct {
	Results []SearchResult `json:"results"`
}

// Search performs a semantic search in the given namespace.
func (c *Client) Search(namespace, apiKey, query string, limit int) ([]SearchResult, error) {
	data, code, err := c.do(http.MethodPost, "/api/v1/search", namespace, apiKey, SearchRequest{
		Query: query,
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", code, string(data))
	}

	var resp SearchResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing search response: %w", err)
	}
	return resp.Results, nil
}

// ─── Health ──────────────────────────────────────────────────────────────────

// HealthResponse is the payload returned by the health endpoint.
type HealthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Embedder string `json:"embedder"`
}

// Health returns the current health of the upstream Memex instance.
func (c *Client) Health() (*HealthResponse, error) {
	data, code, err := c.do(http.MethodGet, "/api/v1/health", "", "", nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", code, string(data))
	}

	var h HealthResponse
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("parsing health response: %w", err)
	}
	return &h, nil
}
