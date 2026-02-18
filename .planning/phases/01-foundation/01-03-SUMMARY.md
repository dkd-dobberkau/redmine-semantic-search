---
phase: 01-foundation
plan: 03
subsystem: infra
tags: [go, qdrant, embedder, tei, uuid-v5, grpc, http-client, interface]

# Dependency graph
requires:
  - phase: 01-01
    provides: Go module with qdrant/go-client and google/uuid dependencies, internal package structure
provides:
  - internal/embedder/embedder.go — Embedder interface with EmbedPassages and EmbedQuery methods
  - internal/embedder/tei.go — TEI HTTP implementation with e5 prefix encapsulation
  - internal/qdrant/collection.go — EnsureCollection with 7 payload indexes and alias (idempotent)
  - internal/qdrant/pointid.go — Deterministic UUID v5 PointID from content_type:redmine_id
affects: [01-04, 02-01, 02-02, 02-03, all subsequent plans using Embedder or Qdrant]

# Tech tracking
tech-stack:
  added:
    - github.com/qdrant/go-client v1.16.2 (promoted from indirect to used)
    - github.com/google/uuid v1.6.0 (promoted from indirect to used)
    - google.golang.org/grpc v1.76.0 (transitive, added to go.sum)
    - google.golang.org/protobuf (transitive, added to go.sum)
  patterns:
    - Two-method Embedder interface (EmbedPassages/EmbedQuery) prevents wrong-prefix-mode bugs at compile time
    - var _ Embedder = (*TEIEmbedder)(nil) compile-time interface check
    - EnsureCollection idempotency via CollectionExists guard before CreateCollection
    - FieldType pointer via idx.fieldType capture (required by qdrant proto: FieldType is *FieldType in struct)
    - UUID v5 deterministic point IDs via uuid.NewSHA1 with fixed application-specific namespace

key-files:
  created:
    - internal/embedder/embedder.go
    - internal/embedder/tei.go
    - internal/qdrant/collection.go
    - internal/qdrant/pointid.go
  modified:
    - go.mod (qdrant/go-client and uuid promoted; grpc transitive added)
    - go.sum (transitive gRPC and protobuf dependencies added)

key-decisions:
  - "Embedder interface uses two methods (EmbedPassages/EmbedQuery) not a mode enum — makes wrong-prefix usage impossible at compile time"
  - "FieldType in CreateFieldIndexCollection is *FieldType (pointer), not FieldType — must use local variable capture for pointer"
  - "qdrant.FieldType enum names are FieldType_FieldTypeKeyword/Integer/Datetime (not the shorter forms in research doc)"
  - "EnsureCollection skips creation but still calls ensureAlias when collection exists — alias is always guaranteed"
  - "go get github.com/qdrant/go-client/qdrant@v1.16.2 required to pull all transitive gRPC dependencies into go.sum"

patterns-established:
  - "Pattern: Embedder interface hides all model-specific prefix handling — callers never touch 'passage:' or 'query:' strings"
  - "Pattern: Idempotent init with CollectionExists guard — safe to call on every startup, no error if collection already exists"
  - "Pattern: All 7 payload indexes created at collection init with Wait=true — never add indexes to a populated collection"
  - "Pattern: UUID v5 via uuid.NewSHA1(namespace, []byte(key)) for deterministic point IDs — same input always same UUID"

requirements-completed: [INFRA-01, INFRA-02]

# Metrics
duration: 2min
completed: 2026-02-18
---

# Phase 1 Plan 03: Embedder Interface, TEI Client, Qdrant Collection Init, and Point IDs Summary

**Embedder interface with TEI HTTP client (e5 prefix encapsulation), idempotent Qdrant collection init with all 7 payload indexes and alias, and deterministic UUID v5 point ID generation via google/uuid**

## Performance

- **Duration:** 2 min
- **Started:** 2026-02-18T13:40:42Z
- **Completed:** 2026-02-18T13:43:11Z
- **Tasks:** 2
- **Files modified:** 4 created, 2 modified (go.mod, go.sum)

## Accomplishments

- Implemented Embedder interface with two methods that prevent wrong-prefix-mode bugs at compile time
- TEIEmbedder encapsulates e5 prefix handling — callers pass raw text and never deal with "passage: " / "query: " strings
- EnsureCollection creates collection with 768d Cosine vectors, on-disk storage, all 7 payload indexes with Wait=true, and alias "redmine_search" -> "redmine_search_v1"; idempotent on repeated calls
- PointID produces deterministic UUID v5 strings from content_type:redmine_id — same inputs always yield the same UUID, enabling upsert idempotency

## Task Commits

Each task was committed atomically:

1. **Task 1: Embedder interface and TEI HTTP client** - `ef3f574` (feat)
2. **Task 2: Qdrant collection init, payload indexes, alias, and point ID** - `cb7eebb` (feat)

**Plan metadata:** (docs commit follows)

## Files Created/Modified

- `internal/embedder/embedder.go` - Embedder interface with EmbedPassages and EmbedQuery method signatures; package-level doc explaining encapsulation contract
- `internal/embedder/tei.go` - TEIEmbedder: NewTEIEmbedder constructor, EmbedPassages/EmbedQuery with e5 prefix handling, internal embed() with error distinction for network/status/decode failures; compile-time interface check
- `internal/qdrant/collection.go` - EnsureCollection (idempotent), createCollection (768d Cosine OnDisk), createPayloadIndexes (7 indexes Wait=true), ensureAlias; CollectionName/AliasName/VectorDimension constants
- `internal/qdrant/pointid.go` - PointIDNamespace fixed UUID, PointID(contentType, redmineID) via uuid.NewSHA1
- `go.mod` - qdrant/go-client and google/uuid moved from indirect to used; gRPC dependencies added
- `go.sum` - gRPC and protobuf transitive dependencies added

## Decisions Made

- Two-method Embedder interface (EmbedPassages/EmbedQuery) instead of single Embed with mode parameter — makes it impossible to accidentally use the wrong prefix mode; interface contract is self-documenting
- `FieldType` in `CreateFieldIndexCollection` is `*FieldType` (pointer), not a value type — used local variable capture (`fieldType := idx.fieldType`) to take address correctly
- Correct qdrant enum names discovered from source: `FieldType_FieldTypeKeyword`, `FieldType_FieldTypeInteger`, `FieldType_FieldTypeDatetime` (not the shorter forms shown in research doc)
- `EnsureCollection` always calls `ensureAlias` even when collection exists — guarantees alias is set regardless of whether collection was just created or already existed
- `go get github.com/qdrant/go-client/qdrant@v1.16.2` (with subpackage path) was required to pull all transitive gRPC/protobuf dependencies into go.sum; `go get github.com/qdrant/go-client@v1.16.2` alone was insufficient

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added transitive gRPC dependencies to go.sum**
- **Found during:** Task 2 (Qdrant collection init)
- **Issue:** `go build ./internal/qdrant/...` failed with "missing go.sum entry" for google.golang.org/grpc and google.golang.org/protobuf. The initial `go get github.com/qdrant/go-client@v1.16.2` added the module to go.mod but did not resolve all transitive deps into go.sum.
- **Fix:** Ran `go get github.com/qdrant/go-client/qdrant@v1.16.2` (with the subpackage path) which triggered download of grpc v1.76.0, protobuf v1.36.10, and genproto.
- **Files modified:** `go.mod`, `go.sum`
- **Verification:** `go build ./internal/qdrant/...` succeeded; `go build ./...` and `go vet ./...` both pass
- **Committed in:** `cb7eebb` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 3 — blocking dependency)
**Impact on plan:** Required for compilation. No scope creep.

## Issues Encountered

- Qdrant go-client uses `*FieldType` (pointer to enum) in `CreateFieldIndexCollection.FieldType`, not the enum value directly. The research doc pattern `FieldType: idx.fieldType` was incorrect — needed `FieldType: &fieldType` with a local variable capture. Discovered by reading the protobuf-generated struct definition before writing code.

## User Setup Required

None - no external service configuration required for this plan. All code compiles and passes vet with no running services needed.

## Next Phase Readiness

- Embedder interface ready for bench/recall benchmark (Plan 01-04)
- EnsureCollection ready for integration with Qdrant at startup (Plans 02+)
- PointID utility ready for indexer pipeline (Phase 2)
- `go build ./...` and `go vet ./...` pass cleanly on the entire project
- No blockers for 01-04 (Recall benchmark) or 02-01 (Redmine client)

## Self-Check: PASSED

- FOUND: internal/embedder/embedder.go
- FOUND: internal/embedder/tei.go
- FOUND: internal/qdrant/collection.go
- FOUND: internal/qdrant/pointid.go
- FOUND commit ef3f574: feat(01-03): implement Embedder interface and TEI HTTP client
- FOUND commit cb7eebb: feat(01-03): implement Qdrant collection init, payload indexes, alias, and point ID

---
*Phase: 01-foundation*
*Completed: 2026-02-18*
