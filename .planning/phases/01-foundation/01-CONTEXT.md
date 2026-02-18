# Phase 1: Foundation - Context

**Gathered:** 2026-02-18
**Status:** Ready for planning

<domain>
## Phase Boundary

Go module setup, Docker Compose stack with all services (Qdrant, TEI embedding, Go binary), Embedder interface with TEI implementation, Qdrant collection with all payload indexes and alias setup, and embedding model benchmark to validate multilingual-e5-base on DE/EN content. No indexing pipeline, no search API, no Redmine interaction.

</domain>

<decisions>
## Implementation Decisions

### Embedding Model
- Default model: `intfloat/multilingual-e5-base` (768 dimensions) via Hugging Face TEI
- Only local TEI implementation in Phase 1 — no OpenAI embedder yet (interface supports adding it later)
- e5 prefix handling (`query: ` / `passage: `): Claude's discretion on where to handle this in the Embedder interface

### Embedding Benchmark
- Claude's discretion on benchmark approach (synthetic test set or real data export)
- Must validate DE/EN recall before vector dimensionality (768d) is committed to production
- Benchmark result determines go/no-go for model choice before Phase 2 begins

### Config & Environment
- YAML primary with env var overrides (viper standard pattern)
- Config file location: `./config.yml` next to the binary
- Fail fast on startup: validate all required fields (REDMINE_URL, REDMINE_API_KEY, QDRANT_HOST, EMBEDDING_URL) at boot, exit with clear error listing missing values
- Ship `config.example.yml` with full comments explaining each option and its default value

### Docker Compose Stack
- Service naming prefix: `redmine-search-*` (redmine-search-qdrant, redmine-search-embedding, redmine-search-indexer)
- Standard ports: 6333/6334 for Qdrant, 8080 for TEI, 8090 for API
- Qdrant data: named Docker volume (`qdrant_data`)
- Go binary: multi-stage Dockerfile for CI/prod, local build option for dev iteration (both supported)
- Custom Docker network for service isolation

### Qdrant Collection
- Collection name: `redmine_search` (as in requirements doc)
- Aliases from day 1: alias `redmine_search` pointing to `redmine_search_v1` — ready for blue-green reindex in Phase 3
- Point IDs: UUID v5 (SHA1-based) from `{content_type}:{redmine_id}` — deterministic for upsert idempotency
- Storage: on-disk with mmap (optimized for 500k vector target, lower RAM usage)
- Distance metric: Cosine
- All payload indexes created at collection init: `project_id`, `content_type`, `tracker`, `status`, `author`, `created_on`, `updated_on`

### Claude's Discretion
- Benchmark methodology (synthetic vs real data, evaluation metrics)
- e5 prefix handling strategy in Embedder interface
- Exact Go project layout and module naming convention
- TEI Docker image tag selection (verify current CPU tag)
- Health check configuration for Docker services

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches. The requirements document already specifies the module structure (`cmd/indexer`, `internal/{config,embedder,qdrant,...}`, `api/`).

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 01-foundation*
*Context gathered: 2026-02-18*
