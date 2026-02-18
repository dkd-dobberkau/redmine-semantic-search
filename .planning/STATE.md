# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-18)

**Core value:** Nutzer finden relevante Redmine-Inhalte uber semantische Suche, auch wenn sie die exakte Formulierung nicht kennen — ohne das Berechtigungsmodell zu umgehen.
**Current focus:** Phase 1 — Foundation

## Current Position

Phase: 1 of 5 (Foundation)
Plan: 3 of 4 in current phase
Status: In progress
Last activity: 2026-02-18 — Plan 01-03 completed (Embedder interface, TEI client, Qdrant collection init, point IDs)

Progress: [████░░░░░░] 15%

## Performance Metrics

**Velocity:**
- Total plans completed: 3
- Average duration: 3 min
- Total execution time: 0.15 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01-foundation | 3/4 | 9 min | 3 min |

**Recent Trend:**
- Last 5 plans: 4 min, 3 min, 2 min
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

### Pending Todos

None yet.

### Blockers/Concerns

- [Phase 4]: Hybrid search needs pre-plan research — verify Qdrant built-in BM25 sparse vector API shape (>=1.7) and TEI SPLADE model support before planning. Run `/gsd:research-phase` before planning Phase 4.
- [Phase 5 Tika, deferred to v2]: If document indexing is promoted to v1, Tika 3.x REST API and Docker image stability need verification.

## Session Continuity

Last session: 2026-02-18
Stopped at: Completed 01-03-PLAN.md (Embedder interface, TEI client, Qdrant collection init, point IDs). Ready for 01-04.
Resume file: None
