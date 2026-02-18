# Pitfalls Research

**Domain:** Semantic Search Infrastructure — Go + Qdrant + Embedding Models (Redmine Semantic Search)
**Researched:** 2026-02-18
**Confidence:** MEDIUM (WebSearch and WebFetch were blocked; findings are drawn from training knowledge of Go, Qdrant, embedding pipelines, and permission-filtered search systems — cross-checked against the project requirements document)

---

## Critical Pitfalls

### Pitfall 1: Payload Indexes Not Created Before Data Is Loaded

**What goes wrong:**
Qdrant allows inserting points with any payload fields, but filtering on those fields will silently do a full collection scan unless a payload index exists. The first batch of 100k vectors ingests fine, queries return correct results in testing (small data), then at production scale filtered searches time out or blow past the 200ms P95 target.

**Why it happens:**
Developers treat Qdrant like a document database where you add data first and create indexes when needed. In Qdrant, payload indexes must be created before data is written for the index to cover existing data efficiently. The collection creation step and the index creation step are separate API calls that are easy to defer or forget.

**How to avoid:**
Create all payload indexes as part of collection initialization, before the first upsert. Fields requiring indexes for this project: `project_id` (integer), `content_type` (keyword), `tracker` (keyword), `status` (keyword), `author` (keyword), `created_on` (datetime), `updated_on` (datetime). Wrap collection setup in an idempotent `EnsureCollection()` function that creates the collection and all indexes atomically on startup. Use `CreateFieldIndex` via the gRPC client immediately after `CreateCollection`.

**Warning signs:**
- Search latency is low at 1k vectors, climbs linearly past 10k
- `explain` or `search` response includes `"indexed_vectors_count": 0` for filtered fields
- CPU spikes on Qdrant during filtered searches but not pure vector searches

**Phase to address:**
M1 (Proof of Concept) — the collection initialization code must embed this from day one, otherwise the schema is wrong at every subsequent phase.

---

### Pitfall 2: Incremental Sync Missing Deleted Documents

**What goes wrong:**
The `updated_on` polling strategy finds modified and new documents but cannot detect deleted ones. Over weeks, the Qdrant index accumulates ghost vectors for Issues that were deleted in Redmine. Users receive search results pointing to 404 URLs. Worse, deleted private issues may appear in results if permission cache is warm when the delete happens.

**Why it happens:**
The Redmine REST API does not return deleted objects — they simply stop appearing. Polling `updated_on >= last_sync_time` returns new and changed items, but silence about deletions. This is a fundamental limitation of change-data-capture via timestamp polling.

**How to avoid:**
Implement a periodic reconciliation job (separate from the incremental sync) that: (1) fetches all current Redmine IDs for each content type via the REST API (paginated, IDs only, low bandwidth), (2) fetches all Qdrant point IDs for that content type via `scroll`, (3) computes the set difference, (4) deletes the orphaned Qdrant points. Run this reconciliation on the full-reindex schedule (e.g., weekly Sunday 02:00) rather than on every incremental sync cycle. For the incremental path, add a soft-consistency check: if a search result URL returns 404 from Redmine, proactively delete that point and re-run the query.

**Warning signs:**
- Growing discrepancy between Qdrant point count and Redmine object count over time
- Users report search results leading to "Issue not found" pages
- Point count in collection increases monotonically even during quiet periods on Redmine

**Phase to address:**
M2 (Kernfunktionalität) — must be addressed when incremental sync is built; leaving it to later means stale data accumulates from the start.

---

### Pitfall 3: Embedding Model Mismatch After Initial Deployment

**What goes wrong:**
The project starts with `all-MiniLM-L6-v2` (384 dimensions). After a quality benchmark reveals that German content needs `multilingual-e5-base` (768 dimensions), the team switches models. Every existing vector in Qdrant is now invalid — dimensions differ, cosine similarity scores are meaningless across model boundaries, and semantic neighborhoods are incompatible. A full reindex is required, causing an outage window or complex blue-green dance.

**Why it happens:**
Model selection is treated as a configuration detail rather than a schema-level decision. The Qdrant collection is created with a fixed vector dimension at schema creation time; switching models requires recreating the collection. Teams underestimate the cost of model migration because it works fine in development with small datasets.

**How to avoid:**
Treat model selection as a locked architectural decision before M1 data is written to production. Run a multilingual benchmark on a representative sample of actual Redmine content (DE + EN mix) before committing to a model. Given the DE/EN requirement, `multilingual-e5-base` or `multilingual-e5-large` is almost certainly the correct choice over `all-MiniLM-L6-v2` — validate this in M1 rather than accepting the English-first model as default. Store the model identifier as a payload field on a dedicated metadata point in Qdrant so the running system can detect and refuse to serve results from a mismatched model.

**Warning signs:**
- German queries return poor results while English queries are good (model is English-dominant)
- Search team discusses "just changing the model" without mentioning reindex
- No benchmark was done on actual DE/EN content before model was chosen

**Phase to address:**
M1 (Proof of Concept) — the benchmark must happen here. The model is the schema; it cannot be changed cheaply later.

---

### Pitfall 4: Pre-Filtering Permission Bypass via Oversampling Gap

**What goes wrong:**
The design uses `project_ids` as a Qdrant pre-filter (correct), plus oversampling for post-filter (oversampling_factor=2). With a large number of projects and uneven content distribution, Qdrant returns `limit * oversampling_factor` results from the permitted projects, which then get post-filtered for private issues. If a user has access to 200 projects but most content is in 3 of them, the oversampling may exhaust the private-issue post-filter pool and silently under-deliver results (pagination breaks). But the more dangerous failure is the inverse: if post-filtering is accidentally skipped (e.g., a code path that bypasses it), content from permitted projects that the user cannot see at finer granularity (private issues, confidential wikis) leaks into results.

**Why it happens:**
Two-stage filtering is easy to implement incorrectly. The pre-filter stage provides false confidence — developers test the happy path where all project members can see all content and never exercise the post-filter path. The oversampling logic adds complexity that is easy to get wrong under pagination.

**How to avoid:**
Define the security contract precisely in code: the pre-filter (Qdrant payload filter on `project_id`) is a performance optimization, NOT the security boundary. The post-filter (Redmine API permission check per result) IS the security boundary — it must run for every result, with no bypass path. Write integration tests that explicitly test: (1) a user with project access cannot see private issues within that project, (2) an admin user sees content a regular user cannot. Never skip post-filter even if pre-filter returned fewer results than limit. For pagination, implement cursor-based oversampling: request `limit * oversampling_factor` from Qdrant, post-filter, if result count < limit, fetch another batch (loop with offset), never expose raw Qdrant offsets to the client.

**Warning signs:**
- Integration tests only test happy paths (user can see all content in accessible projects)
- No test exists for private issues within accessible projects
- Post-filter is wrapped in a feature flag or can be disabled via config
- Pagination returns inconsistent result counts for the same query

**Phase to address:**
M2 (Kernfunktionalität) — permission model must be correct from the moment the API is built. Never ship this without explicit negative-case tests.

---

### Pitfall 5: Chunking Creates Duplicate Top Results from One Document

**What goes wrong:**
Long wiki pages and documents are split into overlapping chunks, each stored as a separate Qdrant point. A query semantically relevant to one section of a long document returns the top 5 results as 5 different chunks from the same document. Users see the same document listed multiple times with different snippets and different (but similar) scores. Pagination is also broken: page 1 might return 20 chunks from 3 documents while the user expected 20 distinct documents.

**Why it happens:**
Chunk deduplication ("parent-document rollup") is mentioned in the requirements but underestimated in implementation complexity. The naive approach of just storing chunks and returning raw Qdrant results does not address this. The Qdrant `group_by` feature exists for exactly this use case but requires deliberate implementation.

**How to avoid:**
Use Qdrant's `search_groups` / `group_by` API with `group_by: "parent_id"` and `group_size: 1` to get the best-scoring chunk per parent document, then expose the parent document as the result. For documents without chunking (short issues, journals), set `parent_id = redmine_id` so the grouping still works uniformly. When the snippet is generated, use the chunk's text (the best-matching chunk), not the full document text. Test this with a document that has 10+ chunks and verify the search API returns it as one result.

**Warning signs:**
- A single long wiki page appears 5+ times in top-10 results for any query touching that page
- Result count in API response matches Qdrant raw result count (not document count)
- No `group_by` or deduplication logic exists in search handler code

**Phase to address:**
M3 (Hybrid Search & Qualität) — chunking is introduced in M4 but the grouping strategy must be designed in M3 alongside hybrid search, because hybrid search scoring changes how chunks rank relative to each other.

---

### Pitfall 6: gRPC Connection Pool Exhaustion Under Concurrent Indexing

**What goes wrong:**
The Go indexer uses goroutines to process content types in parallel (issues, wikis, journals, documents simultaneously). Each goroutine creates its own gRPC connection to Qdrant, or worse, calls `grpc.Dial` repeatedly without connection reuse. Under a full reindex with 4 workers and batches of 100, the gRPC connection count to Qdrant spikes, Qdrant starts rejecting connections, and the reindex fails partway through with "connection refused" or "stream closed" errors.

**Why it happens:**
`grpc.Dial` is cheap to call but each call creates a new TCP connection by default. Developers used to HTTP clients (which handle connection pooling automatically) do not realize gRPC connections must be explicitly shared. The Qdrant gRPC client wrapper (`github.com/qdrant/go-client`) does not enforce connection sharing.

**How to avoid:**
Create a single `grpc.ClientConn` at application startup and pass it to all components via dependency injection. The Qdrant Go client accepts a shared connection. For concurrent batch upserts, use a semaphore (buffered channel in Go) to limit inflight gRPC calls to Qdrant, not more than 10-20 concurrent streams. Set `grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(...))` appropriately for large batch responses. Add a connection health check on startup and a reconnect loop.

**Warning signs:**
- Qdrant logs show "max concurrent streams exceeded"
- Full reindex fails randomly at different progress points
- Go process file descriptor count climbs during reindex
- Indexing works fine with 1 worker but fails with 4

**Phase to address:**
M1 (Proof of Concept) — the Qdrant client wrapper should be built correctly from the start. Fixing connection management after goroutine architecture is established is a refactoring task.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Truncate long texts to 512 tokens instead of chunking | Simpler code, one vector per document | Poor recall for long wiki pages; only first 512 tokens indexed | MVP only if documents are mostly short issues; must be replaced in M4 |
| Store permission check result in-memory Go map (no Redis) | No external dependency | Permission cache lost on restart; cold-start latency spike on first requests | Acceptable for single-instance deployment, not for multi-replica |
| Use `offset`-based Qdrant pagination instead of scroll cursors | Simpler API | Qdrant `offset` pagination degrades at high offsets (full scan); breaks for >10k results | Never for production search; offset is only safe for admin tooling |
| Single Qdrant collection for all content types | Simple schema | Cannot tune vector index parameters per content type; harder to reindex one type without affecting others | Acceptable for MVP at <100k vectors; review at M5 |
| Synchronous Redmine API calls in indexer (no queue) | No queue infrastructure needed | Redmine API rate limits or slowdowns block the entire indexer pipeline | Acceptable for MVP polling interval >1min; add worker queue if interval drops to seconds |
| Hardcode hybrid search weight (0.3) | Avoids tuning complexity | Weight may be wrong for DE/EN mixed content; no ability to tune per query type | Acceptable as default; must expose as config before M5 |

---

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Redmine REST API | Assuming `updated_on` filter is timezone-naive | Redmine `updated_on` is UTC; always send comparison timestamps in UTC ISO 8601 format, never local time |
| Redmine REST API | Fetching all projects in one call | Redmine paginates project lists at 25 per page by default; must paginate with `offset` and `limit` up to the `total_count` response field |
| Redmine REST API | Using the admin API key for all requests | The search API must impersonate the requesting user's API key to get accurate project membership; using admin key bypasses all permission boundaries |
| Redmine REST API | No handling for Redmine's 429 / 503 responses | Redmine under load returns 503 with no Retry-After header; implement exponential backoff with jitter, not fixed sleep |
| Qdrant gRPC | Sending vectors as `[]float64` instead of `[]float32` | Qdrant stores vectors as float32 internally; sending float64 wastes bandwidth and may cause subtle precision differences; always use float32 |
| Qdrant gRPC | Using `DeletePoints` with point IDs that may not exist | Qdrant returns an error if you try to delete a non-existent point ID in some client versions; use `DeletePoints` with a filter condition instead, or check existence first |
| Qdrant gRPC | Not setting `wait: true` on upsert during reindex | With `wait: false` (async), the indexer can outpace Qdrant's indexing and the collection is not fully searchable until Qdrant catches up; use `wait: true` during batch upsert to get backpressure |
| Hugging Face TEI | Sending unbounded text to embedding API | TEI truncates silently at model max tokens; very long texts get truncated without error — meaning the embedding represents only the beginning; pre-tokenize and chunk before sending |
| Apache Tika | No timeout on Tika REST calls | Large PDFs or corrupt files can cause Tika to hang indefinitely; always set a 30s timeout on the HTTP call and a separate circuit breaker for repeated Tika failures |
| Apache Tika | Assuming Tika always returns UTF-8 | Tika may return content with mixed encodings or BOM markers for older Office files; normalize all extracted text to UTF-8 with invalid-sequence stripping before embedding |

---

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Fetching full Redmine issue text per API call without batching | Indexer throughput stays at 5-10 docs/sec despite goroutines; Redmine load spikes | Batch Redmine API calls using `?ids[]=1&ids[]=2` where the API supports it; for issues use pagineted list endpoint rather than per-issue fetch | Always; this trap prevents meeting the 100 docs/sec target from day one |
| Embedding one text at a time (no batch) | Embedding latency dominates; TEI/HF model underutilized | Send texts in batches of 32-64 to the embedding API; TEI supports batch inference natively | At any scale; batching provides 10-20x throughput improvement |
| Not creating a sparse vector index before hybrid search | Hybrid search query time is O(n) for sparse component | Create named sparse vector field in Qdrant collection schema; Qdrant uses an inverted index for sparse vectors only if the field is declared as sparse at collection creation | At >10k vectors, sparse scan becomes noticeable; at >100k it is the dominant cost |
| Qdrant `scroll` without `with_payload: false` for ID reconciliation | Reconciliation job transfers entire payload for all 500k points | Use `scroll` with `with_payload: false, with_vector: false` for ID-only reconciliation; only fetch what you need | Immediately visible at 100k+ points: scroll returns megabytes of unnecessary data |
| Rebuilding full reindex in the same collection (blocking search) | Search returns stale or empty results during reindex | Implement blue-green: create new collection with alias, populate it, atomically swap alias, delete old collection | Whenever a full reindex is triggered; this is a correctness issue, not just performance |
| Qdrant `hnsw_config` defaults insufficient for filtered search | Filtered search accuracy degrades (false negatives) at 500k vectors | Increase `m` (HNSW graph degree) and `ef_construct` for the collection when accuracy benchmarks reveal recall < 0.95; defaults are tuned for pure ANN, not filtered ANN | At 100k+ vectors with selective filters (e.g., single project with 5% of all content) |

---

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Logging Redmine API keys in structured logs | API keys exfiltrated via log aggregation (ELK, Loki, etc.) | Scrub all credential fields from log context; use a log middleware that strips `Authorization`, `api_key`, `X-Redmine-API-Key` headers before writing to slog |
| Exposing the Qdrant admin port (6333 HTTP + 6334 gRPC) on 0.0.0.0 | Anyone on the network can read, modify, or delete all vectors without authentication | Qdrant has no built-in authentication in the open-source version; bind to `127.0.0.1` or use Docker network isolation; never expose Qdrant ports outside the container network |
| Trusting client-supplied `project_ids` in search filter | Clients bypass permission model by supplying arbitrary project IDs | The search API must always resolve project_ids server-side from the authenticated user's Redmine token; never accept project_ids from the request body as the permission source |
| Redmine admin API key used as the indexer credential with read-write access | Compromise of indexer container gives attacker full Redmine admin access | Create a dedicated Redmine user for the indexer with read-only access; use the minimum required permissions |
| No rate limiting on the search API | A single slow client or scraper can starve embedding model and Qdrant | Implement per-user and per-IP rate limiting at the Go HTTP middleware layer; the embedding call is the most expensive operation — limit to 10 req/s per user |
| Storing the permission cache (project_ids per user) in a shared cache without user isolation | Cache poisoning: one user's permission set visible to another | Namespace all cache keys by user ID; if using an in-memory map, never expose the map directly — always copy the slice before returning |

---

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Returning raw cosine similarity scores (0.0–1.0) to users | Users don't know what 0.73 means; they expect ranked results, not scores | Normalize or hide scores in the default response; optionally expose scores behind a `?debug=true` flag for developers |
| No feedback when the index is stale (last sync failed) | Users get outdated results and assume the system is broken | Expose `last_successful_sync_at` in the `/health` response and surface it in any admin UI; add a staleness alert if last sync > 2x polling interval |
| Snippet generation that cuts mid-word or mid-sentence | Snippets are unreadable; users cannot assess relevance | Use sentence boundary detection for snippet extraction; extend snippet to the nearest sentence end, not just character count |
| Returning journal entries (comments) as top results for issue queries | Users see a comment from 2021 at rank #1 instead of the actual issue | Apply a small score bias toward `content_type=issue` for queries that don't specify a content type filter; or group journals under their parent issue in results |
| No distinction between "no results found" and "you have no project access" | Users think the system is broken when they actually have no projects | The API should return `{"results": [], "reason": "no_projects_accessible"}` when the user has zero permitted projects, distinct from a genuine empty result set |

---

## "Looks Done But Isn't" Checklist

- [ ] **Incremental Sync:** Appears to work in testing — verify it also handles deletions, not just updates and inserts. Check point count divergence over 7 days.
- [ ] **Hybrid Search:** BM25/sparse component appears in config — verify sparse vectors are actually stored (check Qdrant collection info for named sparse vector dimension) and not just dense vectors.
- [ ] **Permission Pre-Filter:** Search returns only results from accessible projects — verify the filter uses the server-resolved project_ids from the user's Redmine token, not a client-supplied parameter.
- [ ] **Full Reindex Without Downtime:** Reindex completes — verify that search remained available during reindex (query the alias during reindex, check response times, confirm results are served from old collection until swap).
- [ ] **Chunking Deduplication:** Chunked documents appear in results — verify each document appears exactly once in the result list even when multiple chunks score highly.
- [ ] **Multilingual Quality:** Search works — run German queries against German content AND cross-lingual queries (German query, English content or vice versa) and measure recall.
- [ ] **Tika Timeout Handling:** Document indexing completes — verify what happens when Tika is given a 100MB PDF or a corrupt DOCX (timeout fires, error is logged, indexer continues with next document).
- [ ] **Embedding Batch Error Handling:** Batch upsert succeeds — verify that a single malformed text in a batch of 32 does not silently drop all 32 embeddings (check TEI error response format and Go error handling).
- [ ] **Graceful Shutdown:** Service stops cleanly — verify that in-flight Qdrant upsert batches complete (or are rolled back cleanly) when SIGTERM is received during a full reindex.

---

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Wrong embedding model in production | HIGH | (1) Create new collection with correct model name/dimension, (2) trigger full reindex into new collection, (3) swap alias after reindex completes, (4) delete old collection. Expect 30–60 min downtime or blue-green complexity. |
| Stale vectors for deleted Redmine objects | MEDIUM | Run ID reconciliation job manually: scroll all Qdrant IDs, fetch all Redmine IDs via API, delete the diff. Can run without downtime. |
| Missing payload indexes (filter performance) | MEDIUM | Create payload indexes on running collection via `CreateFieldIndex` API — Qdrant will build the index in background without downtime. Monitor indexing status before declaring resolved. |
| Corrupt sync state (last_sync timestamp wrong) | LOW | Reset sync state to an earlier timestamp and re-run incremental sync. Upsert semantics mean re-processing already-indexed documents is safe — they will be overwritten idempotently. |
| Permission cache serving stale data after user role change | LOW | Expose a cache invalidation endpoint (`POST /admin/cache/invalidate`) or reduce TTL temporarily. For immediate fix, restart the search API service (cache is in-memory). |
| Qdrant collection in inconsistent state after partial reindex | HIGH | Always keep the old collection alive until the alias swap is confirmed successful. Recovery: re-trigger full reindex. Do not attempt to patch a partially-populated collection. |

---

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Payload indexes missing | M1 — collection initialization | Query filtered search at 10k vectors and measure latency; must be <50ms for filter-only query |
| Incremental sync misses deletions | M2 — sync logic | Run 7-day simulation: index 1000 items, delete 50 in Redmine, run sync cycles, verify point count drops |
| Wrong embedding model selection | M1 — benchmark before collection creation | Recall@10 test on DE/EN query set; German queries must return German-language results in top 3 |
| Permission filter bypass | M2 — search API authentication | Integration test: user A cannot see content from projects user A is not a member of, including private issues in accessible projects |
| Chunk deduplication missing | M3 (design) + M4 (implementation) | Single long document must appear exactly once in top-10 results for a query highly relevant to that document |
| gRPC connection exhaustion | M1 — Qdrant client wrapper | Full reindex with 8 parallel workers must complete without connection errors |
| Deleted objects in index | M2 — reconciliation job | After manually deleting 10% of test data in Redmine, reconciliation job must reduce Qdrant point count by same amount |
| Sparse vector not actually stored | M3 — hybrid search implementation | Verify via Qdrant collection info API that named sparse vector exists and has non-zero indexed point count |
| Tika hanging on bad files | M4 — document pipeline | Feed a 200MB PDF and a zero-byte file; both must complete within 35 seconds with error logged, not hung goroutine |
| Oversampling pagination correctness | M2 — search API | Paginate through 100+ results for a broad query; verify no duplicate results across pages, verify total count is stable |

---

## Sources

- Qdrant official documentation (training knowledge): collection configuration, payload indexes, sparse vectors, HNSW parameters, scroll API, search_groups
- Qdrant Go client (`github.com/qdrant/go-client`) gRPC usage patterns
- Hugging Face Text Embeddings Inference (TEI) batching behavior
- Go gRPC best practices: single connection, semaphore-bounded concurrency
- Redmine REST API pagination and `updated_on` filter semantics (v4.x, v5.x)
- Apache Tika REST API timeout and encoding behavior
- General vector search system design: blue-green reindex, permission pre-filtering, chunk deduplication, model migration cost

**Confidence note:** WebSearch and WebFetch were unavailable during this research session. All findings are from training knowledge verified against the project requirements document. Confidence is MEDIUM — the pitfalls are well-established patterns in this domain, but specific Qdrant version behaviors (e.g., exact behavior of `DeletePoints` on missing IDs, `wait` parameter semantics) should be confirmed against the current Qdrant Go client documentation and CHANGELOG before implementation.

---
*Pitfalls research for: Redmine Semantic Search (RSS) — Go + Qdrant + Embedding Pipeline*
*Researched: 2026-02-18*
