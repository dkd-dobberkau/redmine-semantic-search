// Command server starts the Redmine semantic search HTTP API server.
// It serves the search endpoint (requires X-Redmine-API-Key authentication)
// and a public health endpoint for monitoring and Docker health checks.
package main

import (
	"context"
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

	// Create TEI embedder for vectorizing search queries.
	teiEmbedder := embedder.NewTEIEmbedder(cfg.EmbeddingURL)

	// Create Redmine client for user validation and project lookup.
	redmineClient := redmine.NewClient(cfg.RedmineURL, cfg.RedmineAPIKey)

	// Create permission cache: TTL-backed, singleflight-coalesced.
	cacheTTL := time.Duration(cfg.PermissionCacheTTL) * time.Minute
	permCache := auth.NewPermissionCache(redmineClient, cacheTTL, logger)

	// Create auth middleware: enforces X-Redmine-API-Key on protected routes.
	authMiddleware := auth.NewAuthMiddleware(permCache, logger)

	// Create handlers.
	searchHandler := search.NewSearchHandler(teiEmbedder, qdrantClient, logger)
	healthHandler := search.NewHealthHandler(qdrantClient, cfg.EmbeddingURL, logger)

	// Register routes using Go 1.22 method+path pattern syntax.
	// Search requires auth; health is intentionally public for monitoring.
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/search", authMiddleware.Wrap(searchHandler))
	mux.HandleFunc("GET /api/v1/health", healthHandler.ServeHTTP)

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
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
