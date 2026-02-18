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

// Compile-time assertion: TEIEmbedder must implement Embedder.
var _ Embedder = (*TEIEmbedder)(nil)

// TEIEmbedder is an Embedder implementation that calls the Hugging Face
// Text Embeddings Inference (TEI) service over HTTP.
//
// e5 prefix handling ("passage: " / "query: ") is encapsulated here —
// callers pass raw text and never deal with model-specific prefixes.
type TEIEmbedder struct {
	baseURL    string
	httpClient *http.Client
}

// NewTEIEmbedder creates a new TEIEmbedder targeting the given base URL.
// The base URL should not have a trailing slash (e.g. "http://localhost:8080").
// A 30-second timeout is applied to each HTTP request.
func NewTEIEmbedder(baseURL string) *TEIEmbedder {
	return &TEIEmbedder{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// EmbedPassages prepends "passage: " to each text and calls the TEI /embed
// endpoint, returning one vector per input text in the same order.
//
// The "passage: " prefix is required by multilingual-e5 models for indexing —
// omitting it measurably degrades retrieval quality.
func (e *TEIEmbedder) EmbedPassages(ctx context.Context, texts []string) ([][]float32, error) {
	prefixed := make([]string, len(texts))
	for i, t := range texts {
		prefixed[i] = "passage: " + t
	}
	return e.embed(ctx, prefixed)
}

// EmbedQuery prepends "query: " to the text and calls the TEI /embed endpoint,
// returning a single vector for the query.
//
// The "query: " prefix is required by multilingual-e5 models for retrieval —
// omitting it measurably degrades retrieval quality.
func (e *TEIEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vecs, err := e.embed(ctx, []string{"query: " + text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("TEI embed: empty response for query")
	}
	return vecs[0], nil
}

// embed sends a POST request to the TEI /embed endpoint with the given inputs
// and decodes the response as [][]float32.
//
// Error messages distinguish between network failures, non-200 status codes,
// and JSON decode errors. Non-200 responses include the HTTP status and the
// response body (TEI returns error details there).
//
// Retry logic is intentionally absent — the caller is responsible for retry
// (TEI cold-start handling is done at the benchmark/integration level).
func (e *TEIEmbedder) embed(ctx context.Context, inputs []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{"inputs": inputs})
	if err != nil {
		return nil, fmt.Errorf("TEI embed: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("TEI embed: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TEI embed: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read the body — TEI returns error details in the response body.
		errBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("TEI embed: unexpected status %d (and could not read response body: %v)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("TEI embed: unexpected status %d: %s", resp.StatusCode, string(errBody))
	}

	var result [][]float32
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("TEI embed: decode response: %w", err)
	}

	return result, nil
}
