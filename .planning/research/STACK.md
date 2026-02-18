# Stack Research

**Domain:** Semantic search infrastructure — Go indexer + Qdrant vector database + embedding service
**Researched:** 2026-02-18
**Confidence:** MEDIUM
**Note on verification:** WebSearch and WebFetch were unavailable during this research session. Version numbers are sourced from training data (cutoff August 2025) and must be verified against official releases before use. Architecture recommendations and rationale are HIGH confidence; specific version pins are LOW confidence and flagged below.

---

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended | Confidence |
|------------|---------|---------|-----------------|------------|
| Go | 1.22+ (use 1.23.x latest stable) | Primary language — indexer, search API | Single binary deployment, first-class goroutine concurrency for parallel API calls and batch processing, stdlib covers HTTP + slog + context without extra deps. Already constrained in requirements. | HIGH |
| Qdrant | 1.9.x (latest stable) | Vector database | Native Payload Filters for project-based permission pre-filtering; named vector support for dense+sparse hybrid search in a single collection; gRPC and REST; on-disk storage for >100k vector deployments; active development with production track record | MEDIUM — verify latest stable at hub.docker.com/r/qdrant/qdrant |
| github.com/qdrant/go-client | v1.x (latest) | Qdrant gRPC client from Go | Official Qdrant-maintained client; gRPC-based (low latency, streaming); uses generated protobuf types matching Qdrant server API exactly. No viable community alternatives with comparable coverage. | MEDIUM — verify at github.com/qdrant/go-client/releases |
| Hugging Face TEI (text-embeddings-inference) | latest | Local embedding service | Purpose-built inference server for embedding models; ships as Docker image; OpenAI-compatible `/embed` endpoint so the Go client needs no special SDK; supports all sentence-transformers models; GPU-optional (CPU works for <50ms latency target at batch=32) | MEDIUM — verify at github.com/huggingface/text-embeddings-inference |
| Apache Tika | 2.x (latest 2.x stable) | Text extraction from PDF/DOCX/ODT | Industry-standard; REST API over HTTP from any language; Docker image available; handles 1000+ file formats; already constrained in requirements. Tika 3.x is GA as of late 2024 — check if Docker image is stable | MEDIUM — verify latest at hub.docker.com/r/apache/tika |

### Go Libraries

| Library | Version | Purpose | When to Use | Confidence |
|---------|---------|---------|-------------|------------|
| github.com/qdrant/go-client | v1.x | Qdrant gRPC wire protocol | Always — it is the only official Go client | MEDIUM |
| github.com/spf13/viper | v1.18.x | Configuration — YAML file + env var overlay | Use for the config subsystem; supports `REDMINE_API_KEY` env override over YAML defaults; hot-reload possible but not needed for v1 | MEDIUM — verify at github.com/spf13/viper/releases |
| github.com/robfig/cron/v3 | v3.0.x | Cron scheduling for index intervals | Use for `full_reindex_cron` (e.g. `0 2 * * 0`); v3 is the current stable major with context support | HIGH — API stable since 2019, v3 is current |
| github.com/prometheus/client_golang | v1.19.x | Prometheus metrics export | Use for `/metrics` endpoint; wrap with `promhttp.Handler()`; define custom counters for docs indexed, search latency histograms, queue depth | MEDIUM — verify at github.com/prometheus/client_golang/releases |
| golang.org/x/sync/errgroup | stdlib-adjacent (x/sync) | Bounded goroutine pools for parallel fetch | Use in the indexer pipeline worker pool to limit concurrency; pairs with `semaphore.Weighted` for rate control | HIGH — part of golang.org/x/sync, stable API |
| github.com/cenkalti/backoff/v4 | v4.x | Exponential backoff for retry logic | Use for Qdrant upsert retries, Redmine API retries, embedding service retries; provides jitter; v4 is current | HIGH — stable, widely used |
| log/slog | stdlib (Go 1.21+) | Structured JSON logging | Use directly; no external library needed; `slog.New(slog.NewJSONHandler(os.Stdout, opts))` produces JSON logs compatible with any log aggregator | HIGH — stdlib since Go 1.21 |
| net/http | stdlib | HTTP client for Redmine REST API + TEI embedding | Use stdlib; add `http.Client` with explicit `Timeout` and custom `Transport` for connection pooling | HIGH — stdlib |
| encoding/json | stdlib | JSON serialization for API responses | Use stdlib; avoid third-party JSON libs unless profiling shows stdlib bottleneck (unlikely at this scale) | HIGH — stdlib |
| github.com/go-chi/chi/v5 | v5.x | HTTP router for search API | Lightweight, stdlib-compatible, no reflection magic; good middleware story (request ID, logging, CORS, recovery); preferred over gorilla/mux (archived) and Gin (heavier) for a service with 4-6 routes | HIGH — active, well maintained |
| github.com/stretchr/testify | v1.9.x | Test assertions | Use `require` + `assert` packages; pairs with `net/http/httptest` for handler testing | HIGH — de-facto standard |

### Embedding Model Recommendations

| Model | Dimensions | Language | Use Case | Hosting | Confidence |
|-------|------------|---------|---------|---------|------------|
| `intfloat/multilingual-e5-base` | 768 | 100 languages incl. DE+EN | **Recommended default** — handles German + English Redmine instances; good balance of quality and size | TEI | HIGH |
| `intfloat/multilingual-e5-large` | 1024 | 100 languages incl. DE+EN | Higher quality for large corpora (>200k docs), costs ~2x RAM | TEI | HIGH |
| `sentence-transformers/all-MiniLM-L6-v2` | 384 | English only | Use only for English-only instances where speed is paramount; 6x faster than multilingual-e5-base | TEI | HIGH |
| `deepset/gbert-base` | 768 | German | Use for German-only instances; outperforms multilingual on pure-German corpora | TEI | MEDIUM |
| `openai/text-embedding-3-small` | 1536 (configurable 512-1536) | Multilingual | Cloud fallback when GPU unavailable; ~$0.02/1M tokens; data leaves the network | OpenAI API | HIGH |
| `openai/text-embedding-3-large` | 3072 (configurable) | Multilingual | Highest quality cloud option; 3x cost of 3-small | OpenAI API | HIGH |

**For sparse vectors (hybrid search):** Use `prithivida/Splade_PP_en_v1` for English or `naver/splade-v3` — both supported by TEI. Alternatively, use Qdrant's built-in BM25 sparse vector support (as of Qdrant 1.7+) which requires no external model.

### Development Tools

| Tool | Purpose | Notes | Confidence |
|------|---------|-------|------------|
| Docker Compose v2 | Local development orchestration | Use `docker compose` (plugin, not `docker-compose` v1); no `version:` field in compose file | HIGH |
| golangci-lint | Static analysis and linting | Run `errcheck`, `govet`, `staticcheck`, `exhaustive`; configure `.golangci.yml` | HIGH |
| go test -race | Race condition detection | Always run with `-race` in CI; critical for goroutine-heavy indexer code | HIGH |
| ko (google/ko) | Build Go container images | Alternative to Dockerfile for pure-Go services; produces minimal images without Docker daemon; optional | MEDIUM |
| Makefile | Build task runner | Use for `make test`, `make lint`, `make docker-build`; keeps CI and local commands aligned | HIGH |

---

## Installation

```bash
# Initialize module
go mod init github.com/yourorg/redmine-semantic-search

# Core client libraries
go get github.com/qdrant/go-client@latest
go get github.com/spf13/viper@latest
go get github.com/robfig/cron/v3@latest
go get github.com/prometheus/client_golang/prometheus@latest
go get github.com/prometheus/client_golang/prometheus/promhttp@latest

# HTTP router
go get github.com/go-chi/chi/v5@latest

# Retry logic
go get github.com/cenkalti/backoff/v4@latest

# Goroutine utilities
go get golang.org/x/sync@latest

# Test utilities
go get github.com/stretchr/testify@latest
```

---

## Alternatives Considered

| Recommended | Alternative | Why Not | When Alternative Is Better |
|-------------|-------------|---------|---------------------------|
| github.com/go-chi/chi/v5 | github.com/gin-gonic/gin | Gin brings reflection-based binding, middleware ecosystem is heavier; overkill for a 4-6 route API | Use Gin if you need automatic request validation, or if the team already uses Gin across services |
| github.com/go-chi/chi/v5 | github.com/gorilla/mux | gorilla/mux is archived (no new releases); security/compat issues may emerge | Never — mux is archived |
| github.com/go-chi/chi/v5 | net/http ServeMux (stdlib) | Go 1.22 ServeMux supports method routing and path params — viable for very simple APIs; lacks middleware chaining | Use stdlib ServeMux for ≤3 routes with no middleware |
| github.com/cenkalti/backoff/v4 | github.com/avast/retry-go | retry-go is simpler API but less control over jitter and max elapsed time | Use retry-go for simple cases without timing requirements |
| github.com/spf13/viper | github.com/knadh/koanf | koanf is more modular and has fewer deps; viper has larger ecosystem and YAML+env is the stated requirement | Use koanf for new projects where deps are a concern; viper is safe here |
| Hugging Face TEI | Ollama | Ollama is optimized for LLMs with chat interfaces, not embedding throughput; limited batching | Use Ollama only for casual local development; TEI is production-grade |
| Hugging Face TEI | Custom FastAPI + sentence-transformers | Python FastAPI is easy to build but slow for batch inference vs. Rust-based TEI; maintenance burden | Use FastAPI if TEI does not support your specific model |
| intfloat/multilingual-e5-base | sentence-transformers/paraphrase-multilingual-mpnet-base-v2 | Slightly lower benchmark scores on MTEB; e5-base is current SOTA for retrieval | Use mpnet if e5-base is unavailable |
| Qdrant | Weaviate | Weaviate has its own module system but heavier resource requirements; less transparent Payload filter semantics | Use Weaviate if you need GraphQL API or built-in text vectorization |
| Qdrant | Milvus | Milvus requires Kubernetes for HA deployment; significantly more complex ops than Qdrant; overkill for <1M vectors | Use Milvus at >10M vectors requiring distributed scaling |
| Qdrant | pgvector (PostgreSQL) | pgvector is excellent for SQL-first teams; but ANN performance degrades vs. purpose-built stores at >100k vectors; no native sparse vector support for hybrid search | Use pgvector if the team already has Postgres and the dataset stays <100k vectors |
| Apache Tika | pdfcpu / unidoc | Go-native, no JVM; but limited to PDFs; Tika handles DOCX, ODT, ODS natively | Use Go-native libraries if only PDF extraction is needed and JVM overhead is unacceptable |

---

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| gorilla/mux | Archived repository — no new releases since 2022; no security patches | github.com/go-chi/chi/v5 |
| github.com/sirupsen/logrus | Maintenance mode — author recommends slog; extra dependency for functionality now in stdlib | log/slog (stdlib, Go 1.21+) |
| github.com/uber-go/zap | Excellent library but unnecessary complexity when slog provides structured JSON logging in stdlib; adds 2 deps | log/slog — if you need high-throughput logging beyond slog, use zap, otherwise skip |
| gRPC-only Qdrant without TLS | gRPC on port 6334 without TLS is fine in Docker Compose (private network), but never expose port 6334 externally without TLS | Use Qdrant's API key authentication + TLS termination at reverse proxy (Traefik/nginx) |
| OpenAI embeddings for sensitive data | Redmine content (bug reports, internal wikis) leaves your network; GDPR implications for EU deployments | intfloat/multilingual-e5-base on TEI — self-hosted, zero data egress |
| Chunking with chunk_size > 512 tokens | Most embedding models have 512-token context window; tokens beyond that are silently truncated causing silent quality loss | Respect model-specific max sequence length; MiniLM=256, multilingual-e5-base=512, e5-large=512 |
| docker-compose v1 (standalone binary) | v1 is EOL since May 2024; no longer maintained by Docker | Docker Compose v2 plugin (`docker compose` without hyphen) |
| Storing Redmine API keys in Go source | Credentials in source become secrets in git history | Environment variables via `.env` file (gitignored) or Docker secrets |
| Synchronous embedding calls per document | 1 HTTP call per document at 100 docs/s = 100 req/s to embedding service; TEI supports batch requests | Batch embedding: collect 32-64 documents, single `POST /embed` with array |
| `updated_on` polling without cursor persistence | If the indexer crashes mid-run, it may re-process or skip documents | Persist the cursor (last successful `updated_on` timestamp) to a file or a Qdrant metadata point after each batch |

---

## Stack Patterns by Deployment Variant

**If German-only Redmine instance:**
- Use `deepset/gbert-base` or `deepset/gbert-large` as embedding model
- Dimension: 768
- TEI supports these models directly

**If multilingual Redmine instance (German + English, the common case):**
- Use `intfloat/multilingual-e5-base` as default
- Prepend inputs with `"query: "` for search queries and `"passage: "` for indexed documents (required by e5 models)
- This is a critical implementation detail — e5 models require task-specific prefixes

**If data privacy is paramount and GPU is available:**
- Use `intfloat/multilingual-e5-large` on TEI with GPU
- Achieves <10ms embedding latency at batch=32
- TEI Docker image: `ghcr.io/huggingface/text-embeddings-inference:1.x-gpu`

**If cloud embeddings are acceptable:**
- Use `text-embedding-3-small` (1536 dims, configurable down to 512 for lower cost)
- Implement Embedder interface with OpenAI client via plain `net/http` — no official Go SDK needed for embeddings
- Set dimension to 512 via `dimensions` parameter to reduce Qdrant storage by 66%

**If hybrid search is required (FR-11):**
- Use Qdrant named vectors: `dense` (multilingual-e5) + `sparse` (SPLADE or BM25)
- For BM25 sparse: Use Qdrant's built-in `sparse_idf` vector type (Qdrant ≥1.7) — eliminates need for a second embedding model
- For SPLADE: Use `naver/splade-v3` or `prithivida/Splade_PP_en_v1` on TEI
- Hybrid scoring via Qdrant's `RRF` (Reciprocal Rank Fusion) or `linear_combination` fusion modes

**If full reindex without search interruption is needed (FR-06):**
- Use Qdrant collection aliases: index into `redmine_search_v2`, then `PUT /collections/aliases` to atomically swap `redmine_search` → `redmine_search_v2`
- Delete old collection after alias swap
- Zero downtime: search API always hits alias, not collection name directly

---

## Version Compatibility

| Component | Compatible With | Notes |
|-----------|-----------------|-------|
| qdrant/go-client v1.x | Qdrant server 1.7+ | gRPC API version-aligned; use matching major versions; named vectors and sparse vector support requires Qdrant ≥1.7 |
| Go 1.22 | slog (stdlib) | slog is stable since Go 1.21; no compat issues |
| Go 1.22 | chi/v5 | chi/v5 requires Go 1.13+; fully compatible |
| TEI latest | multilingual-e5-base | TEI supports e5 models natively; CPU-only image works for batch=32 at acceptable latency |
| Tika 2.x | Java 11+ | Tika Docker image bundles Java; no host Java needed |
| Tika 3.x | Java 17+ | Tika 3.x requires Java 17; Docker image handles this |
| viper v1.18+ | Go 1.21+ | viper v1.18 introduced slog support; compatible with Go 1.22 |
| prometheus/client_golang v1.x | Prometheus 2.x+ scrape format | Standard OpenMetrics; no compatibility issues |

---

## Critical Implementation Details

### e5 Model Input Prefixes (Easy to Miss)

`intfloat/multilingual-e5-*` models require task prefixes on all inputs:

```go
// For documents being indexed:
text := "passage: " + document.Content

// For search queries:
query := "query: " + userQuery
```

Omitting these prefixes causes measurably worse retrieval quality. This is documented in the model card but easy to miss.

### Qdrant Point ID Strategy

Qdrant point IDs must be either UUID or uint64. For deterministic IDs from Redmine content:

```go
// Deterministic UUID from content type + Redmine ID
import "github.com/google/uuid"

func pointID(contentType string, redmineID int) string {
    namespace := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8") // fixed namespace
    return uuid.NewSHA1(namespace, []byte(contentType + ":" + strconv.Itoa(redmineID))).String()
}
```

This ensures upsert idempotency: re-indexing the same issue always overwrites the same Qdrant point.

### Qdrant Batch Upsert Sizing

Optimal batch size for Qdrant upserts depends on vector dimensions:
- 384 dims: batch up to 500 points
- 768 dims: batch up to 200 points
- 1536 dims: batch up to 100 points

The requirements doc specifies `batch_size: 100` which is safe for all dimension sizes.

### gRPC Connection Management

Qdrant gRPC connections should be reused across requests:

```go
// Initialize once at startup, reuse across goroutines
conn, err := grpc.NewClient(
    "qdrant-host:6334",
    grpc.WithTransportCredentials(insecure.NewCredentials()), // Docker Compose only
)
client := qdrant.NewPointsClient(conn)
```

Do not create a new connection per request — gRPC connections are expensive to establish.

---

## Sources

- Requirements document `/redmine-semantic-search-requirements.md` — technology choices, architecture, data model (HIGH confidence — project source of truth)
- Training data (cutoff August 2025) — library versions, model recommendations, patterns (MEDIUM confidence — versions must be verified)
- Qdrant documentation (training data) — named vectors, payload filters, alias API, sparse vector support (HIGH confidence on features, MEDIUM on version numbers)
- intfloat/multilingual-e5 model card (training data) — query/passage prefix requirement (HIGH confidence — documented model behavior)
- Go standard library documentation — slog, net/http, context patterns (HIGH confidence — stdlib is stable)

**VERSION VERIFICATION REQUIRED before implementation:**
- `github.com/qdrant/go-client` — check https://github.com/qdrant/go-client/releases
- Qdrant Docker image — check https://hub.docker.com/r/qdrant/qdrant/tags
- `github.com/spf13/viper` — check https://github.com/spf13/viper/releases
- `github.com/prometheus/client_golang` — check https://github.com/prometheus/client_golang/releases
- Hugging Face TEI — check https://github.com/huggingface/text-embeddings-inference/releases
- Apache Tika Docker — check https://hub.docker.com/r/apache/tika/tags

---
*Stack research for: Redmine Semantic Search (RSS) — Go indexer + Qdrant + embedding service*
*Researched: 2026-02-18*
*Confidence: MEDIUM (versions unverified; WebSearch/WebFetch unavailable during session)*
