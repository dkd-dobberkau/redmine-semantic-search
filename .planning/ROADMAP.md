# Roadmap: Redmine Semantic Search (RSS)

## Overview

Redmine Semantic Search is built in five phases that follow the component dependency graph: infrastructure and embedding infrastructure must be validated before any vectors are written; issue indexing and core search with permission enforcement ship together as the indivisible minimum viable product; content breadth (wiki, journals) and operational hardening follow once the core pipeline is proven; hybrid search upgrades retrieval quality for exact-term queries; and the final phase completes the API surface with similar-issues lookup and the protected admin reindex endpoint.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Foundation** - Go module, Docker Compose, Embedder interface, Qdrant collection with payload indexes, embedding model benchmark (completed 2026-02-18)
- [ ] **Phase 2: Core Issue Search** - Issue indexer pipeline, incremental sync, deletion reconciliation, semantic search API with two-phase permission enforcement
- [ ] **Phase 3: Content Breadth and Operations** - Wiki and journal indexing, zero-downtime full reindex, operational hardening (logging, graceful shutdown, idempotency, retry, OpenAPI spec)
- [ ] **Phase 4: Hybrid Search** - Sparse vector pipeline, BM25/SPLADE integration, configurable hybrid weight, Reciprocal Rank Fusion scoring
- [ ] **Phase 5: API Completeness and Admin** - Similar issues endpoint, admin reindex endpoint, full API surface complete

## Phase Details

### Phase 1: Foundation
**Goal**: The deployment infrastructure runs, the embedding model is validated against real DE/EN content, and the Qdrant collection exists with all payload indexes — so the indexer pipeline has a correct, tested foundation to build on
**Depends on**: Nothing (first phase)
**Requirements**: INFRA-01, INFRA-02, INFRA-03, OPS-01, OPS-02
**Success Criteria** (what must be TRUE):
  1. `docker compose up` starts all services (Qdrant, TEI embedding server, Go binary) with no manual steps beyond copying an env file
  2. The Go binary embeds a sample DE and EN text and writes the vector to Qdrant without error
  3. The Qdrant collection exists with payload indexes on `project_id`, `content_type`, `tracker`, `status`, `author_id`, and `created_on` before any document is indexed
  4. A Recall@10 benchmark on representative Redmine DE/EN content confirms the selected embedding model (multilingual-e5-base or alternative) before vector dimensionality is committed
  5. All service parameters are configurable via environment variables or YAML config file with no hardcoded values
**Plans**: 4 plans

Plans:
- [x] 01-01-PLAN.md — Go module setup, project layout, config system with viper YAML + env overrides
- [x] 01-02-PLAN.md — Docker Compose stack (Qdrant, TEI, Go binary) with multi-stage Dockerfile
- [x] 01-03-PLAN.md — Embedder interface + TEI implementation; Qdrant collection init with payload indexes and alias
- [ ] 01-04-PLAN.md — Embedding model benchmark (DE/EN Recall@10 with synthetic QA pairs)

### Phase 2: Core Issue Search
**Goal**: Users can submit a natural-language query and receive permission-filtered, relevance-ranked Redmine issues — and the index stays fresh through incremental sync with deletion reconciliation
**Depends on**: Phase 1
**Requirements**: IDX-01, IDX-04, IDX-06, IDX-07, SRCH-01, SRCH-03, SRCH-04, SRCH-05, AUTH-01, AUTH-02, AUTH-03, API-01, API-02
**Success Criteria** (what must be TRUE):
  1. A user with a valid Redmine API key queries `GET /api/v1/search?q=...` and receives ranked issues from only the projects they have access to
  2. A user without a valid API key receives a 401 response; a user querying a project they cannot access receives no results from that project
  3. `GET /api/v1/health` returns the status of the Qdrant connection and embedding service
  4. Issues modified in Redmine appear in search results within one polling interval (default 5 minutes) without a full reindex
  5. Issues deleted from Redmine are removed from the index by the ID reconciliation job and no longer appear in search results
  6. Search results include faceted filters (project, tracker, status, author, date range, content type), pagination, and a text snippet per result
**Plans**: TBD

Plans:
- [ ] 02-01: Redmine REST client (paginated issue fetch, `updated_on` cursor, custom fields)
- [ ] 02-02: Indexer pipeline (fetch → text prep/chunking → embed → batch upsert to Qdrant, idempotent via deterministic UUIDs)
- [ ] 02-03: Incremental sync scheduler + deletion reconciliation (ID diff job)
- [ ] 02-04: Auth middleware (Redmine API key validation) + permission resolver (project_ids pre-filter)
- [ ] 02-05: Search handler (`GET /api/v1/search`: embed query, Qdrant filtered ANN, post-filter, facets, pagination, snippets) + health endpoint

### Phase 3: Content Breadth and Operations
**Goal**: Wiki pages and journal entries are searchable; the index can be fully rebuilt without search downtime; and the service is operationally hardened for production (structured logging, graceful shutdown, retry, idempotency, OpenAPI documentation)
**Depends on**: Phase 2
**Requirements**: IDX-02, IDX-03, IDX-05, OPS-03, OPS-04, OPS-05, OPS-06, API-05
**Success Criteria** (what must be TRUE):
  1. A user can search for content from wiki pages and journal entries, not just issue descriptions
  2. `POST /api/v1/admin/reindex` triggers a full index rebuild that completes without returning zero results to concurrent search queries (Qdrant collection alias swap)
  3. The service logs structured JSON to stdout at a configurable level, including indexing progress and error context
  4. Sending SIGTERM to the process causes in-flight batch operations to complete before the process exits
  5. The OpenAPI 3.x specification accurately documents all implemented endpoints and is served at `/api/v1/openapi.json`
**Plans**: TBD

Plans:
- [ ] 03-01: Wiki indexing pipeline (Redmine wiki REST API, Textile/Markdown to plaintext conversion)
- [ ] 03-02: Journal indexing pipeline (journal entries as independent vectors linked to parent issue)
- [ ] 03-03: Full reindex with zero downtime (shadow collection creation, alias swap, old collection cleanup)
- [ ] 03-04: Operational hardening (structured slog JSON, graceful shutdown, retry with exponential backoff, upsert idempotency verification, OpenAPI spec)

### Phase 4: Hybrid Search
**Goal**: Search results improve for exact-term queries (ticket IDs, version strings, error codes) by combining dense vector retrieval with sparse/BM25 retrieval using configurable fusion weighting
**Depends on**: Phase 3
**Requirements**: SRCH-02
**Success Criteria** (what must be TRUE):
  1. A query containing an exact ticket ID (e.g. `#12345`) or error code returns that item in the top results, not buried below semantically similar but textually different items
  2. The `hybrid_weight` parameter on `GET /api/v1/search` adjusts the blend between vector and sparse retrieval, with `hybrid_weight=0` returning pure vector results and `hybrid_weight=1` returning pure sparse results
  3. Hybrid search respects the same permission pre-filter and post-filter as semantic-only search
**Plans**: TBD

Plans:
- [ ] 04-01: Research Qdrant built-in BM25 sparse vector API (Qdrant >=1.7) and TEI SPLADE support; decide BM25 vs SPLADE approach
- [ ] 04-02: Sparse vector pipeline (generate sparse vectors at index time; store as named vector in Qdrant collection)
- [ ] 04-03: Hybrid query handler (RRF fusion, `hybrid_weight` parameter, permission filter parity with semantic search)

### Phase 5: API Completeness and Admin
**Goal**: The full API surface is live — users can discover similar issues without a new query, and administrators can trigger reindex operations through a protected endpoint
**Depends on**: Phase 3
**Requirements**: SRCH-06, API-03, API-04
**Success Criteria** (what must be TRUE):
  1. `GET /api/v1/similar/{content_type}/{id}` returns semantically similar issues to a given issue, filtered by the requesting user's permissions
  2. The similar issues endpoint returns no results for content types or IDs the requesting user cannot access, identical to search endpoint permission behavior
  3. `POST /api/v1/admin/reindex` is protected and only accessible to requests with admin-level credentials; unauthorized requests receive a 403 response
**Plans**: TBD

Plans:
- [ ] 05-01: Similar issues endpoint (reuse stored vector as query, apply permission pre/post filter, no additional embedding call)
- [ ] 05-02: Admin reindex endpoint with RBAC protection and audit logging

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5

Note: Phase 4 and Phase 5 both depend on Phase 3 and can be executed in parallel or sequentially.

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation | 4/4 | Complete   | 2026-02-18 |
| 2. Core Issue Search | 0/5 | Not started | - |
| 3. Content Breadth and Operations | 0/4 | Not started | - |
| 4. Hybrid Search | 0/3 | Not started | - |
| 5. API Completeness and Admin | 0/2 | Not started | - |
