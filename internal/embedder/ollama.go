package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Compile-time assertion: OllamaEmbedder must implement Embedder.
var _ Embedder = (*OllamaEmbedder)(nil)

// OllamaEmbedder is an Embedder implementation that calls the Ollama
// /api/embed endpoint over HTTP.
//
// Prefix handling ("search_document: " / "search_query: ") is encapsulated
// here — callers pass raw text and never deal with model-specific prefixes.
type OllamaEmbedder struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOllamaEmbedder creates a new OllamaEmbedder targeting the given base URL
// and model name. The base URL should not have a trailing slash.
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

// EmbedPassages prepends "search_document: " to each text and calls the Ollama
// /api/embed endpoint, returning one vector per input text in the same order.
func (e *OllamaEmbedder) EmbedPassages(ctx context.Context, texts []string) ([][]float32, error) {
	prefixed := make([]string, len(texts))
	for i, t := range texts {
		prefixed[i] = "search_document: " + t
	}
	return e.embed(ctx, prefixed)
}

// EmbedQuery prepends "search_query: " to the text and calls the Ollama
// /api/embed endpoint, returning a single vector for the query.
func (e *OllamaEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vecs, err := e.embed(ctx, []string{"search_query: " + text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("ollama embed: empty response for query")
	}
	return vecs[0], nil
}

// ollamaRequest is the JSON body for the Ollama /api/embed endpoint.
type ollamaRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// ollamaResponse is the JSON response from the Ollama /api/embed endpoint.
type ollamaResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// embed sends a POST request to the Ollama /api/embed endpoint with the given
// inputs and decodes the response. Ollama supports batched input natively.
func (e *OllamaEmbedder) embed(ctx context.Context, inputs []string) ([][]float32, error) {
	body, err := json.Marshal(ollamaRequest{
		Model: e.model,
		Input: inputs,
	})
	if err != nil {
		return nil, fmt.Errorf("ollama embed: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama embed: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("ollama embed: unexpected status %d (and could not read response body: %v)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("ollama embed: unexpected status %d: %s", resp.StatusCode, string(errBody))
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama embed: decode response: %w", err)
	}

	return result.Embeddings, nil
}
