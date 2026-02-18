package main

import (
	"log/slog"
	"os"

	"github.com/oliverpool/redmine-semantic-search/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("configuration loaded",
		"redmine_url", cfg.RedmineURL,
		"redmine_api_key", "[REDACTED]",
		"qdrant_host", cfg.QdrantHost,
		"qdrant_port", cfg.QdrantPort,
		"embedding_url", cfg.EmbeddingURL,
	)
}
