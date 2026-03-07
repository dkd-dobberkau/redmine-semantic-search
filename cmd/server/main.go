// Command server starts the Redmine semantic search HTTP API server.
// It serves the search endpoint (requires X-Redmine-API-Key authentication)
// and a public health endpoint for monitoring and Docker health checks.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/qdrant/go-client/qdrant"

	"github.com/oliverpool/redmine-semantic-search/internal/auth"
	"github.com/oliverpool/redmine-semantic-search/internal/config"
	"github.com/oliverpool/redmine-semantic-search/internal/embedder"
	"github.com/oliverpool/redmine-semantic-search/internal/redmine"
	"github.com/oliverpool/redmine-semantic-search/internal/search"
)

const shutdownTimeout = 15 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger.Info("configuration loaded",
		"redmine_url", cfg.RedmineURL,
		"qdrant_host", cfg.QdrantHost,
		"qdrant_port", cfg.QdrantPort,
		"embedding_url", cfg.EmbeddingURL,
		"listen_addr", cfg.ListenAddr,
	)

	// Connect to Qdrant via gRPC.
	qdrantClient, err := qdrant.NewClient(&qdrant.Config{
		Host: cfg.QdrantHost,
		Port: cfg.QdrantPort,
	})
	if err != nil {
		logger.Error("failed to create Qdrant client", "error", err)
		os.Exit(1)
	}
	defer qdrantClient.Close()

	// Create the embedder based on configured provider.
	var emb embedder.Embedder
	switch cfg.EmbeddingProvider {
	case "tei":
		emb = embedder.NewTEIEmbedder(cfg.EmbeddingURL)
	case "ollama":
		emb = embedder.NewOllamaEmbedder(cfg.EmbeddingURL, cfg.EmbeddingModel)
	default:
		logger.Error("unknown embedding provider", "provider", cfg.EmbeddingProvider)
		os.Exit(1)
	}

	// Create Redmine client for user validation and project lookup.
	redmineClient := redmine.NewClient(cfg.RedmineURL, cfg.RedmineAPIKey)

	// Create permission cache: TTL-backed, singleflight-coalesced.
	cacheTTL := time.Duration(cfg.PermissionCacheTTL) * time.Minute
	permCache := auth.NewPermissionCache(redmineClient, cacheTTL, logger)

	// Create auth middleware: enforces X-Redmine-API-Key on protected routes.
	authMiddleware := auth.NewAuthMiddleware(permCache, logger)

	// Create handlers.
	searchHandler := search.NewSearchHandler(emb, qdrantClient, logger)
	healthHandler := search.NewHealthHandler(qdrantClient, cfg.EmbeddingURL, cfg.EmbeddingProvider, logger)

	// Register routes using Go 1.22 method+path pattern syntax.
	// Search requires auth; health is intentionally public for monitoring.
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/search", authMiddleware.Wrap(searchHandler))
	mux.HandleFunc("GET /api/v1/health", healthHandler.ServeHTTP)
	similarHandler := search.NewSimilarHandler(qdrantClient, logger)
	mux.Handle("GET /api/v1/similar", authMiddleware.Wrap(similarHandler))
	mux.HandleFunc("GET /api/v1/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		info, err := qdrantClient.GetCollectionInfo(r.Context(), "redmine_search_v1")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{"indexed_issues": 0})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"indexed_issues": info.GetPointsCount()})
	})
	mux.HandleFunc("GET /api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"redmine_url": cfg.RedmineURL,
			"api_key":     cfg.RedmineAPIKey,
		})
	})
	mux.Handle("GET /", http.FileServer(http.Dir("web")))

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      cors(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start the server in a goroutine so we can wait for a shutdown signal.
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	logger.Info("search server started", "addr", cfg.ListenAddr)

	// Block until we receive SIGINT or SIGTERM.
	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-sigCtx.Done()

	logger.Info("shutdown signal received, draining connections")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	} else {
		logger.Info("server shutdown complete")
	}
}

// cors wraps an http.Handler with permissive CORS headers for local development.
func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "X-Redmine-API-Key, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}
