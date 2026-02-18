---
phase: 02-core-issue-search
plan: 02
subsystem: indexer
tags: [qdrant, embedder, pipeline, config, chunking, upsert, delete-by-filter]

# Dependency graph
requires:
  - phase: 02-01
    provides: "Redmine REST client (redmine.Issue struct), text.StripMarkup, text.ChunkText, embedder.Embedder interface, qdrant.Client, qdrant.AliasName"
  - phase: 01-03
    provides: "EnsureCollection with payload indexes, qdrant package with PointIDNamespace and PointID"
provides:
  - "internal/indexer/pipeline.go: Pipeline.IndexIssues (full strip→chunk→embed→upsert pipeline)"
  - "internal/indexer/pipeline.go: Pipeline.DeleteIssueChunks (filter-based chunk deletion)"
  - "internal/qdrant/pointid.go: ChunkPointID (deterministic chunk-level UUID v5)"
  - "internal/config/config.go: SyncInterval, SyncBatchSize, ReconcileSchedule, ListenAddr, PermissionCacheTTL fields"
affects: [02-03, 02-04, 02-05]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Chunk-level point IDs via UUID v5 key format issue:<id>:chunk:<N>"
    - "Delete-before-upsert pattern for re-indexing (prevents stale chunk orphans)"
    - "Upsert batching at 100 points per call with Wait=true for durability"
    - "author_id stored as integer payload for identity-safe post-filtering (not display name)"

key-files:
  created:
    - internal/indexer/pipeline.go
  modified:
    - internal/qdrant/pointid.go
    - internal/config/config.go
    - config.example.yml
    - .env.example

key-decisions:
  - "DeleteIssueChunks called before every IndexIssues upsert using NewPointsSelectorFilter — avoids stale chunk orphans when re-indexing changes chunk count"
  - "author_id (int) stored alongside author (string) in payload so post-filtering of private issues uses numeric user ID, not display name"
  - "ChunkPointID placed in internal/qdrant/pointid.go (not pipeline.go) to keep deterministic ID logic in one canonical location"
  - "Upsert batch size 100 (not per-chunk) to bound memory while keeping Qdrant round-trips low"

patterns-established:
  - "Pipeline struct with injected embedder + qdrant.Client + slog.Logger for testability"
  - "chunkEntry intermediate struct correlates embedding results back to source issue/chunk metadata"
  - "truncate() helper works on runes (not bytes) for correct DE/EN UTF-8 handling"

requirements-completed: [IDX-01]

# Metrics
duration: 2min
completed: 2026-02-18
---

# Phase 02 Plan 02: Indexer Pipeline + Config Extension Summary

**Issue-to-Qdrant pipeline using strip+chunk+embed+upsert with filter-based chunk cleanup and full payload (including author_id for identity post-filtering), plus sync/server config fields**

## Performance

- **Duration:** 2 min
- **Started:** 2026-02-18T16:53:56Z
- **Completed:** 2026-02-18T16:55:56Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Implemented `internal/indexer/pipeline.go` with the complete issue indexing pipeline: strip markup, chunk text, batch embed via TEI, upsert to Qdrant with all 14 payload fields and deterministic chunk UUIDs
- Added `DeleteIssueChunks` which uses `NewPointsSelectorFilter` to delete all existing chunks by `(content_type=issue AND redmine_id=N)` filter before re-indexing — prevents stale chunk orphans
- Added `ChunkPointID` to `internal/qdrant/pointid.go` for chunk-level deterministic UUID v5 IDs using key `issue:<id>:chunk:<index>`
- Extended `Config` struct with 5 new fields (SyncInterval, SyncBatchSize, ReconcileSchedule, ListenAddr, PermissionCacheTTL) with sensible defaults wired via `viper.SetDefault`

## Task Commits

Each task was committed atomically:

1. **Task 1: Indexer pipeline — strip, chunk, embed, and upsert issues to Qdrant** - `55b7156` (feat)
2. **Task 2: Extend config with sync, server, and reconciliation fields** - `a7728a7` (feat)

**Plan metadata:** (docs commit below)

## Files Created/Modified
- `internal/indexer/pipeline.go` — Pipeline struct with IndexIssues, DeleteIssueChunks, buildFullText, truncate helpers
- `internal/qdrant/pointid.go` — Added ChunkPointID function for deterministic chunk-level UUIDs
- `internal/config/config.go` — Extended Config struct and Load() with 5 new fields + viper defaults
- `config.example.yml` — Documented new sync/server fields with comments, env var names, defaults
- `.env.example` — Added SYNC_INTERVAL, SYNC_BATCH_SIZE, RECONCILE_SCHEDULE, LISTEN_ADDR, PERMISSION_CACHE_TTL

## Decisions Made
- `NewPointsSelectorFilter` is available in go-client v1.16.2 — used directly instead of constructing `PointsSelector_Filter` protobuf manually
- `author_id` (integer) stored in payload alongside `author` (display name string) so permission post-filtering of private issues uses numeric ID for exact identity comparison, not a potentially non-unique display name
- `ChunkPointID` placed in `internal/qdrant/pointid.go` (same file as `PointID`) to keep all deterministic ID logic co-located; no circular import since indexer already imports qdrant package

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None — `NewPointsSelectorFilter` was directly available (the open question from RESEARCH.md was resolved in favor of the simpler direct API call).

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Pipeline ready for 02-03 (scheduler): `Pipeline.IndexIssues` and `Pipeline.DeleteIssueChunks` are the primary entry points
- Config fields for 02-03 (SyncInterval, SyncBatchSize, ReconcileSchedule) and 02-05 (ListenAddr, PermissionCacheTTL) are defined and have defaults
- All Qdrant payload fields required by 02-04 (search handler) are populated: redmine_id, subject, tracker, status, project_id, author, is_private, text_preview, chunk_index

---
*Phase: 02-core-issue-search*
*Completed: 2026-02-18*
