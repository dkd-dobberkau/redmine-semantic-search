# Project Research Summary

**Project:** Redmine Semantic Search (RSS)
**Domain:** Semantic search infrastructure — Go indexer + Qdrant vector database + embedding service
**Researched:** 2026-02-18
**Confidence:** MEDIUM (architecture and features HIGH; library versions unverified due to WebSearch/WebFetch unavailability)

## Executive Summary

Redmine Semantic Search is a self-hosted semantic search infrastructure layer built on three pillars: a Go binary that indexes Redmine content and serves a search API, a Qdrant vector database for ANN retrieval with permission-aware payload filtering, and a Hugging Face Text Embeddings Inference (TEI) sidecar for embedding generation. No competitor provides this combination for self-hosted Redmine. GitLab AI search is cloud-gated, Jira's Atlassian Intelligence is cloud-only, and Linear has no semantic search at all. The recommended approach is an API-first, single-binary Go service deployed via Docker Compose — minimizing operational complexity for self-hosted Redmine operators while keeping the architecture clean enough to scale.

The recommended embedding model is `intfloat/multilingual-e5-base` (768 dimensions), which covers the German/English mixed content common in the target market. The choice of embedding model is a schema-level decision: it locks vector dimensionality at collection creation time and changing it later requires a full reindex. This model decision must be validated in the first milestone with a benchmark on real Redmine content before any vectors are written to production. The `Embedder` interface abstraction (local TEI or OpenAI) allows model swapping without code changes, but not without a reindex.

The two most critical risks are security and data staleness. The permission pre-filter (Qdrant payload filter on `project_ids`) is a performance optimization, not the security boundary — the post-filter (per-result Redmine API permission check) is the actual security boundary and must never be bypassed. Data staleness arises because `updated_on` polling cannot detect deleted Redmine objects; a periodic ID-reconciliation job is required from the moment incremental sync ships. Both risks have clear mitigations that must be baked into early milestones, not deferred.

---

## Key Findings

### Recommended Stack

The stack is Go 1.22+ for the core binary (single binary serves both indexer scheduler and search HTTP API), Qdrant as the vector store (gRPC port 6334, official `github.com/qdrant/go-client`), Hugging Face TEI as the embedding sidecar, and Apache Tika for attachment text extraction. HTTP routing uses `github.com/go-chi/chi/v5`; configuration uses `github.com/spf13/viper`; scheduling uses `github.com/robfig/cron/v3`; observability uses `github.com/prometheus/client_golang` and stdlib `log/slog`. Goroutine concurrency is managed via `golang.org/x/sync/errgroup` and retry logic via `github.com/cenkalti/backoff/v4`.

All version numbers in STACK.md require verification against official release pages before implementation — WebSearch was unavailable during research.

**Core technologies:**
- Go 1.22+: primary language — single binary, goroutine concurrency, strong stdlib (slog, net/http)
- Qdrant 1.9.x: vector database — native payload filters, named vectors, alias API, gRPC; chosen over Weaviate (heavier ops) and pgvector (limited at >100k vectors)
- `intfloat/multilingual-e5-base` (768d): recommended embedding model — covers German + English; requires `"query: "` and `"passage: "` input prefixes (critical, easy to miss)
- Hugging Face TEI: self-hosted embedding server — OpenAI-compatible REST, batch inference, CPU-viable at batch=32
- Apache Tika 2.x/3.x: attachment text extraction — Docker sidecar, handles PDF/DOCX/ODT
- `github.com/go-chi/chi/v5`: HTTP router — lightweight, stdlib-compatible, strong middleware story
- Docker Compose v2: deployment orchestration — no `version:` field, `docker compose` plugin syntax

**Critical implementation details:**
- e5 models require `"passage: "` prefix on indexed text and `"query: "` prefix on search queries — omitting this measurably degrades retrieval quality
- Qdrant point IDs must be deterministic UUIDs derived from `contentType + redmineID` to ensure upsert idempotency
- gRPC connections to Qdrant must be created once at startup and shared across all goroutines
- Qdrant batch upsert: 200 points/call at 768 dimensions; use `wait: true` for backpressure

### Expected Features

**Must have (table stakes) — v1 launch:**
- Free-text semantic query — core value proposition
- Relevance-ranked results — cosine similarity from Qdrant provides this
- Permission-aware results — non-negotiable; security failure if absent
- API-key authentication (validate against Redmine, never store credentials)
- Faceted filters (project, tracker, status, content_type, date range)
- Pagination — offset-based sufficient for MVP
- Incremental index freshness — polling every 5 minutes acceptable
- Result snippets from `text_preview` field — plain text, no semantic highlighting in v1
- REST API (`GET /api/v1/search`, `GET /api/v1/health`)
- Docker Compose deployment

**Should have (competitive differentiators) — v1.x:**
- Wiki and journal indexing (after issue search quality is validated)
- Hybrid search (vector + BM25/sparse via Qdrant named vectors + RRF fusion)
- Similar Issues (`GET /api/v1/similar/{type}/{id}`) — no additional embedding call needed
- Full reindex without downtime (Qdrant collection alias swap)
- Deletion sync (ID reconciliation job)
- Prometheus metrics (p95 latency, zero-result rate, index staleness)
- OpenAPI specification

**Defer (v2+):**
- Document indexing via Tika (high complexity — requires chunking pipeline, Tika sidecar, OCR quality issues)
- Autocomplete/suggest (requires separate prefix-indexed structure)
- Cross-encoder re-ranking (only if bi-encoder recall is demonstrably insufficient)
- LLM-generated answers (scope creep — this is retrieval infrastructure, not RAG)
- Web UI (API-first; UI is an integration concern)

**Anti-features (explicitly out of scope):**
- Real-time sync via webhooks (Redmine has no native webhooks; polling is the correct model)
- Redmine plugin (Ruby) replacement — API-first is the right approach
- User-personalized ranking — requires full MLOps pipeline

### Architecture Approach

The system follows a clean pipeline architecture: a Redmine API client feeds a worker-pool-based indexer that batches documents through an Embedder interface to Qdrant via gRPC. The search path is stateless: auth middleware validates the API key against Redmine, a permission resolver fetches and caches allowed `project_ids` (5-minute TTL), the query is embedded with the same Embedder interface, Qdrant applies the permission pre-filter and returns candidates, and a post-filter applies fine-grained checks before formatting the response. Both the indexer and the search API share a single binary, a single Qdrant gRPC connection, and the same Embedder instance.

**Major components:**
1. `internal/redmine` — paginated REST polling client; incremental sync via `updated_on` cursor
2. `internal/embedder` — `Embedder` interface with local TEI and OpenAI implementations; critical seam for model swapping
3. `internal/indexer` — pipeline orchestrator: fetch → extract → chunk → embed → batch upsert → persist sync state
4. `internal/qdrant` — thin gRPC wrapper; handles upsert, search, collection and alias management
5. `api/` — chi-based HTTP server; auth middleware; search and health handlers; permission resolver with LRU cache
6. `internal/extractor` — Tika REST client for binary attachment text extraction (deferred to v1.x/v2)
7. `internal/metrics` — Prometheus counters and histograms for indexing rate, search latency, error rate

**Key architectural patterns:**
- Embedder interface (dependency inversion) — both indexer and search handler depend on interface, not implementation
- Worker pool with bounded channels — producer goroutine feeds channel; pool of workers accumulates batches; prevents memory pressure if Qdrant is slow
- Two-phase permission filtering — Qdrant pre-filter (performance) + Redmine post-filter (security boundary)
- Qdrant collection alias for blue-green reindex — write to shadow collection, atomic alias swap, zero search downtime

### Critical Pitfalls

1. **Payload indexes not created at collection init** — Without indexes on `project_id`, `content_type`, `tracker`, `status`, etc., Qdrant does full collection scans for every filtered search. Queries that pass in testing at 1k vectors fail the 200ms P95 SLA at 100k vectors. Prevention: `EnsureCollection()` creates all indexes before the first upsert, in M1.

2. **Wrong embedding model locked in before benchmark** — Switching from MiniLM-L6-v2 (384d) to multilingual-e5-base (768d) requires recreating the collection and full reindex. Treat model selection as a schema decision, not a config detail. Run a DE/EN recall benchmark on real Redmine content in M1 before writing any production vectors. Multilingual-e5-base is almost certainly correct given the target market.

3. **Incremental sync misses deleted documents** — `updated_on` polling finds new and changed items but cannot see deletions. Ghost vectors accumulate, causing 404 search results and potential stale-permission leaks. Prevention: a periodic ID-reconciliation job (weekly or on full-reindex schedule) computes the diff between Qdrant point IDs and current Redmine IDs and deletes orphans. Must ship alongside incremental sync in M2.

4. **Permission post-filter bypassed** — The Qdrant pre-filter (`project_id` membership) is not the security boundary. Any code path that skips the post-filter (private issue check) is a security bug. Prevention: write integration tests for negative cases (user cannot see private issues in accessible projects) in M2 and never gate post-filter behind a config flag.

5. **gRPC connection pool exhaustion under parallel indexing** — Creating a new gRPC connection per goroutine causes Qdrant to reject connections during full reindex with multiple workers. Prevention: single shared `grpc.ClientConn` created at startup, passed via dependency injection; semaphore limits inflight concurrent streams. Must be correct in M1 when the Qdrant client wrapper is first written.

6. **Chunk deduplication missing** — Long documents chunked into multiple Qdrant points return the same parent document 5+ times in top-10 results. Prevention: use Qdrant `search_groups` with `group_by: "parent_id"` to return one result per parent document. Design the grouping strategy in M3 before implementing chunking in M4.

---

## Implications for Roadmap

Based on research, the architecture and pitfall analysis clearly suggest a 6-phase build order, matching the component dependency graph in ARCHITECTURE.md.

### Phase 1: Foundation and Infrastructure (M1 — Proof of Concept)

**Rationale:** Configuration, metrics, the Embedder interface, and the Qdrant client wrapper are dependencies of everything else. The embedding model benchmark must happen here — before any vectors are written — because model selection is a schema-level decision. The gRPC connection management must be correct from day one (pitfall 6). Payload indexes must be baked into collection initialization (pitfall 1).

**Delivers:** Working Go module with Docker Compose (Qdrant + TEI + Go binary); `EnsureCollection()` with all payload indexes; Embedder interface with local TEI implementation; single verified embedding of a sample document; model benchmark on representative DE/EN Redmine content.

**Features addressed:** Docker Compose deployment (NF-10); embedding infrastructure (FR-10 prerequisite)

**Pitfalls to avoid:** Missing payload indexes (create at collection init); wrong model selection (benchmark before committing); gRPC connection exhaustion (single shared connection from day one)

**Research flag:** STANDARD — Go module setup, Qdrant collection initialization, TEI Docker setup are well-documented patterns. No additional research phase needed.

---

### Phase 2: Issue Indexing and Core Search API (M2 — Core Functionality)

**Rationale:** Issues are the primary Redmine artifact and validate the core value proposition. The search API must ship with working permission enforcement (two-phase filter) from its first version — retrofitting security is higher risk than building it correctly initially. Incremental sync and the deletion reconciliation job must both ship together to prevent index drift from day one.

**Delivers:** Paginated Redmine issue fetcher; full indexer pipeline (fetch → embed → batch upsert → persist sync state); incremental sync scheduler; ID reconciliation job for deletion detection; `GET /api/v1/search` with auth middleware, permission resolver, pre-filter, post-filter, faceted filters, pagination, snippets; `GET /api/v1/health`.

**Features addressed:** FR-01 (issue indexing), FR-05 (incremental updates), FR-10 (dense vector search), FR-12 (faceted filters), FR-13 (pagination), FR-14 (snippets), FR-20/21 (permission pre/post filter), FR-22 (API-key auth), FR-30/31 (REST API, health endpoint)

**Pitfalls to avoid:** Deleted documents not synced (ship reconciliation job with sync); permission post-filter bypassed (integration tests for negative cases before marking done); oversampling pagination correctness (test across 100+ results)

**Research flag:** STANDARD — Redmine REST API pagination, Qdrant filtered ANN search, and two-phase permission patterns are well-documented. No additional research phase needed, but verify Redmine API `updated_on` timezone behavior and pagination limits against actual Redmine version in use.

---

### Phase 3: Wiki, Journal Indexing and Operational Hardening (M3 — Breadth and Quality)

**Rationale:** After issue search quality is validated with real data, add the remaining content types. Wiki and journal indexing follow the same pipeline pattern as issues — low additional complexity, high user value. Operational features (full reindex without downtime, Prometheus metrics, permission caching, deletion sync hardening) become critical once the system is in production use.

**Delivers:** Wiki and journal indexing pipelines; full reindex with Qdrant collection alias swap (zero downtime); Prometheus metrics endpoint; permission resolver with LRU cache and TTL; OpenAPI specification; deletion sync verification.

**Features addressed:** FR-02 (wiki indexing), FR-04 (journal indexing), FR-06 (full reindex without downtime), FR-07 (deletion sync), FR-23 (permission caching), NF-13 (Prometheus metrics), FR-33 (OpenAPI spec)

**Pitfalls to avoid:** Blue-green reindex breaking active searches (verify alias swap holds live traffic during reindex); chunk deduplication design (establish `parent_id` grouping strategy before M4 introduces chunking)

**Research flag:** STANDARD — Qdrant collection alias API and Prometheus Go client are standard patterns. Wiki/journal Redmine REST API endpoints follow the same pattern as issues.

---

### Phase 4: Hybrid Search and Chunking Pipeline (M4 — Quality Uplift)

**Rationale:** Hybrid search (dense + sparse) fixes the failure mode of pure semantic search: exact terms like ticket IDs (`#12345`), version strings (`v4.2.1`), and error codes. This requires Qdrant named vectors and either BM25 sparse vectors (built-in Qdrant ≥1.7) or a SPLADE model on TEI. Chunking (for long wikis and journals) requires the `parent_id` grouping strategy designed in M3. These two features are grouped because hybrid search scoring interacts with chunk ranking — they must be designed together.

**Delivers:** Sparse vector pipeline (BM25 or SPLADE via TEI named vector); configurable `hybrid_weight` parameter; Reciprocal Rank Fusion scoring; chunking pipeline (512 tokens, 50-token overlap); `search_groups` deduplication by `parent_id`.

**Features addressed:** FR-11 (hybrid search), section 8.3 (chunk-level retrieval with parent deduplication), FR-15 prerequisite

**Pitfalls to avoid:** Sparse vectors not actually stored (verify Qdrant collection info shows non-zero sparse vector count); chunk deduplication missing (verify long documents appear exactly once in results)

**Research flag:** NEEDS RESEARCH — Qdrant built-in BM25 sparse vector exact API (as of Qdrant 1.7+) vs. SPLADE on TEI: verify current API shape and performance characteristics. SPLADE model (`naver/splade-v3`) on TEI: confirm TEI supports it at the version being deployed. Hybrid scoring (RRF vs. linear combination): benchmark on real Redmine data to determine which fusion mode serves DE/EN content better.

---

### Phase 5: Similar Issues and Document Indexing (M5 — Feature Completeness)

**Rationale:** Similar Issues (`GET /api/v1/similar/{type}/{id}`) has very low incremental cost — it reuses the stored vector as the query, requires no additional embedding call, and just requires applying the existing permission filter. Document indexing via Tika is deferred to this phase because it has the highest operational complexity (Tika sidecar, chunking required for all documents, OCR quality variability) and validates only after core search quality is proven.

**Delivers:** Similar Issues endpoint with permission pre/post filter; Apache Tika sidecar integration; document indexing pipeline (with chunking, since all documents exceed token limits); fallback for OCR-poor extractions.

**Features addressed:** FR-15 (similar issues), FR-03 (document indexing via Tika)

**Pitfalls to avoid:** Similar Issues must apply the same permission post-filter as regular search (not just pre-filter); Tika hanging on corrupt or large files (always set 30s timeout, circuit breaker, continue on error); Tika encoding normalization (strip BOM, normalize to UTF-8)

**Research flag:** STANDARD for similar issues (pure vector lookup). NEEDS RESEARCH for Tika: verify current Tika 3.x Docker image stability and REST API differences from 2.x; confirm handling of password-protected PDFs and very large files.

---

### Phase 6: Production Hardening and Admin Tooling (M6 — Production Readiness)

**Rationale:** All functional milestones are complete. This phase addresses operational maturity: rate limiting, admin reindex endpoint with proper RBAC and audit logging, cache invalidation endpoint, graceful shutdown verification, race condition testing, and multilingual quality benchmarking. The "looks done but isn't" checklist from PITFALLS.md provides a direct test plan.

**Delivers:** Per-user and per-IP rate limiting at HTTP middleware layer; admin reindex endpoint with RBAC; cache invalidation endpoint; verified graceful shutdown (in-flight upsert batches complete on SIGTERM); full race-condition test suite (`go test -race`); multilingual recall benchmark (German queries against German and English content); verified `zero_result_rate` and staleness alerting.

**Features addressed:** FR-32 (admin reindex endpoint hardening), NF security hardening, NF observability completeness

**Pitfalls to avoid:** Logging API keys in structured logs (scrub Authorization headers in slog middleware); Qdrant admin ports exposed externally (Docker network isolation); client-supplied `project_ids` trusted as permission source (always resolve server-side)

**Research flag:** STANDARD — rate limiting in Go HTTP middleware, Prometheus alerting rules, and race testing are all established patterns.

---

### Phase Ordering Rationale

- Phases 1-2 are the critical path: no search capability exists until the Qdrant collection is initialized (Phase 1), the indexer pipeline runs (Phase 2), and the search API enforces permissions (Phase 2). These cannot be parallelized.
- Phase 3 can begin immediately after Phase 2 ships issue search. Wiki/journal pipelines follow the same code pattern; operational hardening becomes urgent once Phase 2 is in production.
- Phases 4 and 5 are quality-uplift phases that depend on the Phase 3 full-reindex capability (needed to add named sparse vectors to an existing collection).
- Phase 6 is not optional — it addresses security hardening that was scoped for clarity but is production-blocking. It can overlap with Phase 5 testing.
- Chunking strategy (Phase 4) must be designed before implementing document indexing (Phase 5), which requires chunking. The dependency is a design dependency, not an implementation one.

### Research Flags

Phases needing deeper research during planning:
- **Phase 4 (Hybrid Search):** Qdrant built-in BM25 sparse vector API exact shape (Qdrant ≥1.7), SPLADE model support in current TEI version, and RRF vs. linear combination benchmarking on DE/EN content all need verification against current releases. Run `/gsd:research-phase` before planning Phase 4.
- **Phase 5 (Document Indexing):** Tika 3.x REST API differences from 2.x, Docker image stability, and behavior on password-protected files need verification. Run `/gsd:research-phase` before planning the Tika integration portion of Phase 5.

Phases with standard patterns (skip research-phase):
- **Phase 1:** Go module initialization, Qdrant collection setup, Docker Compose — all well-documented.
- **Phase 2:** Redmine REST API pagination, two-phase permission filtering, Qdrant filtered ANN search — established patterns.
- **Phase 3:** Qdrant alias API, Prometheus Go client, wiki/journal REST endpoints — standard.
- **Phase 6:** Rate limiting middleware, race testing, log scrubbing — standard Go patterns.

---

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | MEDIUM | Architecture HIGH; specific library versions unverified (WebSearch unavailable). Verify all version pins at official release pages before writing go.mod. |
| Features | MEDIUM | Feature set derived from requirements doc (HIGH confidence) and competitor analysis (MEDIUM — GitLab 17.x AI search state uncertain, Redmine webhook ecosystem uncertain). |
| Architecture | HIGH | Derived directly from requirements document + well-established semantic search patterns (Qdrant docs, Go pipeline idioms, two-phase permission filtering). All patterns are production-tested at comparable scale. |
| Pitfalls | MEDIUM | All pitfalls are well-established in this domain; specific Qdrant client behaviors (DeletePoints on missing IDs, `wait` parameter semantics) should be verified against current go-client CHANGELOG. |

**Overall confidence:** MEDIUM-HIGH — architectural approach is sound and high-confidence; version pins and one or two Qdrant API behaviors need verification before implementation begins.

### Gaps to Address

- **Library version pins:** All `go get` versions in STACK.md are from training data (cutoff Aug 2025). Verify `qdrant/go-client`, `spf13/viper`, `prometheus/client_golang`, TEI, and Tika Docker tags against official release pages before writing `go.mod`.
- **Embedding model benchmark:** The recommendation of `multilingual-e5-base` is HIGH confidence directionally, but must be validated with a Recall@10 test on real Redmine content (DE/EN mix) in M1 before committing vector dimensionality. A wrong choice here requires full reindex.
- **Qdrant built-in BM25 sparse vector API:** The research flags Qdrant ≥1.7 as supporting BM25 sparse vectors natively. Verify the exact API shape and TEI integration before planning Phase 4.
- **Redmine version-specific API behavior:** `updated_on` timezone handling, project list pagination (default 25/page), and private issue field availability may vary between Redmine 4.x and 5.x. Confirm against the specific Redmine version in the target deployment.
- **Oversampling factor tuning:** The research recommends 2× oversampling for post-filter. The correct factor depends on the ratio of private issues to total issues in the target Redmine instance. Expose as a configuration parameter and document the tuning guidance.

---

## Sources

### Primary (HIGH confidence)
- `/redmine-semantic-search-requirements.md` — project requirements, data model, API spec, configuration schema (project source of truth)
- `.planning/PROJECT.md` — project context and constraints
- Qdrant official documentation (training knowledge) — collection aliases, payload indexes, named vectors, sparse vector support, `search_groups` API
- intfloat/multilingual-e5 model card — query/passage prefix requirement (documented model behavior)
- Go standard library documentation — slog, net/http, context, goroutine patterns

### Secondary (MEDIUM confidence)
- Training knowledge (cutoff Aug 2025) — library version numbers, TEI batching behavior, Redmine REST API pagination semantics, competitor feature analysis
- Hugging Face TEI documentation (training knowledge) — batch inference, model support, Docker image variants
- Apache Tika 2.x/3.x documentation (training knowledge) — REST API, encoding behavior, timeout handling

### Tertiary (LOW confidence)
- GitLab 17.x AI search feature parity — could not verify current state; cloud-gated features may have changed
- Redmine webhook plugin ecosystem — varies by Redmine version; not needed for v1 (polling confirmed as correct approach)
- Qdrant sparse vector production performance at 500k+ vectors — benchmarks from training data; should be verified against current Qdrant release notes

---

*Research completed: 2026-02-18*
*Ready for roadmap: yes*
