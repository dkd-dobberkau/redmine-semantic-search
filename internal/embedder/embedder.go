// Package embedder provides the Embedder interface and implementations for
// converting text to dense float vectors.
//
// The Embedder interface abstracts the embedding model, allowing callers to
// switch between local (TEI) and cloud (e.g. OpenAI) implementations without
// changing any calling code. All model-specific requirements (e.g. e5 prefix
// strings) are encapsulated inside each concrete implementation.
package embedder

import "context"

// Embedder converts text to dense float vectors.
// Implementations handle all model-specific prefix requirements internally —
// callers never deal with "query: " or "passage: " strings.
type Embedder interface {
	// EmbedPassages embeds a batch of document passages for indexing.
	// For e5 models, implementations prepend "passage: " internally.
	// Returns one vector per input text in the same order as the input slice.
	EmbedPassages(ctx context.Context, texts []string) ([][]float32, error)

	// EmbedQuery embeds a single search query for retrieval.
	// For e5 models, implementations prepend "query: " internally.
	// Returns a single vector representing the query.
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}
