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

// Package embedder provides a client for generating text embeddings via any
// OpenAI-compatible API. This includes OpenAI itself, Ollama (v1/embeddings),
// Groq, Together, and any other provider that speaks the OpenAI embeddings
// protocol. Configure via OPENAI_BASE_URL, OPENAI_API_KEY, and
// OPENAI_EMBEDDING_MODEL environment variables.
package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	// defaultTimeout is the HTTP client timeout for embedding requests.
	defaultTimeout = 60 * time.Second

	// embeddingsPath is the OpenAI-compatible embeddings endpoint path.
	embeddingsPath = "/v1/embeddings"
)

// Embedder calls an OpenAI-compatible API to generate text embeddings.
// Works with OpenAI, Ollama, Groq, Together, and any compatible provider.
type Embedder struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// New creates an Embedder targeting the given OpenAI-compatible base URL,
// using the provided API key and model name.
//
// For Ollama: baseURL = "http://localhost:11434", apiKey = "ollama" (any non-empty string)
// For OpenAI: baseURL = "https://api.openai.com", apiKey = "sk-..."
func New(baseURL, apiKey, model string) *Embedder {
	return &Embedder{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// embeddingRequest is the JSON payload for the OpenAI embeddings endpoint.
type embeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// embeddingResponse is the JSON response from the OpenAI embeddings endpoint.
type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed generates an embedding vector for the given text using the configured
// model. Returns a float32 slice whose length equals the model's embedding
// dimension. The caller must use the same model for ingestion and search.
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	payload, err := json.Marshal(embeddingRequest{
		Model: e.model,
		Input: text,
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		e.baseURL+embeddingsPath,
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("creating embedding HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling embeddings API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embeddings API returned status %d", resp.StatusCode)
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding embeddings response: %w", err)
	}

	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embeddings API returned empty vector for model %q", e.model)
	}

	return result.Data[0].Embedding, nil
}

// EmbedBatch generates embeddings for a slice of texts sequentially.
// Returns vectors in the same order as the input texts.
// Stops and returns an error with the failing index on any error.
func (e *Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))
	for i, text := range texts {
		vec, err := e.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("embedding text at index %d: %w", i, err)
		}
		vectors = append(vectors, vec)
	}
	return vectors, nil
}

// Ping verifies that the embeddings API is reachable and the configured model
// is available by generating an embedding for a short test string.
// Returns nil on success.
func (e *Embedder) Ping(ctx context.Context) error {
	if _, err := e.Embed(ctx, "ping"); err != nil {
		return fmt.Errorf("pinging embeddings API at %s with model %q: %w", e.baseURL, e.model, err)
	}
	return nil
}
