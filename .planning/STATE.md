# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-18)

**Core value:** Nutzer finden relevante Redmine-Inhalte uber semantische Suche, auch wenn sie die exakte Formulierung nicht kennen — ohne das Berechtigungsmodell zu umgehen.
**Current focus:** Phase 2 — Core Issue Search

## Current Position

Phase: 1 of 5 (Foundation)
Plan: 4 of 4 in current phase — PHASE COMPLETE
Status: Complete
Last activity: 2026-02-18 — Plan 01-04 completed (Recall@10 benchmark, multilingual-e5-base confirmed, Phase 1 foundation complete)

Progress: [█████░░░░░] 20%

## Performance Metrics

**Velocity:**
- Total plans completed: 4
- Average duration: 6 min
- Total execution time: 0.37 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01-foundation | 4/4 | 22 min | 5.5 min |

**Recent Trend:**
- Last 5 plans: 4 min, 3 min, 2 min, 13 min
- Trend: stable (13 min was benchmark execution including Docker pull + disk cleanup)

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

### Pending Todos

None yet.

### Blockers/Concerns

- [Phase 4]: Hybrid search needs pre-plan research — verify Qdrant built-in BM25 sparse vector API shape (>=1.7) and TEI SPLADE model support before planning. Run `/gsd:research-phase` before planning Phase 4.
- [Phase 5 Tika, deferred to v2]: If document indexing is promoted to v1, Tika 3.x REST API and Docker image stability need verification.

## Session Continuity

Last session: 2026-02-18
Stopped at: Phase 2 planned (5 plans, 3 waves). Verification passed. Ready for execution.
Resume file: .planning/phases/02-core-issue-search/02-01-PLAN.md
