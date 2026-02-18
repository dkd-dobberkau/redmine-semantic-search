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
