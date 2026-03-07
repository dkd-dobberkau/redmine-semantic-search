// Package qdrant provides Qdrant-specific utilities for the redmine-semantic-search
// indexer, including collection initialization and deterministic point ID generation.
package qdrant

import (
	"fmt"

	"github.com/google/uuid"
)

// PointIDNamespace is a fixed, application-specific UUID v4 used as the
// namespace for all UUID v5 point ID generation. It must never change —
// changing it would invalidate all existing point IDs stored in Qdrant.
//
// This is NOT uuid.NameSpaceDNS or any other shared namespace; it is unique
// to this application to prevent UUID collisions with other systems.
var PointIDNamespace = uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

// PointID returns a deterministic UUID v5 string for a given content type and
// Redmine object ID. The same inputs always produce the same UUID, enabling
// idempotent upsert operations — re-indexing the same object overwrites its
// existing point in Qdrant rather than creating a duplicate.
//
// Example:
//
//	PointID("issue", 123)   // always returns the same UUID
//	PointID("wiki", 42)     // different content_type yields a different UUID
func PointID(contentType string, redmineID int) string {
	key := fmt.Sprintf("%s:%d", contentType, redmineID)
	return uuid.NewSHA1(PointIDNamespace, []byte(key)).String()
}

// ChunkPointID returns a deterministic UUID v5 string for a specific chunk of a
// Redmine issue. Each (redmineID, chunkIndex) pair maps to a unique, stable UUID,
// enabling idempotent chunk upserts. Different chunks of the same issue produce
// distinct UUIDs, preventing collisions in Qdrant.
//
// The key format "issue:<id>:chunk:<index>" ensures no overlap with PointID keys.
//
// Example:
//
//	ChunkPointID(123, 0)  // first chunk of issue 123
//	ChunkPointID(123, 1)  // second chunk of issue 123 — different UUID
func ChunkPointID(redmineID, chunkIndex int) string {
	key := fmt.Sprintf("issue:%d:chunk:%d", redmineID, chunkIndex)
	return uuid.NewSHA1(PointIDNamespace, []byte(key)).String()
}

// JournalChunkPointID returns a deterministic UUID v5 string for a specific chunk
// of a Redmine journal entry. The key format "journal:<id>:chunk:<index>" ensures
// no overlap with issue chunk keys.
func JournalChunkPointID(journalID, chunkIndex int) string {
	key := fmt.Sprintf("journal:%d:chunk:%d", journalID, chunkIndex)
	return uuid.NewSHA1(PointIDNamespace, []byte(key)).String()
}
