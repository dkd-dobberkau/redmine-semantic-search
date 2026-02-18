# Architecture Research

**Domain:** Semantic search infrastructure (Go indexer + Qdrant + embedding service)
**Researched:** 2026-02-18
**Confidence:** HIGH — Architecture derived directly from the project requirements document and well-established semantic search system patterns. The stack choices (Go, Qdrant gRPC client, Hugging Face TEI, Apache Tika) are all mature and their integration patterns are well documented in their respective official sources.

---

## Standard Architecture

### System Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                        INDEXING LAYER                                │
│                                                                      │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐             │
│  │ Redmine      │   │ Text         │   │ Batch        │             │
│  │ API Client   │──▶│ Extractor    │──▶│ Embedder     │             │
│  │ (polling)    │   │ (Tika)       │   │ (TEI / OAI)  │             │
│  └──────────────┘   └──────────────┘   └──────┬───────┘             │
│         │                                      │                     │
│         ▼                                      ▼                     │
│  ┌──────────────┐                     ┌──────────────┐              │
│  │ Sync State   │                     │ Qdrant Upsert│              │
│  │ (last_sync   │                     │ (gRPC batch) │              │
│  │  timestamp)  │                     └──────┬───────┘              │
│  └──────────────┘                            │                       │
│                                              │                       │
├──────────────────────────────────────────────▼───────────────────────┤
│                        STORAGE LAYER                                 │
│                                                                      │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │                         Qdrant                                  │  │
│  │   Collection: redmine_search                                    │  │
│  │   ┌───────────────────────┐  ┌──────────────────────────────┐  │  │
│  │   │ Dense Vectors         │  │ Sparse Vectors (SPLADE/BM25) │  │  │
│  │   │ (cosine, dim 384–1536)│  │ (hybrid search)              │  │  │
│  │   └───────────────────────┘  └──────────────────────────────┘  │  │
│  │   Payload indices: project_id, content_type, tracker,           │  │
│  │   status, author, created_on, updated_on, redmine_id            │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                                                      │
├──────────────────────────────────────────────────────────────────────┤
│                        SEARCH LAYER                                  │
│                                                                      │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐             │
│  │ Auth         │   │ Permission   │   │ Query        │             │
│  │ Middleware   │──▶│ Resolver     │──▶│ Embedder     │             │
│  │ (API key /   │   │ (project_ids │   │ (same iface  │             │
│  │  session)    │   │  + cache)    │   │  as indexer) │             │
│  └──────────────┘   └──────────────┘   └──────┬───────┘             │
│                                               │                      │
│                                               ▼                      │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐             │
│  │ Response     │   │ Post-Filter  │   │ Qdrant       │             │
│  │ Formatter    │◀──│ (private     │◀──│ Search       │             │
│  │ (snippet,    │   │  issues etc) │   │ (pre-filter  │             │
│  │  pagination) │   └──────────────┘   │  project_ids)│             │
│  └──────────────┘                      └──────────────┘             │
│                                                                      │
│  HTTP Endpoints:                                                     │
│    GET  /api/v1/search                                               │
│    GET  /api/v1/similar/{content_type}/{id}                          │
│    GET  /api/v1/health                                               │
│    POST /api/v1/admin/reindex  (protected)                           │
└──────────────────────────────────────────────────────────────────────┘

External services (sidecar containers):
  [Redmine] ◀── REST/JSON ──▶ [Indexer]
  [Apache Tika] ◀── REST ──▶ [Indexer / extractor package]
  [Embedding Service (TEI)] ◀── REST/HTTP ──▶ [Indexer + Search API]
  [Qdrant] ◀── gRPC ──▶ [Indexer + Search API]
```

### Component Responsibilities

| Component | Responsibility | Communicates With |
|-----------|----------------|-------------------|
| **Redmine API Client** (`internal/redmine`) | Paginated REST polling of issues, wikis, journals, documents; incremental sync via `updated_on`; attachment download | Redmine REST API (outbound HTTP) |
| **Text Extractor** (`internal/extractor`) | Binary attachment text extraction (PDF, DOCX, ODT) via Tika REST API | Apache Tika sidecar (outbound HTTP) |
| **Embedder** (`internal/embedder`) | Converts text strings to dense float vectors; interface-backed so OpenAI and local TEI are interchangeable; sparse vector generation for hybrid search | Embedding service (outbound HTTP) |
| **Indexer Orchestrator** (`internal/indexer`) | Drives the full pipeline: fetch → extract → chunk → embed → upsert; manages batch sizing, retry/backoff, worker concurrency, sync state persistence | All internal packages |
| **Qdrant Client Wrapper** (`internal/qdrant`) | gRPC batch upsert, collection and alias management, blue-green reindex swap | Qdrant gRPC port 6334 |
| **Config** (`internal/config`) | Loads YAML + env vars via Viper; validates and exposes typed config struct | Used by all packages |
| **Metrics** (`internal/metrics`) | Prometheus counters and histograms: indexing rate, embedding latency, search latency, queue depth, error rate | Scraped by Prometheus; used by all packages |
| **HTTP Server** (`api/server`) | Starts chi/stdlib HTTP server, wires middleware and handlers, manages graceful shutdown | OS signals; handlers and middleware |
| **Handlers** (`api/handlers`) | Parses search request, calls permission resolver, calls embedder, calls Qdrant search, formats response | Permission resolver, Embedder, Qdrant client |
| **Auth Middleware** (`api/middleware`) | Validates Redmine API key or session token on every request | Redmine REST API (token validation) |
| **Permission Resolver** | Fetches and caches allowed `project_ids` for authenticated user; TTL-based cache | Redmine REST API (outbound); in-process cache |
| **Qdrant (storage)** | Stores dense + sparse vectors with structured payload; serves ANN queries with payload pre-filtering | Indexer and Search API (gRPC) |
| **Apache Tika (sidecar)** | Extracts plain text from binary documents | Extractor package (REST) |
| **Embedding Service (sidecar)** | Serves sentence-transformer model as REST API; handles batching and GPU acceleration | Embedder package (REST) |

---

## Recommended Project Structure

This matches the structure defined in the requirements document. Rationale is added per module.

```
redmine-semantic-search/
├── cmd/
│   └── indexer/
│       └── main.go              # Entry point: parse CLI flags, wire deps, start scheduler
├── internal/
│   ├── config/
│   │   └── config.go            # Single typed Config struct; loaded once at startup
│   ├── redmine/
│   │   ├── client.go            # Base HTTP client, auth header, rate-limit handling
│   │   ├── issues.go            # Paginated issue fetching with updated_on filter
│   │   ├── wikis.go             # Wiki page fetching per project
│   │   └── models.go            # Go structs matching Redmine JSON responses
│   ├── embedder/
│   │   ├── embedder.go          # Interface: Embed(ctx, []string) ([][]float32, error)
│   │   ├── openai.go            # OpenAI text-embedding-3-small implementation
│   │   └── local.go             # Hugging Face TEI / FastAPI implementation
│   ├── extractor/
│   │   └── tika.go              # POST binary → Tika, return plain text
│   ├── indexer/
│   │   ├── indexer.go           # Top-level pipeline orchestration per content type
│   │   ├── batch.go             # Worker pool, batch accumulation, retry with backoff
│   │   └── sync.go              # Read/write last_sync timestamp; full vs incremental mode
│   ├── qdrant/
│   │   ├── client.go            # gRPC connection pool wrapper; upsert, search, delete
│   │   └── collection.go        # Collection creation, alias management for blue-green reindex
│   └── metrics/
│       └── prometheus.go        # Register and expose Prometheus metrics
├── api/
│   ├── server.go                # HTTP server setup, graceful shutdown handler
│   ├── handlers.go              # /search, /similar, /health, /admin/reindex handlers
│   └── middleware.go            # Auth validation, structured request logging, CORS
├── deployments/
│   ├── docker-compose.yml       # All services: qdrant, embedding, tika, indexer
│   ├── Dockerfile               # Multi-stage: builder + slim runtime, non-root user
│   └── config.example.yml       # Documented configuration template
└── go.mod
```

### Structure Rationale

- **`cmd/indexer/`:** A single binary serves both the indexer scheduler and the search API HTTP server. This reduces operational surface area for small deployments. The entrypoint wires all dependencies via manual dependency injection — no DI container needed in Go.
- **`internal/`:** Go's `internal` package rule enforces that these packages are not importable from outside the module. This makes the boundary between library code and binary explicit.
- **`internal/embedder/` (interface):** The `Embedder` interface is the critical seam. Both the indexer pipeline and the search handler use the same interface. Swapping from local TEI to OpenAI requires only a config change, not code changes.
- **`internal/qdrant/`:** A thin wrapper over the official `github.com/qdrant/go-client` gRPC client. It handles connection management and exposes domain-specific methods (upsert documents, search with filter, manage aliases) rather than raw gRPC calls.
- **`api/`:** Outside `internal/` intentionally — the HTTP API is a separate concern from the indexer logic, even though they share the same binary.
- **`deployments/`:** All deployment artifacts co-located. The Dockerfile uses multi-stage builds to produce a ~20 MB final image.

---

## Architectural Patterns

### Pattern 1: Embedder Interface (Dependency Inversion)

**What:** Define a single Go interface for embedding. All callers depend on the interface, not on a concrete implementation. Two implementations (OpenAI, local TEI) satisfy the same interface.

**When to use:** Any time an external service may need to be swapped. This is the primary extensibility point in the system.

**Trade-offs:** Adds a thin abstraction layer. The benefit is that switching embedding providers requires zero changes to calling code; the cost is negligible (one interface method per call).

**Example:**
```go
// internal/embedder/embedder.go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// internal/embedder/local.go
type LocalEmbedder struct {
    baseURL    string
    httpClient *http.Client
}

func (e *LocalEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
    // POST to TEI /embed endpoint
}

// internal/embedder/openai.go
type OpenAIEmbedder struct {
    apiKey string
    model  string
    client *http.Client
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
    // POST to OpenAI /embeddings endpoint
}
```

### Pattern 2: Pipeline with Worker Pool and Batching

**What:** The indexer orchestrates a producer-consumer pipeline. A producer goroutine fetches Redmine objects and writes them to a channel. A configurable pool of worker goroutines reads from the channel, applies text extraction and embedding in batches, then writes to Qdrant.

**When to use:** Any time throughput is constrained by an external service with batching support (embedding APIs, Qdrant upsert). Batch sizes for embedding (32 texts per call) and Qdrant upsert (100 points per call) should be tuned separately.

**Trade-offs:** Improves throughput significantly (parallelism + batching). Adds complexity around error handling — a batch failure must not silently drop documents. Implement per-batch retry with exponential backoff before propagating failures.

**Example:**
```go
// internal/indexer/batch.go
func (b *BatchWorker) Run(ctx context.Context, in <-chan Document) error {
    batch := make([]Document, 0, b.batchSize)
    for {
        select {
        case doc, ok := <-in:
            if !ok {
                return b.flush(ctx, batch) // drain remaining
            }
            batch = append(batch, doc)
            if len(batch) >= b.batchSize {
                if err := b.flush(ctx, batch); err != nil {
                    return err
                }
                batch = batch[:0]
            }
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

### Pattern 3: Two-Phase Permission Filtering (Pre-filter + Post-filter)

**What:** On every search request, resolve the set of `project_ids` the authenticated user may access. Pass these as a `must` filter to Qdrant so non-authorized vectors are never returned as candidates. Apply a secondary post-filter pass for finer-grained permissions (private issues) using oversampling.

**When to use:** Any system where the data store cannot enforce business-layer authorization natively. This is the standard approach for multi-tenant search over Qdrant.

**Trade-offs:** The pre-filter requires a Qdrant payload index on `project_id` — without the index, Qdrant scans all points. The post-filter requires fetching more results than delivered (oversampling factor, default 2×), which is a small overhead. The permission cache (5-minute TTL) reduces Redmine API load but means a permission change may take up to 5 minutes to propagate.

**Example:**
```go
// api/handlers.go (simplified)
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
    user := middleware.UserFromContext(r.Context())
    allowedProjectIDs, err := h.permissions.Resolve(r.Context(), user)
    // ...
    queryVec, err := h.embedder.Embed(r.Context(), []string{query})
    // ...
    results, err := h.qdrant.Search(r.Context(), qdrant.SearchRequest{
        Vector:    queryVec[0],
        Filter:    qdrant.MustFilter("project_id", allowedProjectIDs),
        Limit:     limit * oversamplingFactor,
        WithPayload: true,
    })
    results = postFilter(results, user) // private-issue check
    // paginate and format
}
```

### Pattern 4: Qdrant Collection Alias for Blue-Green Reindex

**What:** Maintain a stable alias `redmine_search` pointing to the active collection. During full reindex, write to a new collection (`redmine_search_v2`), then atomically swap the alias. The old collection is deleted after a configurable grace period.

**When to use:** Any time a full reindex must not interrupt in-flight search queries. This is the standard pattern for zero-downtime reindexing in Qdrant.

**Trade-offs:** Requires 2× storage during the swap window. The alias swap in Qdrant is atomic. The grace period before deleting the old collection allows in-flight reads to complete.

---

## Data Flow

### Indexing Pipeline (incremental and full)

```
Redmine REST API
    │
    │  (paginated, filtered by updated_on)
    ▼
[redmine.Client] — fetch issues/wikis/journals/documents
    │
    │  (raw Redmine objects)
    ▼
[indexer.Orchestrator] — fan out per content type
    │
    ├──▶ [extractor.TikaClient] — for attachments only
    │         │
    │         │  (plain text)
    │         ▼
    │    [text assembly] — title + body + custom fields → single string
    │
    │  (assembled text strings, batched)
    ▼
[embedder.Embedder] — POST to TEI or OpenAI
    │
    │  ([]float32 dense vectors, optional sparse vectors)
    ▼
[qdrant.Client] — BatchUpsert with payload
    │
    │  (point ID = deterministic hash of content_type + redmine_id)
    ▼
[Qdrant collection] — persisted vectors + payload
    │
    ▼
[indexer.SyncState] — persist last_sync timestamp
```

### Search Request Flow

```
HTTP Client (Redmine frontend / user)
    │
    │  GET /api/v1/search?q=...&project_id=...
    ▼
[api/middleware: Auth] — validate Redmine API key / session
    │
    ▼
[api/middleware: Logging + Metrics]
    │
    ▼
[api/handlers: SearchHandler]
    │
    ├──▶ [Permission Resolver] — fetch allowed project_ids from cache or Redmine API
    │
    ├──▶ [embedder.Embedder] — vectorize query string
    │
    ├──▶ [qdrant.Client Search] — ANN search with project_id pre-filter + optional
    │         facet filters (tracker, status, date_range, content_type)
    │
    ├──▶ [Post-filter] — remove private issues user cannot see
    │
    ├──▶ [Snippet assembler] — extract relevant text preview from payload
    │
    ▼
JSON response: {total, results[{redmine_id, score, snippet, url, ...}]}
```

### Key Data Flows Summary

1. **Incremental sync:** Scheduler triggers every N minutes → Redmine client fetches objects with `updated_on >= last_sync` → pipeline processes and upserts → timestamp advanced. No reprocessing of unchanged documents.
2. **Full reindex:** Write to new collection (alias-backed) → pipeline processes all Redmine content in priority order (issues first, then wikis, journals, documents) → alias swap → old collection dropped.
3. **Delete sync:** Periodic reconciliation compares Redmine `GET /issues.json?limit=all&only_ids=true` ID set against Qdrant payload index → orphaned points deleted. Alternatively, triggered by Redmine webhook plugin.
4. **Permission resolution:** On search request → check in-process LRU cache keyed by user API key → on miss, call `GET /redmine/my/account` + `GET /projects.json` → populate cache with allowed IDs + TTL.
5. **Chunking flow:** Long documents (> 512 tokens) → split into overlapping chunks (512 tokens, 50 overlap) → each chunk gets its own Qdrant point with `parent_id` payload → search results deduplicated by `parent_id`, highest chunk score wins.

---

## Build Order (Component Dependencies)

The following order minimizes blocked work and allows early end-to-end testing:

```
Phase 1 — Foundation (no external dependencies beyond config)
  └── internal/config         ← needed by everything
  └── internal/metrics        ← needed by everything
  └── internal/embedder       ← interface only, then local implementation

Phase 2 — Storage client (depends on Qdrant running)
  └── internal/qdrant/client.go      ← upsert + search
  └── internal/qdrant/collection.go  ← collection + alias management

Phase 3 — Data acquisition (depends on Redmine access + config)
  └── internal/redmine/models.go     ← structs first (no deps)
  └── internal/redmine/client.go
  └── internal/redmine/issues.go     ← issues first for MVP
  └── internal/redmine/wikis.go

Phase 4 — Indexer pipeline (depends on phases 1-3)
  └── internal/indexer/sync.go       ← sync state before orchestration
  └── internal/indexer/batch.go      ← batch worker before orchestrator
  └── internal/indexer/indexer.go    ← wires all internal packages
  └── internal/extractor/tika.go     ← can be deferred (documents only)

Phase 5 — Search API (depends on phases 1-2, shares embedder)
  └── api/middleware.go   ← auth + logging before handlers
  └── api/handlers.go
  └── api/server.go

Phase 6 — Hardening
  └── Permission caching
  └── Blue-green alias management
  └── Hybrid search (sparse vectors)
  └── Snippet generation
```

**Critical dependency:** The embedder interface (Phase 1) must be defined before Phase 4 or Phase 5 begins, because both the indexer and the search handler depend on it. Defining the interface first allows parallel development of the indexer pipeline and the search API.

---

## Anti-Patterns

### Anti-Pattern 1: Fetching Individual Embeddings Per Document

**What people do:** Call the embedding API once per document inside the fetch loop, accumulating latency for each HTTP round-trip.

**Why it's wrong:** For 100,000 documents, individual calls at 50 ms each = 83 minutes of embedding time. Most embedding APIs and local TEI support batch requests of 32–512 texts per call, reducing total time by 30-100×.

**Do this instead:** Accumulate documents in a batch buffer, send when the batch reaches the configured size or a flush timer fires. The `internal/indexer/batch.go` module owns this responsibility.

### Anti-Pattern 2: Skipping Payload Indices on Qdrant

**What people do:** Store payload fields (project_id, tracker, status) without creating Qdrant payload indices, then rely on post-ANN filtering.

**Why it's wrong:** Without payload indices, Qdrant must scan every candidate point to evaluate filter predicates. With indices, Qdrant can prune the search space before ANN traversal, dramatically improving latency under filtering (which is the default case for permission-aware search).

**Do this instead:** Create payload indices for every field that appears in filters at collection creation time: `project_id` (integer), `content_type` (keyword), `tracker` (keyword), `status` (keyword), `author` (keyword), `created_on` (datetime), `updated_on` (datetime). This is done in `internal/qdrant/collection.go` during collection setup.

### Anti-Pattern 3: Storing Permission Logic in the Vector Database

**What people do:** Attempt to encode user roles or ACLs as Qdrant payload fields and handle all authorization inside the filter.

**Why it's wrong:** Redmine's permission model is dynamic (roles, groups, project memberships change frequently) and Qdrant is not an authorization store. Keeping stale ACL data in Qdrant risks serving unauthorized results and creates a synchronization problem.

**Do this instead:** Use Qdrant only for the coarse pre-filter (`project_id` membership list derived from Redmine at query time). Keep the authoritative authorization check in the Search API, which calls Redmine or a short-TTL cache for fresh membership data.

### Anti-Pattern 4: Reindexing In-Place (No Blue-Green Swap)

**What people do:** Delete all points from the active collection, then re-populate. Searches during this window return empty or partial results.

**Why it's wrong:** A full reindex of 100,000 documents takes 20-30 minutes. Returning empty search results for 30 minutes is unacceptable.

**Do this instead:** Write the new index to a shadow collection, then perform an atomic Qdrant alias swap (single API call, zero downtime). The old collection serves reads until the alias is switched.

### Anti-Pattern 5: Ignoring Text Length Before Embedding

**What people do:** Pass full document text directly to the embedding model, truncating silently at the model's token limit.

**Why it's wrong:** A long wiki page truncated at 512 tokens loses the majority of its content. The resulting vector represents only the introduction, not the full document. Searches for content in the body will miss the document.

**Do this instead:** Implement chunking for any content exceeding the model's token limit. Store chunks as separate Qdrant points with a `parent_id` payload field. At search time, deduplicate by `parent_id` and return the highest-scoring chunk. The chunking parameters (512 tokens, 50-token overlap) should be validated against real Redmine data.

---

## Integration Points

### External Services

| Service | Integration Pattern | Protocol | Notes |
|---------|---------------------|----------|-------|
| Redmine REST API | Pull-based polling (indexer), token validation (search API) | HTTPS/REST JSON | Rate-limit: use configurable batch_size (default 100); handle 429 with backoff |
| Apache Tika | Sidecar container, synchronous request per attachment | HTTP/REST (multipart) | Default port 9998; configure 30s timeout; only needed for document content type |
| Hugging Face TEI / FastAPI embedding service | Batched POST requests from embedder package | HTTP/REST JSON | Default port 8080; shared by indexer and search API; critical path for search latency |
| Qdrant | gRPC (port 6334) for data plane; HTTP (port 6333) for admin | gRPC / HTTP REST | Use official `github.com/qdrant/go-client`; prefer gRPC for throughput; HTTP for health checks |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| `cmd/indexer` ↔ `internal/*` | Direct function calls (same binary) | Dependency injection via constructor functions; no shared global state |
| `api/handlers` ↔ `internal/embedder` | Shared `Embedder` interface instance | Same interface used by indexer and search handler; one instance per binary |
| `api/handlers` ↔ `internal/qdrant` | Shared `QdrantClient` instance | Connection pool is reused across handlers and indexer goroutines |
| `internal/indexer` ↔ `internal/qdrant` | Direct method calls | The Qdrant client is injected into the indexer orchestrator at startup |
| Indexer goroutines ↔ batch worker | Go channels (`chan Document`) | Bounded channel prevents memory accumulation if Qdrant is slow |
| Permission resolver ↔ Redmine API | HTTP + in-process LRU cache | Cache keyed by user API key; 5-minute TTL; invalidation on 403 from Qdrant post-filter |

---

## Scaling Considerations

| Scale | Architecture Adjustments |
|-------|--------------------------|
| < 50k vectors, single Redmine instance | Docker Compose, single Go binary, local TEI model (MiniLM-L6-v2 384d), Qdrant in-memory or on-disk. No changes to architecture needed. |
| 50k–500k vectors | Enable Qdrant on-disk storage (`on_disk: true`), tune `hnsw_config` (m=16, ef=100). Increase indexer worker count. Monitor embedding service as the likely bottleneck. |
| 500k+ vectors / multi-Redmine | Qdrant cluster (Kubernetes, sharded collection), separate indexer and search API deployments, dedicated GPU node for embedding service, permission cache backed by Redis instead of in-process LRU. |

### Scaling Priorities

1. **First bottleneck:** Embedding service throughput. A single CPU-based TEI instance processes ~200 texts/second at batch size 32. For initial indexing of 100k documents, this is ~8 minutes — acceptable. If faster indexing is needed, add GPU or horizontal TEI instances (the `Embedder` interface supports this via load-balanced URL).
2. **Second bottleneck:** Qdrant ANN search latency under filtering. This is addressed by payload indices at creation time. HNSW parameters (`m` and `ef_construct`) trade index build time against query latency and recall — the defaults are adequate for 500k vectors.

---

## Sources

- Project requirements document: `/Users/olivier/Versioncontrol/local/redmine-semantic-search/redmine-semantic-search-requirements.md` — PRIMARY SOURCE (HIGH confidence). All component names, data model, configuration schema, and API endpoints are taken directly from this document.
- Qdrant architecture concepts: Established pattern from Qdrant documentation (payload indices, collection aliases, named vectors for hybrid search). Confidence: HIGH — these are core Qdrant features documented in the official Qdrant docs (https://qdrant.tech/documentation/concepts/).
- Go pipeline patterns (channels, worker pools, interface-based DI): Standard Go concurrency idioms from "The Go Programming Language" and official Go blog. Confidence: HIGH.
- Hugging Face Text Embeddings Inference (TEI): Established self-hosted embedding server. Confidence: HIGH — in production use at multiple organizations as of 2025.
- Apache Tika 2.x REST API: Established document extraction tool. Confidence: HIGH.
- Two-phase permission filtering (pre-filter + post-filter with oversampling): Industry-standard pattern for multi-tenant vector search. Confidence: HIGH — used by Elasticsearch, Qdrant, and Weaviate deployments.
- Blue-green alias swap: Documented Qdrant pattern for zero-downtime reindexing. Confidence: HIGH.

---

*Architecture research for: Redmine Semantic Search (RSS) — Go indexer + Qdrant + embedding service*
*Researched: 2026-02-18*
