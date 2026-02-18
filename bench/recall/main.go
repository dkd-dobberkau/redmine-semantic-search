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
	"errors"
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

const (
	vectorDimension = 768

	// teiBatchSize is the maximum number of texts per TEI /embed request.
	// TEI defaults to --max-client-batch-size 32; sending more returns HTTP 422.
	teiBatchSize = 32
)

// teiClient wraps HTTP calls to the TEI /embed endpoint with batching support.
// Large input slices are automatically split into batches of teiBatchSize.
type teiClient struct {
	baseURL    string
	httpClient *http.Client
}

func newTEIClient(baseURL string) *teiClient {
	return &teiClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// embed sends inputs to TEI /embed, batching at teiBatchSize automatically.
// Returns one float32 vector per input in the same order.
func (t *teiClient) embed(ctx context.Context, inputs []string) ([][]float32, error) {
	var all [][]float32
	for start := 0; start < len(inputs); start += teiBatchSize {
		end := start + teiBatchSize
		if end > len(inputs) {
			end = len(inputs)
		}
		batch := inputs[start:end]

		vecs, err := t.embedBatch(ctx, batch)
		if err != nil {
			return nil, err
		}
		all = append(all, vecs...)
	}
	return all, nil
}

// embedBatch sends a single batch (must be <= teiBatchSize) to TEI /embed.
// Returns a permanent error on 4xx responses (not retried) and a transient
// error on connection/5xx failures (eligible for retry).
func (t *teiClient) embedBatch(ctx context.Context, inputs []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{"inputs": inputs})
	if err != nil {
		return nil, backoff.Permanent(fmt.Errorf("marshal request: %w", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, backoff.Permanent(fmt.Errorf("build request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		// Network error — transient, eligible for retry.
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			errBody = []byte("(could not read body)")
		}
		msg := fmt.Sprintf("TEI status %d: %s", resp.StatusCode, string(errBody))

		// 4xx errors are permanent (bad request, auth failure, etc.).
		// 5xx errors are transient (service unavailable, cold start).
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return nil, backoff.Permanent(errors.New(msg))
		}
		return nil, errors.New(msg) // transient: will be retried
	}

	var result [][]float32
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, backoff.Permanent(fmt.Errorf("decode response: %w", err))
	}
	return result, nil
}

// embedWithColdStartRetry embeds inputs with exponential backoff for up to
// 2 minutes to handle TEI cold start (model loading delay after container
// startup). Only transient errors (connection refused, 5xx) are retried;
// 4xx validation errors are treated as permanent failures.
func (t *teiClient) embedWithColdStartRetry(ctx context.Context, inputs []string) ([][]float32, error) {
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 2 * time.Minute
	bo.InitialInterval = 2 * time.Second
	bo.MaxInterval = 15 * time.Second

	var result [][]float32
	err := backoff.RetryNotify(
		func() error {
			var err error
			result, err = t.embed(ctx, inputs)
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

// applyPassagePrefix prepends "passage: " to each text for e5 models.
func applyPassagePrefix(texts []string) []string {
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i] = "passage: " + t
	}
	return out
}

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
	tei := newTEIClient(*embeddingURL)

	// Prepare passage texts, applying e5 prefix if not in no-prefix mode.
	passageTexts := make([]string, len(pairs))
	for i, p := range pairs {
		passageTexts[i] = p.Passage
	}
	if !*noPrefix {
		passageTexts = applyPassagePrefix(passageTexts)
	}

	// Embed all passages with TEI cold-start retry on the first call.
	slog.Info("embedding passages", "count", len(passageTexts))
	passageVecs, err := tei.embedWithColdStartRetry(ctx, passageTexts)
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
		// Prepare query input with or without e5 prefix.
		queryInput := pair.Query
		if !*noPrefix {
			queryInput = "query: " + pair.Query
		}

		queryVecs, err := tei.embed(ctx, []string{queryInput})
		if err != nil {
			slog.Error("failed to embed query", "index", i, "query", pair.Query, "error", err)
			os.Exit(1)
		}
		if len(queryVecs) == 0 {
			slog.Error("empty embedding response for query", "index", i, "query", pair.Query)
			os.Exit(1)
		}
		queryVec := queryVecs[0]

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
