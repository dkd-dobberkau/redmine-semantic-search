package qdrant

import (
	"context"
	"fmt"

	"github.com/qdrant/go-client/qdrant"
)

const (
	// CollectionName is the actual Qdrant collection used for storage.
	// Applications should use AliasName instead, which supports blue-green reindexing.
	CollectionName = "redmine_search_v1"

	// AliasName is the stable alias that all application code must target.
	// It points to CollectionName, enabling zero-downtime reindexing by switching
	// the alias to a new collection version without changing callers.
	AliasName = "redmine_search"

	// VectorDimension is the output dimension of the multilingual-e5-base model.
	// This is a schema-level constant — changing it requires re-indexing all vectors.
	VectorDimension = 768
)

// EnsureCollection creates the Qdrant collection with all required payload indexes
// and the application alias if they do not already exist.
//
// This function is idempotent: calling it multiple times (e.g. on every startup)
// does not error and does not create duplicate indexes. When the collection already
// exists, it skips creation but still ensures the alias is in place.
//
// IMPORTANT: All payload indexes are created before any document is indexed.
// Adding indexes to a populated collection requires a full blocking re-scan.
// The index list in createPayloadIndexes is the authoritative source of truth
// for which filters are supported.
func EnsureCollection(ctx context.Context, client *qdrant.Client) error {
	exists, err := client.CollectionExists(ctx, CollectionName)
	if err != nil {
		return fmt.Errorf("check collection existence: %w", err)
	}

	if !exists {
		if err := createCollection(ctx, client); err != nil {
			return err
		}
		if err := createPayloadIndexes(ctx, client); err != nil {
			return err
		}
	}

	// Always ensure the alias is set — safe to call even if it already exists.
	if err := ensureAlias(ctx, client); err != nil {
		return err
	}

	return nil
}

// createCollection creates the collection with vector parameters and on-disk storage.
func createCollection(ctx context.Context, client *qdrant.Client) error {
	onDisk := true
	err := client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: CollectionName,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     VectorDimension,
			Distance: qdrant.Distance_Cosine,
			OnDisk:   &onDisk,
		}),
		OnDiskPayload: &onDisk,
	})
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	return nil
}

// createPayloadIndexes creates all 7 payload indexes required for filtered searches.
//
// Wait: true is set on each call to ensure the index is fully built before
// returning. Without this, filter queries may degrade briefly on startup.
//
// Index list:
//   - project_id   (Integer) — filter by Redmine project
//   - content_type (Keyword) — filter by issue/wiki/document/journal
//   - tracker      (Keyword) — filter by Redmine tracker (Bug, Feature, etc.)
//   - status       (Keyword) — filter by issue status
//   - author       (Keyword) — filter by author login or display name
//   - created_on   (Datetime) — filter/sort by creation date
//   - updated_on   (Datetime) — filter/sort by last update date (used for incremental indexing)
func createPayloadIndexes(ctx context.Context, client *qdrant.Client) error {
	type indexSpec struct {
		field     string
		fieldType qdrant.FieldType
	}

	indexes := []indexSpec{
		{"project_id", qdrant.FieldType_FieldTypeInteger},
		{"content_type", qdrant.FieldType_FieldTypeKeyword},
		{"tracker", qdrant.FieldType_FieldTypeKeyword},
		{"status", qdrant.FieldType_FieldTypeKeyword},
		{"author", qdrant.FieldType_FieldTypeKeyword},
		{"created_on", qdrant.FieldType_FieldTypeDatetime},
		{"updated_on", qdrant.FieldType_FieldTypeDatetime},
	}

	wait := true
	for _, idx := range indexes {
		fieldType := idx.fieldType // capture for pointer
		_, err := client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
			CollectionName: CollectionName,
			FieldName:      idx.field,
			FieldType:      &fieldType,
			Wait:           &wait,
		})
		if err != nil {
			return fmt.Errorf("create index %s: %w", idx.field, err)
		}
	}
	return nil
}

// ensureAlias creates the alias pointing from AliasName to CollectionName.
// Qdrant's CreateAlias is idempotent when the alias already points to the same
// collection — it does not error if the alias exists.
func ensureAlias(ctx context.Context, client *qdrant.Client) error {
	if err := client.CreateAlias(ctx, AliasName, CollectionName); err != nil {
		return fmt.Errorf("ensure alias %s -> %s: %w", AliasName, CollectionName, err)
	}
	return nil
}
