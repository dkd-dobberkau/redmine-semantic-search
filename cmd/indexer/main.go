// Package main is the indexer process entrypoint.
//
// The indexer wires all components together and starts the background
// incremental sync and deletion reconciliation jobs. It serves immediately
// without blocking on a full sync — the first incremental poll begins in the
// background after startup.
//
// Shutdown is graceful: on SIGINT or SIGTERM the syncer and reconciler are
// stopped cleanly, allowing any in-flight operations to complete.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oliverpool/redmine-semantic-search/internal/config"
	"github.com/oliverpool/redmine-semantic-search/internal/embedder"
	"github.com/oliverpool/redmine-semantic-search/internal/indexer"
	qdrantpkg "github.com/oliverpool/redmine-semantic-search/internal/qdrant"
	"github.com/oliverpool/redmine-semantic-search/internal/redmine"
	"github.com/qdrant/go-client/qdrant"
)

func main() {
	// Step 1: Load configuration.
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Step 2: Set up structured JSON logger.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Step 3: Connect to Qdrant.
	qdrantClient, err := qdrant.NewClient(&qdrant.Config{
		Host: cfg.QdrantHost,
		Port: cfg.QdrantPort,
	})
	if err != nil {
		logger.Error("failed to connect to Qdrant", "error", err)
		os.Exit(1)
	}
	defer qdrantClient.Close()

	// Step 4: Ensure the collection and alias exist with all payload indexes.
	ctx := context.Background()
	if err := qdrantpkg.EnsureCollection(ctx, qdrantClient); err != nil {
		logger.Error("failed to ensure Qdrant collection", "error", err)
		os.Exit(1)
	}

	// Step 5: Create the TEI embedder.
	teiEmbedder := embedder.NewTEIEmbedder(cfg.EmbeddingURL)

	// Step 6: Create the Redmine client.
	redmineClient := redmine.NewClient(cfg.RedmineURL, cfg.RedmineAPIKey)

	// Step 7: Create the indexing pipeline.
	pipeline := indexer.NewPipeline(teiEmbedder, qdrantClient, logger)

	// Step 8: Create the incremental syncer.
	syncer := indexer.NewSyncer(
		redmineClient,
		pipeline,
		time.Duration(cfg.SyncInterval)*time.Minute,
		cfg.SyncBatchSize,
		logger,
	)

	// Step 9: Create the deletion reconciler.
	reconciler, err := indexer.NewReconciler(redmineClient, qdrantClient, cfg.ReconcileSchedule, logger)
	if err != nil {
		logger.Error("failed to create reconciler", "error", err)
		os.Exit(1)
	}

	// Step 10: Start both background jobs.
	// signal.NotifyContext cancels the context on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	syncer.Start(ctx)
	reconciler.Start()

	logger.Info("indexer started",
		"sync_interval_minutes", cfg.SyncInterval,
		"reconcile_schedule", cfg.ReconcileSchedule,
	)

	// Step 11: Wait for shutdown signal.
	<-ctx.Done()

	logger.Info("shutting down indexer")

	// Step 12: Stop background jobs and wait for in-flight operations to finish.
	syncer.Stop()
	<-reconciler.Stop().Done()

	logger.Info("shutdown complete")
}
