# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-18)

**Core value:** Nutzer finden relevante Redmine-Inhalte uber semantische Suche, auch wenn sie die exakte Formulierung nicht kennen — ohne das Berechtigungsmodell zu umgehen.
**Current focus:** Phase 2 — Core Issue Search

## Current Position

Phase: 2 of 5 (Core Issue Search)
Plan: 4 of 5 in current phase — COMPLETE
Status: In Progress
Last activity: 2026-02-18 — Plan 02-04 completed (auth middleware, permission cache with singleflight)

Progress: [███████░░░] 28%

## Performance Metrics

**Velocity:**
- Total plans completed: 6
- Average duration: 4 min
- Total execution time: 0.46 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01-foundation | 4/4 | 22 min | 5.5 min |
| 02-core-issue-search | 2/5 | 5 min | 2.5 min |

**Recent Trend:**
- Last 5 plans: 3 min, 2 min, 13 min, 3 min, 2 min
- Trend: stable

*Updated after each plan completion*

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- [Pre-Phase 1]: Embedding model selection is a schema-level decision — run Recall@10 benchmark on real DE/EN Redmine content in Phase 1 before committing vector dimensionality (multilingual-e5-base 768d recommended)
- [Pre-Phase 1]: gRPC connection to Qdrant must be a single shared ClientConn created at startup — not one per goroutine
- [Pre-Phase 1]: All Qdrant payload indexes (project_id, content_type, tracker, status, author_id, created_on) must be created at collection init, before the first upsert
- [01-01]: .gitignore binary names must be anchored with leading slash (/indexer not indexer) to avoid matching source directories
- [01-01]: Viper v1.21 uses go-viper/mapstructure/v2 internally; mapstructure struct tags still work identically — no migration needed
- [01-01]: Config file is optional; validator catches missing fields whether they come from YAML or env vars
- [01-02]: Dockerfile builder stage uses golang:1.25-alpine (not 1.23) — go.mod requires go 1.25.0; golang:1.23 rejects with GOTOOLCHAIN=local
- [01-02]: Qdrant health check uses bash /dev/tcp (no curl in Qdrant image, GitHub issue #4250)
- [01-02]: HF_HUB_CACHE=/data added to TEI service to cache model in ./models volume mount
- [01-03]: Embedder interface uses two methods (EmbedPassages/EmbedQuery) not a mode enum — prevents wrong-prefix usage at compile time
- [01-03]: qdrant.FieldType enum names are FieldType_FieldTypeKeyword/Integer/Datetime (not shorter forms in research doc)
- [01-03]: CreateFieldIndexCollection.FieldType is *FieldType (pointer) — must capture in local var before taking address
- [01-03]: go get github.com/qdrant/go-client/qdrant@v1.16.2 (subpackage path) required to pull all gRPC/protobuf transitive deps into go.sum
- [Phase 01-foundation]: TEI max-client-batch-size is 32 by default; embedding functions must chunk batches at most 32 texts
- [Phase 01-foundation]: backoff.Permanent wraps 4xx errors to prevent retry on validation errors; only network/5xx retried for TEI cold start
- [Phase 01-foundation]: platform: linux/amd64 in docker-compose.yml required on Apple Silicon (TEI and Qdrant publish amd64-only images)
- [Phase 01-foundation]: multilingual-e5-base confirmed as the model for 768d vector schema — Phase 2 proceeds
- [02-01]: Dual apiKey parameter in doJSON allows admin and user keys to share one implementation without wrapper structs
- [02-01]: ChunkSize=1600/ChunkOverlap=200 chars per research discretion (~400/~50 tokens for multilingual-e5-base)
- [02-01]: url.Values.Set encodes ">=" automatically — no manual percent-encoding needed for updated_on cursor
- [02-01]: status_id=* always passed to FetchIssuesSince/FetchAllIssueIDs to include closed issues
- [02-02]: DeleteIssueChunks called before every IndexIssues upsert using NewPointsSelectorFilter — avoids stale chunk orphans when re-indexing changes chunk count
- [02-02]: author_id (int) stored alongside author (string) in payload so post-filtering of private issues uses numeric user ID, not display name
- [02-02]: ChunkPointID placed in internal/qdrant/pointid.go (not pipeline.go) to keep deterministic ID logic in one canonical location
- [02-02]: NewPointsSelectorFilter available directly in go-client v1.16.2 — no manual protobuf construction needed
- [Phase 02-core-issue-search]: ProjectIDs is []int64 (not []int) in UserPermissions for direct use in Qdrant NewMatchInt filter without conversion
- [Phase 02-core-issue-search]: errors.Is used for ErrUnauthorized check in auth middleware — future-proofs against wrapped error variants
- [Phase 02-core-issue-search]: singleflight.Group per PermissionCache instance (not global) — isolates cache stampede prevention per cache and simplifies testing

### Pending Todos

None yet.

### Blockers/Concerns

- [Phase 4]: Hybrid search needs pre-plan research — verify Qdrant built-in BM25 sparse vector API shape (>=1.7) and TEI SPLADE model support before planning. Run `/gsd:research-phase` before planning Phase 4.
- [Phase 5 Tika, deferred to v2]: If document indexing is promoted to v1, Tika 3.x REST API and Docker image stability need verification.

## Session Continuity

Last session: 2026-02-18
Stopped at: Completed 02-02-PLAN.md (indexer pipeline + config extension)
Resume file: .planning/phases/02-core-issue-search/02-03-PLAN.md
