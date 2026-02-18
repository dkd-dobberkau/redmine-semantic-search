// Package main implements a standalone Recall@10 benchmark for the
// multilingual-e5-base embedding model on representative DE/EN Redmine content.
//
// The benchmark is the go/no-go gate for the embedding model choice. It:
//   - Embeds 50+ synthetic DE/EN QA pairs
//   - Upserts all passages into a temporary Qdrant collection
//   - For each query, searches top-K and checks if the correct passage appears
//   - Computes Recall@K and MRR@K as quality metrics
//   - Exits with code 0 on pass (Recall@K >= threshold), code 1 on fail
//
// The benchmark also runs with --no-prefix to prove that e5 prefix handling
// (passage:/query:) measurably improves retrieval quality.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/qdrant/go-client/qdrant"
)

// rawEmbedder calls the TEI /embed endpoint directly, without adding any
// e5 prefixes. Used with --no-prefix to measure the prefix contribution.
type rawEmbedder struct {
	baseURL    string
	httpClient *http.Client
}

func newRawEmbedder(baseURL string) *rawEmbedder {
	return &rawEmbedder{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *rawEmbedder) embed(ctx context.Context, inputs []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{"inputs": inputs})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("unexpected status %d (could not read body: %v)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(errBody))
	}

	var result [][]float32
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

// embedWithRetry wraps the first embedding call with exponential backoff for
// up to 2 minutes to handle TEI cold start (model loading delay).
func embedWithRetry(ctx context.Context, embedFn func(context.Context, []string) ([][]float32, error), inputs []string) ([][]float32, error) {
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 2 * time.Minute
	bo.InitialInterval = 2 * time.Second
	bo.MaxInterval = 15 * time.Second

	var result [][]float32
	err := backoff.RetryNotify(
		func() error {
			var err error
			result, err = embedFn(ctx, inputs)
			return err
		},
		bo,
		func(err error, d time.Duration) {
			slog.Info("TEI not ready, retrying", "error", err, "wait", d)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("TEI cold-start retry exhausted: %w", err)
	}
	return result, nil
}

const vectorDimension = 768

func main() {
	qdrantHost := flag.String("qdrant-host", "localhost", "Qdrant host")
	qdrantPort := flag.Int("qdrant-port", 6334, "Qdrant gRPC port")
	embeddingURL := flag.String("embedding-url", "http://localhost:8080", "TEI base URL (no trailing slash)")
	threshold := flag.Float64("threshold", 0.75, "Minimum Recall@K to pass")
	k := flag.Uint64("k", 10, "Number of results to retrieve (K)")
	noPrefix := flag.Bool("no-prefix", false, "Disable e5 prefixes (passage:/query:) to measure prefix contribution")
	flag.Parse()

	ctx := context.Background()

	// Determine prefix mode label for output.
	prefixLabel := "enabled (passage:/query:)"
	if *noPrefix {
		prefixLabel = "disabled (no prefix)"
	}

	// Create Qdrant client.
	qdrantClient, err := qdrant.NewClient(&qdrant.Config{
		Host: *qdrantHost,
		Port: *qdrantPort,
	})
	if err != nil {
		slog.Error("failed to create Qdrant client", "error", err)
		os.Exit(1)
	}
	defer qdrantClient.Close()

	// Create a unique temporary collection name.
	collectionName := fmt.Sprintf("bench_recall_temp_%d", time.Now().Unix())
	slog.Info("using temporary collection", "name", collectionName)

	// Ensure the collection is deleted after the benchmark, even on failure.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := qdrantClient.DeleteCollection(cleanupCtx, collectionName); err != nil {
			slog.Warn("failed to delete temporary collection", "name", collectionName, "error", err)
		} else {
			slog.Info("temporary collection deleted", "name", collectionName)
		}
	}()

	// Create the temporary collection with 768d Cosine vectors.
	// No payload indexes needed — this is a benchmark collection only.
	if err := createBenchCollection(ctx, qdrantClient, collectionName); err != nil {
		slog.Error("failed to create benchmark collection", "error", err)
		os.Exit(1)
	}

	pairs := BenchmarkPairs

	// Build the passage embedding function (with or without prefix).
	var passageEmbedFn func(ctx context.Context, texts []string) ([][]float32, error)
	var queryEmbedFn func(ctx context.Context, text string) ([]float32, error)

	if *noPrefix {
		raw := newRawEmbedder(*embeddingURL)
		passageEmbedFn = raw.embed
		queryEmbedFn = func(ctx context.Context, text string) ([]float32, error) {
			vecs, err := raw.embed(ctx, []string{text})
			if err != nil {
				return nil, err
			}
			if len(vecs) == 0 {
				return nil, fmt.Errorf("empty embedding response")
			}
			return vecs[0], nil
		}
	} else {
		passageEmbedFn = func(ctx context.Context, texts []string) ([][]float32, error) {
			prefixed := make([]string, len(texts))
			for i, t := range texts {
				prefixed[i] = "passage: " + t
			}
			return embedDirect(ctx, *embeddingURL, prefixed)
		}
		queryEmbedFn = func(ctx context.Context, text string) ([]float32, error) {
			vecs, err := embedDirect(ctx, *embeddingURL, []string{"query: " + text})
			if err != nil {
				return nil, err
			}
			if len(vecs) == 0 {
				return nil, fmt.Errorf("empty embedding response")
			}
			return vecs[0], nil
		}
	}

	// Build passage texts for embedding.
	passageTexts := make([]string, len(pairs))
	for i, p := range pairs {
		passageTexts[i] = p.Passage
	}

	// Embed all passages — use retry for the first batch (TEI cold start).
	slog.Info("embedding passages", "count", len(passageTexts))
	passageVecs, err := embedWithRetry(ctx, passageEmbedFn, passageTexts)
	if err != nil {
		slog.Error("failed to embed passages", "error", err)
		os.Exit(1)
	}
	if len(passageVecs) != len(pairs) {
		slog.Error("passage embedding count mismatch", "expected", len(pairs), "got", len(passageVecs))
		os.Exit(1)
	}

	// Upsert all passages into the temporary collection.
	// Use sequential uint64 IDs (0, 1, 2, ...) — simple and sufficient for benchmark.
	slog.Info("upserting passages", "count", len(passageVecs))
	if err := upsertPoints(ctx, qdrantClient, collectionName, passageVecs); err != nil {
		slog.Error("failed to upsert passages", "error", err)
		os.Exit(1)
	}

	// Evaluate Recall@K and MRR@K for each QA pair.
	hits := 0
	reciprocalRankSum := 0.0

	slog.Info("evaluating queries", "count", len(pairs), "k", *k)

	for i, pair := range pairs {
		queryVec, err := queryEmbedFn(ctx, pair.Query)
		if err != nil {
			slog.Error("failed to embed query", "index", i, "query", pair.Query, "error", err)
			os.Exit(1)
		}

		results, err := qdrantClient.Query(ctx, &qdrant.QueryPoints{
			CollectionName: collectionName,
			Query:          qdrant.NewQueryDense(queryVec),
			Limit:          k,
		})
		if err != nil {
			slog.Error("search failed", "index", i, "error", err)
			os.Exit(1)
		}

		// Check if the correct passage (index i) appears in the top-K results.
		hit := false
		for rank, sp := range results {
			if sp.Id.GetNum() == uint64(i) {
				hit = true
				reciprocalRankSum += 1.0 / float64(rank+1)
				break
			}
		}
		if hit {
			hits++
		}
	}

	// Compute metrics.
	total := len(pairs)
	recallAtK := float64(hits) / float64(total)
	mrrAtK := reciprocalRankSum / float64(total)

	// Determine pass/fail.
	resultLabel := "PASS"
	if recallAtK < *threshold {
		resultLabel = "FAIL"
	}

	// Print results.
	fmt.Println()
	fmt.Printf("=== Recall@%d Benchmark ===\n", *k)
	fmt.Printf("Model:     multilingual-e5-base (768d)\n")
	fmt.Printf("Prefixes:  %s\n", prefixLabel)
	fmt.Printf("Pairs:     %d\n", total)
	fmt.Printf("Hits:      %d/%d\n", hits, total)
	fmt.Printf("Recall@%d: %.4f\n", *k, recallAtK)
	fmt.Printf("MRR@%d:    %.4f\n", *k, mrrAtK)
	fmt.Printf("Threshold: %.2f\n", *threshold)
	fmt.Printf("Result:    %s\n", resultLabel)
	fmt.Println()

	if resultLabel == "FAIL" {
		os.Exit(1)
	}
}

// createBenchCollection creates a minimal temporary Qdrant collection for the
// benchmark. No payload indexes are created — they are not needed for benchmark.
func createBenchCollection(ctx context.Context, client *qdrant.Client, name string) error {
	return client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     vectorDimension,
			Distance: qdrant.Distance_Cosine,
		}),
	})
}

// upsertPoints upserts embedding vectors as PointStructs into the given
// collection, using sequential uint64 IDs (0, 1, 2, ...).
func upsertPoints(ctx context.Context, client *qdrant.Client, collectionName string, vectors [][]float32) error {
	points := make([]*qdrant.PointStruct, len(vectors))
	for i, vec := range vectors {
		points[i] = &qdrant.PointStruct{
			Id:      qdrant.NewIDNum(uint64(i)),
			Vectors: qdrant.NewVectors(vec...),
		}
	}

	wait := true
	_, err := client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collectionName,
		Wait:           &wait,
		Points:         points,
	})
	return err
}

// embedDirect calls the TEI /embed endpoint without going through the Embedder
// interface. Used internally for prefix and no-prefix embedding paths.
func embedDirect(ctx context.Context, baseURL string, inputs []string) ([][]float32, error) {
	httpClient := &http.Client{Timeout: 60 * time.Second}

	body, err := json.Marshal(map[string]any{"inputs": inputs})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("unexpected status %d (could not read body: %v)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(errBody))
	}

	var result [][]float32
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}
