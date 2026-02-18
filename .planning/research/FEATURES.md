# Feature Research

**Domain:** Semantic search infrastructure for project management / issue tracking (Redmine)
**Researched:** 2026-02-18
**Confidence:** MEDIUM — WebSearch and WebFetch unavailable; findings based on training knowledge (cutoff Aug 2025) of production semantic search systems (Elasticsearch, Qdrant, Jira, GitLab, Linear, Notion) cross-referenced against the project requirements document. Mark LOW-confidence items are flagged inline.

---

## Feature Landscape

### Table Stakes (Users Expect These)

Features users assume exist. Missing these = product feels incomplete or untrustworthy.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Free-text semantic query | Users come from Google-level expectations — natural language "show me login problems after v4.2" must work, not just `status:open AND title:login` | LOW (integration point, not novel) | Core value prop. Requires embedding + vector search pipeline already defined in requirements. |
| Relevance-ranked results | Every search engine returns results sorted by score. Unranked = unusable. | LOW | Cosine similarity score from Qdrant provides this out of the box. |
| Faceted filters | Project, tracker, status, author, date range, content type. Users of Jira and GitLab expect this sidebar filtering model. | MEDIUM | Requires Qdrant payload indexes on all filter dimensions. Already in FR-12. |
| Pagination | Browsing more than 20 results is a fundamental search UX expectation. | LOW | FR-13 covers this. Offset-based is sufficient for MVP; keyset later. |
| Permission-aware results | A search system that leaks cross-project content is a security failure. Users trust it won't show what they shouldn't see. | HIGH | Two-stage pre-filter + post-filter. Core differentiator safety net. FR-20/21. |
| Result snippets with context | Users need to see *why* a result matched — a surrounding text excerpt, not just the title. | MEDIUM | Keyword highlighting within snippet is expected. Semantic highlighting (where the meaning matched) is harder. FR-14. |
| Search across all content types | Issues only is not enough — wikis, documents, and journals must be reachable from one query. | MEDIUM | Requires separate indexing pipelines per type, unified result set. FR-01 through FR-04. |
| Direct deep-link to result | Every result must link directly to the Redmine object (issue, wiki page, document). | LOW | URL field in payload already planned in data model section 6.1. |
| Incremental index freshness | Results more than a few minutes stale erode trust. Users expect search to reflect recent edits. | MEDIUM | Polling every 5 minutes is acceptable for MVP. FR-05. |
| Health / status transparency | Admins need to know if the system is indexing, lagging, or broken. | LOW | FR-31 health endpoint. |
| API-key authentication passthrough | The system must accept the same Redmine API keys users already have. Re-inventing auth = adoption blocker. | MEDIUM | FR-22. Must not store credentials itself — validate against Redmine. |

### Differentiators (Competitive Advantage)

Features that set the product apart. Not required by all users, but valued.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Hybrid search (vector + BM25/sparse) | Pure semantic search fails on exact terms: ticket IDs (`#12345`), error codes (`ORA-00942`), version strings (`v4.2.1`), proper nouns. Hybrid catches both. Redmine's own search is keyword-only; this system would be the first to combine both. | HIGH | Requires sparse vector pipeline (SPLADE or BM25 tokenizer), named vectors in Qdrant, reciprocal rank fusion or linear blend. FR-11. Configurable `hybrid_weight` parameter is the right abstraction. |
| Similar Issues ("More Like This") | Prevents duplicate issue creation. When a user opens a new issue, showing semantically similar existing ones saves support time. This is not available in stock Redmine at all. | MEDIUM | Uses the existing vector for a given `redmine_id` as query. Requires no additional embedding call. FR-15. |
| Cross-content-type unified ranking | A search for "deploy fails" should surface a wiki runbook, a bug report, and a journal comment in a single ranked list — not siloed by type. Few systems do this well. | MEDIUM | Requires score normalization across content types (raw cosine scores across different embedding distributions may not be directly comparable). Needs a calibration step or per-type score buckets. |
| Multilingual embedding model | Most Redmine instances in German-speaking markets (like this project's context) mix German and English. A multilingual model (`multilingual-e5-base`) means users can query in German and match English content and vice versa. No other Redmine search add-on does this. | MEDIUM | Model selection is a deployment decision, not a code change (behind Embedder interface). Key selling point for the target market. |
| Configurable hybrid weighting per query | Exposing `hybrid_weight` as a per-request parameter lets integrators tune the blend (e.g., a UI slider or a per-tracker default). Rare in OSS search layers. | LOW | Already in API spec. Document as intentional design, not just an internal tuning knob. |
| Attachment full-text semantic indexing | Searching inside PDFs, DOCX, ODT files is something Redmine cannot do natively. A user searching for a spec mentioned only in an attachment will find it. | HIGH | Depends on Apache Tika sidecar. Text quality varies (scanned PDFs yield garbage). Needs confidence-based fallback. FR-03. |
| Oversampling + post-filter for private issues | Redmine has per-issue privacy flags (private issues). Pre-filtering by project is not granular enough. Oversampling then discarding unauthorized results gives correct recall without exposing data. | HIGH | Oversampling factor must be tuned — too high wastes Qdrant capacity, too low causes pagination underflow. FR-20, section 10. |
| Prometheus metrics for search quality | Exposing `p95_latency`, `result_count_distribution`, `zero_result_rate` helps operators detect index staleness or model degradation before users notice. | MEDIUM | Standard Prometheus exposition. Most semantic search add-ons omit this. NF-13. |
| Blue-green reindex (zero search downtime) | A full reindex that interrupts search for 30 minutes is unusable in production. Collection aliasing in Qdrant solves this. Competitors using Elasticsearch typically need maintenance windows. | HIGH | Requires alias management in Qdrant, atomic pointer swap. FR-06. |
| Chunk-level retrieval with parent deduplication | Long wiki pages return the most relevant passage, not just "wiki page X matched". Deduplication ensures the same parent document appears once with the best-scoring chunk. | HIGH | Requires chunking pipeline, `parent_id` tracking, result merging. Section 8.3. |

### Anti-Features (Commonly Requested, Often Problematic)

Features that seem good but create problems. Explicitly out of scope for v1.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Autocomplete / suggest | Feels polished, users expect it from Google | Partial-text vector search is qualitatively different from prefix search. Naive implementation (vectorize partial input every keystroke) is expensive and produces poor results. Proper autocomplete needs a separate prefix-indexed structure (e.g., Qdrant sparse + BM25 prefix) or a dedicated suggest index. Defers significant complexity. | Build basic prefix-match suggest on title/subject fields only, as a v2 addition, after validating core search quality. |
| Real-time index updates via Redmine webhooks | "Search shows stale data" complaints drive this request | Stock Redmine has no native webhooks. Webhook plugins exist but vary by version and are not universally installed. Making webhooks the primary sync mechanism ties RSS to a third-party plugin dependency, breaks on Redmine upgrades, and requires a webhook receiver with guaranteed delivery. | Polling every 5 minutes is acceptable for MVP. Design the sync interface to accept push events as a future enhancement (event queue adapter), but ship with polling. |
| Redmine plugin (Ruby) replacement of native search | Seamless UX — users see improved search without leaving Redmine | Requires Ruby/Rails expertise, tight coupling to Redmine internals, re-testing on every Redmine upgrade. Maintenance burden is high. The API-first approach lets any frontend integrate. | Ship a standalone REST API. Document how to build a Redmine plugin on top of it as a community contribution. |
| LLM-generated answer summaries ("AI search") | Attractive after ChatGPT popularized this pattern | Out of scope for a search *infrastructure* layer. Adds LLM API dependency, cost, latency, and hallucination risk. The system's job is retrieval, not generation. Adding RAG on top of a broken retrieval layer just produces confidently wrong answers. | Ensure the retrieval API is clean enough for an integrator to layer RAG on top later. Return high-quality snippets and scores. |
| User-personalized ranking (learned preferences) | "Results should improve based on what I click" | Requires click tracking, feedback collection, re-ranking model training pipeline. A full MLOps stack for what is a search infrastructure project. | Return relevance scores that a UI layer can use to collect implicit feedback. Build the feedback loop as a v3+ product decision. |
| Full web UI / frontend | "We need a UI to use this" | Scope creep. RSS is infrastructure. Building a React SPA competes with the Redmine plugin approach and ties the project to frontend tech choices that have nothing to do with the search quality. | API-first. Document the response format clearly so teams can build their own UI or plugin. Example curl/Postman collection is sufficient for v1. |
| Cross-Redmine federation (searching multiple Redmine instances) | Organizations with multiple Redmine instances want unified search | Different permission models, different Redmine versions, different project ID namespaces. Multiplies authentication and namespace collision complexity. Not needed to validate the core value. | Single-instance first. Design the Qdrant collection naming convention to allow multi-tenant extension (`{instance_id}_redmine_search`) as a future pattern, but do not build the federation layer in v1. |
| Semantic re-ranking via cross-encoder | Better search quality for top-N results | Cross-encoders (e.g., ms-marco-MiniLM) require running a second model for every query-document pair in the result set. At 20 results per query and high concurrency, this multiplies inference cost significantly. Only worth it if bi-encoder recall quality is demonstrably insufficient. | Measure bi-encoder quality first. Add cross-encoder re-ranking as a quality-tier option in v2 if benchmark shows meaningful improvement. |

---

## Feature Dependencies

```
[Permission Pre-Filter (FR-21)]
    └──requires──> [Auth / API Key Validation (FR-22)]
    └──requires──> [Project membership cache (FR-23)]

[Hybrid Search (FR-11)]
    └──requires──> [Sparse Vector Pipeline]
                       └──requires──> [SPLADE or BM25 tokenizer integration]
    └──requires──> [Dense Vector Search (FR-10)]
    └──requires──> [Score fusion / blend logic]

[Similar Issues (FR-15)]
    └──requires──> [Dense Vector Search (FR-10)]
    └──requires──> [Issue Indexing (FR-01)]
    (No additional embedding call — uses stored vector as query)

[Snippet Generation (FR-14)]
    └──requires──> [text_preview field in Qdrant payload]
    └──enhances──> [Chunk-level retrieval (section 8.3)]
    (Semantic highlighting enhances but is not required for FR-14)

[Chunk-level retrieval (section 8.3)]
    └──requires──> [Dense Vector Search (FR-10)]
    └──requires──> [parent_id tracking in payload]
    └──requires──> [Result deduplication logic]

[Document Indexing via Tika (FR-03)]
    └──requires──> [Apache Tika sidecar service]
    └──requires──> [Chunk-level retrieval] (documents exceed token limits)

[Full Reindex without downtime (FR-06)]
    └──requires──> [Qdrant collection alias management]
    └──conflicts──> [Single-collection simple setup] (alias adds operational complexity)

[Deletion Sync (FR-07)]
    └──requires──> [ID reconciliation loop OR webhook receiver]
    └──enhances──> [Incremental Updates (FR-05)]

[Faceted Filters (FR-12)]
    └──requires──> [Qdrant payload indexes on: project_id, tracker, status, author, content_type, created_on, updated_on]
    └──requires──> [All four content type indexers (FR-01 through FR-04)]

[Oversampling post-filter]
    └──requires──> [Faceted Filters (FR-12)]
    └──requires──> [Auth pipeline (FR-22)]
    └──enhances──> [Permission Pre-Filter (FR-21)]

[Prometheus Metrics (NF-13)]
    └──requires──> [Health Endpoint (FR-31)]
    └──enhances──> all operational features

[Multilingual embedding]
    └──requires──> [Embedder interface abstraction]
    └──conflicts──> [Single-language index] (switching models requires full reindex)
```

### Dependency Notes

- **Hybrid Search requires Sparse Vector Pipeline:** This is the highest-risk dependency. BM25 tokenization or SPLADE inference adds a second model with different operational characteristics. It must be optional (fallback to pure dense) and behind a feature flag.
- **Document indexing requires chunk-level retrieval:** Documents routinely exceed 512 token limits. Implementing FR-03 without chunking produces truncated, low-quality vectors. These two must be built together.
- **Full Reindex without downtime requires alias management:** This cannot be retrofitted easily. Design the collection management layer with alias support from the first collection creation. Retrofitting breaks existing Qdrant point ID schemes.
- **Changing embedding models requires full reindex:** A switch from MiniLM (384d) to multilingual-e5 (768d) changes vector dimensionality — incompatible. Document this constraint prominently. Blue-green reindex capability (FR-06) is the mitigation.
- **Permission Pre-Filter and Similar Issues do not conflict:** `similar/{type}/{id}` must also apply the permission pre-filter. The caller's `project_ids` must still gate the "similar" result set, not just the regular search.

---

## MVP Definition

### Launch With (v1 — validates core value)

Minimum viable product — what's needed to validate that semantic search over Redmine is meaningfully better than keyword search.

- [ ] **Issue indexing (FR-01)** — Without issues, there is no core value to validate. Issues are the primary artifact in Redmine.
- [ ] **Incremental updates (FR-05)** — A search layer that requires manual reindex to reflect changes is not usable in production. Must run automatically.
- [ ] **Dense vector search (FR-10)** — The semantic search itself. Core value prop.
- [ ] **Faceted filters on project, tracker, status, content_type (FR-12)** — Users need to scope results. Without at least project filter, cross-project result mixing erodes trust.
- [ ] **Pagination (FR-13)** — Without pagination, you cannot trust completeness of results.
- [ ] **Permission pre-filter (FR-21)** — Non-negotiable. Shipping without this is a security failure.
- [ ] **API-key authentication (FR-22)** — Must validate against Redmine before any result is returned.
- [ ] **REST API GET /search and GET /health (FR-30, FR-31)** — The integration surface. Without these, nothing downstream can use the system.
- [ ] **Docker Compose deployment (NF-10)** — Required for self-hosted Redmine customers to actually deploy.
- [ ] **Snippet generation from text_preview (FR-14, simplified)** — Plain text_preview field returned as snippet is sufficient for v1. No semantic highlighting needed.

### Add After Validation (v1.x)

Add once the core search quality is validated with real data.

- [ ] **Wiki indexing (FR-02)** — Second most important content type. Add once issue search quality is confirmed.
- [ ] **Journal indexing (FR-04)** — High value for "what was the discussion on this topic" queries.
- [ ] **Full reindex with alias (FR-06)** — Once production use begins, zero-downtime reindex becomes critical.
- [ ] **Deletion sync (FR-07)** — Index drift becomes a real problem once the system is in regular use.
- [ ] **Hybrid search (FR-11)** — After benchmarking pure semantic results, add hybrid to fix exact-match failures.
- [ ] **Similar Issues (FR-15)** — High value, low incremental cost once vector search is in place.
- [ ] **Permission caching (FR-23)** — Needed once real query volume hits.
- [ ] **Prometheus metrics (NF-13)** — Once deployed, operators need visibility.
- [ ] **OpenAPI spec (FR-33)** — Required for any third-party integration.

### Future Consideration (v2+)

Defer until product-market fit is established.

- [ ] **Document indexing via Tika (FR-03)** — High complexity (Tika sidecar, chunking required, OCR quality issues). Defer until issue+wiki+journal search is validated.
- [ ] **Autocomplete/suggest** — Separate index required. Not MVP.
- [ ] **Multilingual model benchmarking** — Run a quality benchmark with real Redmine data before committing to a model choice for production. The Embedder interface supports this without code changes.
- [ ] **Cross-encoder re-ranking** — Only after bi-encoder quality is measured and found lacking.
- [ ] **Admin reindex endpoint hardening (FR-32)** — Basic endpoint in v1, proper RBAC and audit logging in v2.

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Dense vector search | HIGH | MEDIUM | P1 |
| Issue indexing | HIGH | MEDIUM | P1 |
| Permission pre-filter | HIGH | HIGH | P1 |
| API-key auth | HIGH | MEDIUM | P1 |
| Faceted filters | HIGH | MEDIUM | P1 |
| Pagination | HIGH | LOW | P1 |
| Incremental updates | HIGH | MEDIUM | P1 |
| Result snippets (basic) | HIGH | LOW | P1 |
| REST API surface | HIGH | LOW | P1 |
| Docker Compose deployment | HIGH | LOW | P1 |
| Wiki indexing | HIGH | MEDIUM | P2 |
| Journal indexing | MEDIUM | MEDIUM | P2 |
| Hybrid search | HIGH | HIGH | P2 |
| Similar Issues | HIGH | MEDIUM | P2 |
| Full reindex without downtime | HIGH | HIGH | P2 |
| Deletion sync | MEDIUM | MEDIUM | P2 |
| Permission caching | MEDIUM | LOW | P2 |
| Prometheus metrics | MEDIUM | LOW | P2 |
| OpenAPI spec | MEDIUM | LOW | P2 |
| Document indexing (Tika) | MEDIUM | HIGH | P3 |
| Autocomplete/suggest | LOW | HIGH | P3 |
| Cross-encoder re-ranking | MEDIUM | HIGH | P3 |
| Web UI | LOW | HIGH | Anti-feature |
| Webhook-based sync | LOW | HIGH | Anti-feature |
| LLM answer generation | LOW | HIGH | Anti-feature |

**Priority key:**
- P1: Must have for launch
- P2: Should have, add when possible
- P3: Nice to have, future consideration

---

## Competitor Feature Analysis

The direct competitor space for "semantic search over a self-hosted project management tool" is narrow. The relevant comparators are the search capabilities built into Jira, GitLab, Linear, and Notion — plus general-purpose semantic search layers (Elasticsearch, OpenSearch, Typesense).

| Feature | GitLab Advanced Search (Elasticsearch) | Jira / Atlassian Intelligence | Linear | RSS (our approach) |
|---------|----------------------------------------|-------------------------------|--------|--------------------|
| Keyword search | Yes (full-text, Lucene) | Yes | Yes | Yes (BM25 via sparse vectors) |
| Semantic/vector search | Partial — GitLab 17.x added AI search in limited beta (LOW confidence) | Atlassian Intelligence (cloud-only, proprietary) | No — keyword only | Yes — core feature |
| Self-hosted | Yes (requires Elasticsearch cluster) | Data Center only (no semantic) | No | Yes — designed for self-hosted |
| Permission-aware | Yes | Yes | Yes | Yes — explicit pre-filter design |
| Cross-content-type | Yes (issues, MRs, wikis, code) | Yes (issues, pages) | Issues only | Yes (issues, wikis, docs, journals) |
| Faceted filters | Yes | Yes | Yes | Yes |
| Hybrid search | Elasticsearch RRF (v8+) | Unknown | No | Yes — configurable weight |
| Similar issues | No native feature | Atlassian Intelligence (cloud) | No | Yes (FR-15) |
| Attachment full-text | Yes (code files, not arbitrary docs) | Yes (with indexer) | No | Yes (via Tika) |
| Open source | Search integration OSS, Elasticsearch proprietary | No | No | Yes |
| Embedding model choice | Fixed (GitLab's hosted model) | Fixed (Atlassian's) | N/A | Configurable — local or cloud |
| Multilingual | Depends on Elasticsearch analyzer | Limited | No | Yes — via multilingual-e5 model |
| Redmine compatibility | No | No | No | Yes — designed for Redmine |

**Key insight:** No competitor provides self-hosted semantic search for Redmine. GitLab's AI search is cloud-gated and GitLab-specific. Jira's Atlassian Intelligence is cloud-only. Linear lacks semantic search entirely. RSS fills a real gap for self-hosted Redmine users who need semantic search with data sovereignty.

The differentiator that survives competitive scrutiny: **self-hosted semantic search for Redmine with permission enforcement, hybrid search, and configurable embedding models (local or cloud)**.

---

## Sources

- Project requirements document: `/redmine-semantic-search-requirements.md` (February 2026, authored by Olivier Dobberkau / dkd Internet Service GmbH)
- Project context: `.planning/PROJECT.md`
- Qdrant documentation: training knowledge (cutoff Aug 2025) — hybrid search via named vectors, payload filtering, collection aliases. HIGH confidence for core Qdrant capabilities (well-documented, stable).
- GitLab Advanced Search capabilities: training knowledge + GitLab docs pattern recognition. MEDIUM confidence — GitLab AI search in 17.x was in limited availability as of knowledge cutoff; exact feature set may have evolved.
- Jira / Atlassian Intelligence: training knowledge. MEDIUM confidence — cloud-only semantic features confirmed as of knowledge cutoff.
- Linear, Notion search: training knowledge. HIGH confidence — both are cloud-native, keyword-based for issue search.
- Elasticsearch/OpenSearch hybrid search (RRF): training knowledge. HIGH confidence — Reciprocal Rank Fusion documented in ES 8.x.
- SPLADE / sparse vector models: training knowledge. HIGH confidence — SPLADE v2 and BM25 sparse vectors are established techniques.
- Apache Tika text extraction: training knowledge. HIGH confidence — mature, widely deployed.

**Confidence gaps:**
- GitLab 17.x+ semantic search exact feature parity: LOW — could not verify current state.
- Qdrant sparse vector performance at 500k+ vectors in production: MEDIUM — benchmarks in training data but cannot verify latest release characteristics.
- Redmine webhook plugin ecosystem current state: LOW — varies by Redmine version, could not verify.

---

*Feature research for: Semantic search infrastructure for Redmine (RSS)*
*Researched: 2026-02-18*
