# Phase 1: Foundation - Research

**Researched:** 2026-02-18
**Domain:** Go module setup, Docker Compose stack, Embedder interface, Qdrant collection init, embedding model benchmark
**Confidence:** HIGH (core APIs and patterns verified against official docs and pkg.go.dev)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Embedding Model**
- Default model: `intfloat/multilingual-e5-base` (768 dimensions) via Hugging Face TEI
- Only local TEI implementation in Phase 1 — no OpenAI embedder yet (interface supports adding it later)
- e5 prefix handling (`query: ` / `passage: `): Claude's discretion on where to handle this in the Embedder interface

**Embedding Benchmark**
- Claude's discretion on benchmark approach (synthetic test set or real data export)
- Must validate DE/EN recall before vector dimensionality (768d) is committed to production
- Benchmark result determines go/no-go for model choice before Phase 2 begins

**Config & Environment**
- YAML primary with env var overrides (viper standard pattern)
- Config file location: `./config.yml` next to the binary
- Fail fast on startup: validate all required fields (REDMINE_URL, REDMINE_API_KEY, QDRANT_HOST, EMBEDDING_URL) at boot, exit with clear error listing missing values
- Ship `config.example.yml` with full comments explaining each option and its default value

**Docker Compose Stack**
- Service naming prefix: `redmine-search-*` (redmine-search-qdrant, redmine-search-embedding, redmine-search-indexer)
- Standard ports: 6333/6334 for Qdrant, 8080 for TEI, 8090 for API
- Qdrant data: named Docker volume (`qdrant_data`)
- Go binary: multi-stage Dockerfile for CI/prod, local build option for dev iteration (both supported)
- Custom Docker network for service isolation

**Qdrant Collection**
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
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| INFRA-01 | Embedder Interface — Austauschbare Embedding-Komponente hinter einheitlicher Go-Schnittstelle (lokal oder Cloud) | Go interface pattern with TEI HTTP client; e5 prefix handling in concrete implementation; `Embed(ctx, []string)` signature |
| INFRA-02 | Qdrant Collection Setup — Collection mit Payload-Indizes für alle Filter-Dimensionen, deterministische Punkt-IDs | `qdrant.NewClient`, `CreateCollection` with `OnDisk`, `CreateFieldIndex` per payload field, `CreateAlias`, UUID v5 via `github.com/google/uuid` |
| INFRA-03 | Embedding Model Benchmark — DE/EN Recall-Benchmark mit echten Daten vor Produktiv-Einsatz (multilingual-e5-base als Default) | Synthetic QA pair approach with Recall@10; ground truth via exact search; Go benchmark program pattern |
| OPS-01 | Docker-Compose-Deployment — Alle Komponenten als Docker-Container | TEI `cpu-1.9` image, Qdrant `/healthz` health check, multi-stage Go Dockerfile, named volume, custom network |
| OPS-02 | Konfiguration — Alle Parameter über Umgebungsvariablen oder YAML-Konfigurationsdatei | Viper YAML+env pattern, `SetEnvKeyReplacer`, `Unmarshal` to typed struct, `go-playground/validator` for required fields |
</phase_requirements>

---

## Summary

Phase 1 establishes the non-negotiable foundation that every subsequent phase builds on: a working Docker Compose stack with Qdrant and TEI, a properly initialized Qdrant collection with all payload indexes and an alias, a Go Embedder interface with a TEI implementation, a correctly structured Go module, and an embedding benchmark that gates entry into Phase 2.

The research confirms all locked technical decisions are sound and well-supported by current library APIs (go-client v1.16.0, TEI v1.9.1, viper v1.x). The most critical ordering constraint is that all Qdrant payload indexes must be created at collection init — before any point is upserted — because adding indexes to an existing populated collection requires a full re-scan. The `CollectionExists` guard ensures idempotent re-runs without error.

The embedding benchmark methodology recommended is a synthetic QA pair approach using 50-100 representative DE/EN text pairs (resembling Redmine issue titles and descriptions). Recall@10 is computed by embedding all passages, upserting into a temp Qdrant collection with exact search, then checking whether each query's correct passage appears in the top-10. This validates both the model and the e5 prefix handling before 768 dimensions are committed as the schema.

**Primary recommendation:** Implement collection init as an idempotent `EnsureCollection` function that checks existence, creates with all indexes in a single transaction, and sets up the alias — run it every time on startup so re-deploys are safe.

---

## Standard Stack

### Core Libraries

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/qdrant/go-client` | v1.16.0 | Official Qdrant gRPC client | Only official Go client; gRPC-based; proto types match Qdrant server exactly; `CollectionExists`, `CreateFieldIndex`, `CreateAlias` all present |
| `github.com/google/uuid` | v1.6.0 | UUID v5 deterministic point IDs | `uuid.NewSHA1(namespace, data)` is the standard v5 API; deterministic from same inputs; widely used |
| `github.com/spf13/viper` | v1.x (v1.18+) | YAML config + env var overrides | De-facto standard for Go configuration; `SetEnvKeyReplacer` maps `QDRANT_HOST` → `qdrant.host`; `Unmarshal` to struct |
| `github.com/go-playground/validator/v10` | v10.x | Required-field validation at startup | Struct tag `validate:"required"` with `WithRequiredStructEnabled()` option; provides clear error messages listing all missing fields |

### Supporting Libraries (Phase 1 only)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/cenkalti/backoff/v4` | v4.x | Exponential backoff for TEI/Qdrant calls | Use for `EnsureCollection` and embedding calls; provides jitter to avoid thundering herd |
| `net/http` (stdlib) | Go 1.22+ | TEI HTTP client | TEI speaks plain JSON REST; no SDK needed; set explicit `Timeout` and `Transport` |
| `log/slog` (stdlib) | Go 1.21+ | Structured JSON logging | Benchmark output and startup validation errors |

### Infrastructure Images

| Image | Tag | Purpose | Confidence |
|-------|-----|---------|------------|
| `ghcr.io/huggingface/text-embeddings-inference` | `cpu-1.9` | TEI embedding service (CPU) | HIGH — v1.9.1 is current stable as of 2025-02-17 |
| `qdrant/qdrant` | `latest` or pin to `v1.16.x` | Vector database | HIGH — v1.16.x aligns with go-client v1.16.0 |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `go-playground/validator` | Manual field checks with `fmt.Errorf` | Manual checks are fine for 4 required fields; validator adds struct tags and better error messages; use validator |
| `google/uuid` | `gofrs/uuid/v5` | Both provide `NewSHA1`; google/uuid is more widely used in Go ecosystem |
| Viper | `koanf` | koanf is more modular; viper is locked in by user decision |

**Installation:**
```bash
go get github.com/qdrant/go-client@latest
go get github.com/google/uuid@latest
go get github.com/spf13/viper@latest
go get github.com/go-playground/validator/v10@latest
go get github.com/cenkalti/backoff/v4@latest
```

---

## Architecture Patterns

### Recommended Project Structure

The requirements doc already specifies the module structure. The official Go team recommends `cmd/`, `internal/`, and `api/` — no `pkg/` directory.

```
redmine-semantic-search/
├── cmd/
│   └── indexer/
│       └── main.go              # wire deps, call config.Load(), EnsureCollection(), start benchmark or exit
├── internal/
│   ├── config/
│   │   └── config.go            # Config struct with validate tags; Load() func
│   ├── embedder/
│   │   ├── embedder.go          # Embedder interface
│   │   └── tei.go               # TEI HTTP implementation; handles e5 prefix internally
│   └── qdrant/
│       └── collection.go        # EnsureCollection, CreatePayloadIndexes, EnsureAlias
├── bench/
│   └── recall/
│       └── main.go              # standalone benchmark binary
├── deployments/
│   ├── docker-compose.yml
│   ├── Dockerfile
│   └── config.example.yml
└── go.mod
```

The `bench/recall/` binary is a standalone program (not `go test -bench`), because it needs to spin up an actual Qdrant connection and TEI service to measure real recall. It exits with code 1 if Recall@10 < threshold.

### Pattern 1: Embedder Interface with e5 Prefix Encapsulation

**What:** The `Embedder` interface takes raw text. The `TEIEmbedder` concrete implementation prepends `"passage: "` for indexing and the interface has a separate method (or a mode parameter) for query embedding (`"query: "`).

**Recommendation:** Use two methods on the interface — `EmbedPassages` and `EmbedQuery` — rather than a single `Embed` method with a mode parameter. This makes it impossible to call the wrong variant and is self-documenting.

```go
// Source: internal/embedder/embedder.go
package embedder

import "context"

// Embedder converts text to dense float vectors.
// Implementations handle all model-specific prefix requirements internally.
type Embedder interface {
    // EmbedPassages prepends "passage: " for e5 models and returns one vector per text.
    EmbedPassages(ctx context.Context, texts []string) ([][]float32, error)
    // EmbedQuery prepends "query: " for e5 models and returns a single vector.
    EmbedQuery(ctx context.Context, text string) ([]float32, error)
}
```

**Why two methods over a mode enum:** The e5 prefix requirement is an implementation detail of the TEI/e5 embedder. A future OpenAI embedder (Phase 4+) uses the same interface but ignores the prefix. The interface hides this completely from callers — the indexer always calls `EmbedPassages`, the search handler always calls `EmbedQuery`.

```go
// Source: internal/embedder/tei.go
package embedder

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
)

type TEIEmbedder struct {
    baseURL    string
    httpClient *http.Client
}

func NewTEIEmbedder(baseURL string) *TEIEmbedder {
    return &TEIEmbedder{
        baseURL:    baseURL,
        httpClient: &http.Client{Timeout: 30 * time.Second},
    }
}

func (e *TEIEmbedder) EmbedPassages(ctx context.Context, texts []string) ([][]float32, error) {
    prefixed := make([]string, len(texts))
    for i, t := range texts {
        prefixed[i] = "passage: " + t
    }
    return e.embed(ctx, prefixed)
}

func (e *TEIEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
    vecs, err := e.embed(ctx, []string{"query: " + text})
    if err != nil {
        return nil, err
    }
    return vecs[0], nil
}

func (e *TEIEmbedder) embed(ctx context.Context, inputs []string) ([][]float32, error) {
    body, _ := json.Marshal(map[string]any{"inputs": inputs})
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embed", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    resp, err := e.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("TEI embed: %w", err)
    }
    defer resp.Body.Close()
    var result [][]float32
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("TEI embed decode: %w", err)
    }
    return result, nil
}
```

### Pattern 2: Qdrant Collection Init (Idempotent)

**What:** `EnsureCollection` checks existence first, creates only if absent, then creates all payload indexes and the alias. Safe to call on every startup.

**Why idempotent matters:** If the process is restarted or the Docker Compose stack is recreated with persistent volumes, the collection already exists. A non-idempotent init would fail with "collection already exists" on the second start.

```go
// Source: internal/qdrant/collection.go — pattern verified against pkg.go.dev/github.com/qdrant/go-client/qdrant
package qdrant

import (
    "context"
    "github.com/qdrant/go-client/qdrant"
)

const (
    CollectionName   = "redmine_search_v1"
    AliasName        = "redmine_search"
    VectorDimension  = 768
)

func EnsureCollection(ctx context.Context, client *qdrant.Client) error {
    exists, err := client.CollectionExists(ctx, CollectionName)
    if err != nil {
        return fmt.Errorf("check collection: %w", err)
    }
    if exists.Result {
        return nil // already initialized
    }

    onDisk := true
    if err := client.CreateCollection(ctx, &qdrant.CreateCollection{
        CollectionName: CollectionName,
        VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
            Size:     VectorDimension,
            Distance: qdrant.Distance_Cosine,
            OnDisk:   &onDisk,
        }),
        OnDiskPayload: &onDisk,
    }); err != nil {
        return fmt.Errorf("create collection: %w", err)
    }

    if err := createPayloadIndexes(ctx, client); err != nil {
        return err
    }

    return EnsureAlias(ctx, client)
}

func createPayloadIndexes(ctx context.Context, client *qdrant.Client) error {
    indexes := []struct {
        field     string
        fieldType qdrant.FieldType
    }{
        {"project_id", qdrant.FieldType_Integer},
        {"content_type", qdrant.FieldType_Keyword},
        {"tracker", qdrant.FieldType_Keyword},
        {"status", qdrant.FieldType_Keyword},
        {"author", qdrant.FieldType_Keyword},
        {"created_on", qdrant.FieldType_Datetime},
        {"updated_on", qdrant.FieldType_Datetime},
    }
    for _, idx := range indexes {
        wait := true
        if err := client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
            CollectionName: CollectionName,
            FieldName:      idx.field,
            FieldType:      idx.fieldType,
            Wait:           &wait,
        }); err != nil {
            return fmt.Errorf("create index %s: %w", idx.field, err)
        }
    }
    return nil
}

func EnsureAlias(ctx context.Context, client *qdrant.Client) error {
    _, err := client.CreateAlias(ctx, AliasName, CollectionName)
    return err
}
```

**Critical:** `Wait: true` on each `CreateFieldIndex` call ensures the index is built before returning. Without it, the next startup might start indexing before indexes are ready, degrading filter performance.

### Pattern 3: Config with Viper + Fail-Fast Validation

**Known Viper pitfall:** `AutomaticEnv()` does not work with `Unmarshal()` unless you also call `SetDefault()` or `BindEnv()` for each field, or use `SetEnvKeyReplacer`. The workaround that works reliably: set defaults (even empty string defaults) for all fields, then `AutomaticEnv()` picks them up during `Unmarshal`.

```go
// Source: internal/config/config.go
package config

import (
    "fmt"
    "strings"

    "github.com/go-playground/validator/v10"
    "github.com/spf13/viper"
)

type Config struct {
    RedmineURL    string `mapstructure:"redmine_url"    validate:"required,url"`
    RedmineAPIKey string `mapstructure:"redmine_api_key" validate:"required"`
    QdrantHost    string `mapstructure:"qdrant_host"     validate:"required"`
    QdrantPort    int    `mapstructure:"qdrant_port"`
    EmbeddingURL  string `mapstructure:"embedding_url"   validate:"required,url"`
}

func Load() (*Config, error) {
    viper.SetConfigName("config")
    viper.SetConfigType("yaml")
    viper.AddConfigPath(".")

    // Defaults enable AutomaticEnv to work with Unmarshal
    viper.SetDefault("qdrant_port", 6334)
    viper.SetDefault("redmine_url", "")
    viper.SetDefault("redmine_api_key", "")
    viper.SetDefault("qdrant_host", "")
    viper.SetDefault("embedding_url", "")

    viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
    viper.AutomaticEnv()

    if err := viper.ReadInConfig(); err != nil {
        if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
            return nil, fmt.Errorf("read config: %w", err)
        }
    }

    var cfg Config
    if err := viper.Unmarshal(&cfg); err != nil {
        return nil, fmt.Errorf("unmarshal config: %w", err)
    }

    validate := validator.New(validator.WithRequiredStructEnabled())
    if err := validate.Struct(&cfg); err != nil {
        return nil, fmt.Errorf("config validation failed:\n%w", err)
    }

    return &cfg, nil
}
```

### Pattern 4: UUID v5 Point IDs

```go
// Source: github.com/google/uuid v1.6.0 — NewSHA1 API
import "github.com/google/uuid"

// Fixed application namespace — defined once, never changes.
// Changing this namespace invalidates all existing point IDs.
var pointIDNamespace = uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")

// PointID returns a deterministic UUID v5 for a given content type and Redmine ID.
// Same inputs always produce the same UUID — enables idempotent upserts.
func PointID(contentType string, redmineID int) string {
    key := fmt.Sprintf("%s:%d", contentType, redmineID)
    return uuid.NewSHA1(pointIDNamespace, []byte(key)).String()
}
```

**Important:** The namespace UUID must be a fixed, application-specific value (not `uuid.NameSpaceDNS` or `uuid.NameSpaceURL`). Using a shared namespace risks UUID collisions with other systems using the same namespace + similar data. Generate a random UUID once for the project and hardcode it.

### Pattern 5: Go Multi-Stage Dockerfile

```dockerfile
# Multi-stage build — builder stage
FROM golang:1.23-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/indexer ./cmd/indexer

# Runtime stage — minimal image
FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl && \
    adduser -D -u 1000 appuser
WORKDIR /app
COPY --from=builder /app/indexer .
COPY --chown=appuser:appuser deployments/config.example.yml ./config.example.yml
USER appuser
EXPOSE 8090
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -f http://localhost:8090/health || exit 1
CMD ["./indexer"]
```

**Why alpine over scratch/distroless for this project:** `curl` is needed for the health check, and CA certificates are needed for HTTPS to Redmine and HuggingFace Hub (model download). Alpine keeps the image small (~15 MB) while including these.

### Anti-Patterns to Avoid

- **Creating indexes after first upsert:** Payload indexes created on a populated collection require a full re-scan (blocking). Create all indexes before any upsert, at collection init time.
- **Non-idempotent collection init:** Calling `CreateCollection` unconditionally fails if the collection exists. Always guard with `CollectionExists`.
- **Forgetting `Wait: true` on index creation:** Without waiting, indexes may not be ready when the process continues, causing degraded filter performance during the brief window.
- **Using `AutomaticEnv()` alone without `SetDefault()`:** Viper v1.19+ broke `Unmarshal()` with `AutomaticEnv()` when no default is set. Always set a default (even empty string) for each required field.
- **Hardcoding `uuid.NameSpaceDNS` as the point ID namespace:** Namespace collisions with other systems using the same namespace are unlikely but possible. Use a project-specific UUID.
- **Omitting e5 prefixes:** `multilingual-e5-base` requires `"query: "` and `"passage: "` prefixes. Omitting them measurably degrades retrieval quality. The model card documents this as required, not optional.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Deterministic UUIDs | Custom hash → hex string | `github.com/google/uuid` `NewSHA1` | UUID v5 is a standard; custom hash may not be RFC 4122 compliant; Qdrant accepts UUID strings natively |
| Config env override | Custom `os.Getenv` loop | `viper` with `AutomaticEnv` | Handles YAML merging, type coercion, nested keys; battle-tested |
| Required-field validation | Manual `if cfg.X == ""` checks | `go-playground/validator/v10` | Reports all missing fields at once; struct tags are self-documenting |
| Retry with backoff | `time.Sleep` loop | `cenkalti/backoff/v4` | Handles jitter, max elapsed time, context cancellation correctly; common edge cases are pre-handled |
| HTTP batching to TEI | Manual JSON marshaling per request | `net/http` with `[]string` inputs | TEI `/embed` accepts a JSON array natively; no special library needed |

**Key insight:** The Qdrant collection init and alias setup look simple but have subtle ordering requirements (indexes before data, alias after collection). The go-client API is straightforward — don't abstract it further than the `EnsureCollection` pattern shown above.

---

## Common Pitfalls

### Pitfall 1: Viper AutomaticEnv + Unmarshal Regression (v1.19+)

**What goes wrong:** `viper.AutomaticEnv()` + `viper.Unmarshal(&cfg)` silently returns zero values for all fields that have no explicit default or `BindEnv` call, even if the environment variable is set. This was a behavior change in v1.19.0.

**Why it happens:** `Unmarshal` only processes keys that exist in Viper's key registry. `AutomaticEnv` only resolves env vars for registered keys. Without `SetDefault()`, the key is never registered.

**How to avoid:** Set a default for every config field (even empty string). `viper.SetDefault("redmine_url", "")` registers the key; then `AutomaticEnv` can find the matching `REDMINE_URL` env var.

**Warning signs:** Config struct fields are always zero/empty even when env vars are clearly set.

### Pitfall 2: Payload Indexes Created After Upsert

**What goes wrong:** Collection is created, some documents are upserted, then payload indexes are added. Qdrant must re-scan all existing points to build the index, which is slow and holds a write lock.

**Why it happens:** Teams often create the collection without indexes "just to test" and then forget to add indexes before production load.

**How to avoid:** `EnsureCollection` creates ALL indexes as part of collection init. The index list is defined in code alongside the collection definition — they cannot be separated.

**Warning signs:** Index creation takes minutes on a populated collection; filter queries are slow despite the index existing.

### Pitfall 3: TEI Container Cold Start

**What goes wrong:** Docker Compose starts TEI, but the container is healthy (port 8080 responding) before the model is fully loaded. The Go binary starts, calls `/embed`, and gets a 503.

**Why it happens:** TEI loads the model from disk on first start. The container's port opens before the model is in memory.

**How to avoid:** Implement retry with exponential backoff in `TEIEmbedder.embed()`. The benchmark and collection init should retry on 503/connection refused. Pre-download the model to a Docker volume so subsequent starts are faster.

**Warning signs:** `bench/recall/main.go` fails immediately on first Docker Compose start, succeeds on retry.

### Pitfall 4: Qdrant Health Check Without curl in Image

**What goes wrong:** Standard Docker Compose health check `CMD curl -f http://localhost:6333/healthz` fails because the Qdrant Docker image does not include curl.

**Why it happens:** Qdrant removed curl from their image for security. This is a known issue (GitHub issue #4250).

**How to avoid:** Use bash's `/dev/tcp` feature in the health check, or use the `CMD-SHELL` form with explicit bash:

```yaml
healthcheck:
  test: ["CMD", "bash", "-c", "exec 3<>/dev/tcp/127.0.0.1/6333 && echo -e 'GET /readyz HTTP/1.1\\r\\nHost: localhost\\r\\nConnection: close\\r\\n\\r\\n' >&3 && grep -q 'HTTP/1.1 200' <&3"]
  interval: 10s
  timeout: 5s
  retries: 5
  start_period: 30s
```

Alternatively, add curl to a custom Qdrant image layer — acceptable for dev, not recommended for production.

**Warning signs:** `docker compose ps` shows qdrant as "starting" indefinitely.

### Pitfall 5: Missing e5 Prefix Causes Silent Quality Degradation

**What goes wrong:** Embedding calls work correctly (no error), but Recall@10 scores are significantly worse than the model card reports.

**Why it happens:** `multilingual-e5-base` was trained with `"query: "` and `"passage: "` prefixes as part of the input format. Without these prefixes, the model embeds text in an undefined mode, producing vectors that don't align between queries and passages.

**How to avoid:** Prefix handling is encapsulated in `TEIEmbedder.EmbedPassages()` and `TEIEmbedder.EmbedQuery()`. The benchmark validates this: run it with and without prefixes — the score difference is measurable. Embedding model changes cannot remove these methods without breaking the interface.

**Warning signs:** Benchmark Recall@10 is below 0.5 for simple factual queries.

---

## Code Examples

### TEI /embed API — Request/Response Format

```bash
# Source: huggingface.co/docs/text-embeddings-inference/en/quick_tour
# Batch embedding (preferred)
curl 127.0.0.1:8080/embed \
    -X POST \
    -d '{"inputs":["passage: Redmine issue text here", "passage: Another issue"]}' \
    -H 'Content-Type: application/json'

# Response: [[0.1, 0.2, ...], [0.3, 0.4, ...]]
# 768-element float arrays for multilingual-e5-base
```

TEI defaults: `--max-client-batch-size 32`, `--max-batch-tokens 16384`. A batch of 32 × 512-token texts is well within defaults.

### Docker Compose for Phase 1

```yaml
# Source: user decisions + Qdrant installation docs + TEI quick tour
services:
  redmine-search-qdrant:
    image: qdrant/qdrant:latest
    container_name: redmine-search-qdrant
    ports:
      - "6333:6333"
      - "6334:6334"
    volumes:
      - qdrant_data:/qdrant/storage
    networks:
      - redmine-search-net
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "bash", "-c", "exec 3<>/dev/tcp/127.0.0.1/6333 && echo -e 'GET /readyz HTTP/1.1\\r\\nHost: localhost\\r\\nConnection: close\\r\\n\\r\\n' >&3 && grep -q '200' <&3"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 30s

  redmine-search-embedding:
    image: ghcr.io/huggingface/text-embeddings-inference:cpu-1.9
    container_name: redmine-search-embedding
    ports:
      - "8080:80"
    volumes:
      - ./models:/data
    command: --model-id intfloat/multilingual-e5-base
    networks:
      - redmine-search-net
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:80/health"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 60s

  redmine-search-indexer:
    build: .
    container_name: redmine-search-indexer
    ports:
      - "8090:8090"
    volumes:
      - ./config.yml:/app/config.yml:ro
      - ./logs:/app/logs
    env_file:
      - .env
    depends_on:
      redmine-search-qdrant:
        condition: service_healthy
      redmine-search-embedding:
        condition: service_healthy
    networks:
      - redmine-search-net
    restart: unless-stopped

volumes:
  qdrant_data:

networks:
  redmine-search-net:
    driver: bridge
```

**Note on TEI health check:** The TEI CPU image includes curl (unlike Qdrant), so the standard `curl -f` health check works. TEI exposes `/health` on its internal port 80 (mapped to host 8080). The `start_period: 60s` accounts for model download on first start.

### Qdrant gRPC Client Initialization

```go
// Source: github.com/qdrant/go-client README + pkg.go.dev
// One shared client for the entire process lifetime.
// The go-client manages the gRPC connection pool internally.
client, err := qdrant.NewClient(&qdrant.Config{
    Host: cfg.QdrantHost,
    Port: cfg.QdrantPort, // 6334 (gRPC)
})
if err != nil {
    log.Fatal("qdrant connect", "err", err)
}
defer client.Close()
```

**Confirmed:** `qdrant.Config` supports `PoolSize`, `KeepAliveTime`, `KeepAliveTimeout` as optional fields. For Phase 1, defaults are fine. Do not create a new client per request.

### Benchmark: Recall@10 Implementation Pattern

```go
// Source: bench/recall/main.go pattern
// Recall@10 = correct passage appears in top-10 results for its query

type QAPair struct {
    Query   string // e.g. "Fehler beim Speichern von Anhängen"
    Passage string // e.g. "Issue #1234: Datei-Upload schlägt fehl bei..."
}

func computeRecallAtK(ctx context.Context, pairs []QAPair, embedder embedder.Embedder, client *qdrant.Client, k uint64) (float64, error) {
    // 1. Embed all passages and upsert to a temp collection
    // 2. For each QA pair, embed the query and search top-K
    // 3. Check if the correct passage ID appears in top-K results
    // 4. Recall@K = hits / total pairs
}
```

---

## Benchmark Methodology (Claude's Discretion)

**Recommendation: Synthetic QA pairs from representative domain text**

### Why Synthetic Over Real Data Export

Real Redmine data may not be available in Phase 1 (Redmine connection is Phase 2+). Synthetic pairs can be constructed from publicly available German/English technical text that resembles Redmine issues. This unblocks the benchmark from the Redmine dependency.

### Construction

Create 50-100 QA pairs covering:
- 25 German-language pairs (Redmine issue descriptions, wiki-style text)
- 25 English-language pairs
- Include edge cases: short titles (3-5 words), long descriptions (400+ tokens), code snippets

**Format:**
```go
var benchmarkPairs = []QAPair{
    // German technical
    {
        Query:   "Fehler beim Upload von Anhängen",
        Passage: "passage: Beim Hochladen von Anhängen größer als 10MB tritt ein Timeout-Fehler auf. Betrifft alle Projekte. Reproduzierbar mit Firefox und Chrome.",
    },
    // English technical
    {
        Query:   "login fails with LDAP",
        Passage: "passage: Authentication via LDAP fails when the domain controller is unreachable. Error: connection timeout after 5s. Affects all users in the EU office.",
    },
    // Mixed (German query, German passage)
    // Cross-lingual (DE query, EN passage) — validates multilingual capability
}
```

### Evaluation

1. Embed all passages with `EmbedPassages()` → upsert to temp Qdrant collection (exact search via `HnswConfig{OnDisk: false}` or exact brute-force mode)
2. For each pair, embed the query with `EmbedQuery()` → search top-10
3. Recall@10 = fraction of pairs where the correct passage is in top-10
4. **Threshold:** Recall@10 ≥ 0.80 is acceptable for go/no-go. Below 0.70 = block Phase 2.
5. Run twice: once WITH e5 prefixes, once WITHOUT — document the delta to prove prefix handling is correct.

**Why Recall@10 not Recall@1:** Redmine search typically shows 10-20 results. Recall@10 matches the user's actual experience. MRR (Mean Reciprocal Rank) is also useful to compute as it captures ranking quality.

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Docker Compose v1 (`docker-compose`) | Docker Compose v2 plugin (`docker compose`) | May 2024 (v1 EOL) | No `version:` field; different binary name |
| `viper.AutomaticEnv()` alone | `SetDefault()` + `AutomaticEnv()` | Viper v1.19.0 (2024) | Silent breakage if using `Unmarshal()` |
| TEI `cpu-1.5` or `cpu-1.6` | `cpu-1.9` | Feb 2025 | v1.9 reduces latency 50% for small models; breaking: `--auto-truncate` now defaults to true |
| `qdrant/go-client v1.13` | `v1.16.0` | Nov 2024 | Deprecates `data`, `indices`, `vectors_count` fields; use helper methods |
| `validator.New()` | `validator.New(validator.WithRequiredStructEnabled())` | v10 recent | Correct required-tag behavior for struct fields |

**Deprecated/outdated:**
- `docker-compose` (v1 binary): EOL May 2024. Use `docker compose` (v2 plugin).
- Viper `AutomaticEnv()` without `SetDefault()`: Broken for `Unmarshal()` since v1.19.0.
- TEI `cpu-1.5` / `cpu-1.6`: Superseded by 1.9; use `cpu-1.9`.

---

## Open Questions

1. **TEI model download on first start**
   - What we know: TEI downloads `intfloat/multilingual-e5-base` from HuggingFace Hub on first run; subsequent runs use the cached model
   - What's unclear: Whether the `./models` volume mount is sufficient for caching, or if HF_HOME also needs to be set
   - Recommendation: Set `HF_HUB_CACHE=/data` in the TEI service environment, and mount `./models:/data`. Test by stopping/starting the container and verifying no re-download.

2. **Qdrant healthcheck reliability across image versions**
   - What we know: The `/dev/tcp` bash trick works in current Qdrant images but may not in all versions
   - What's unclear: Whether future Qdrant images will include a built-in healthcheck binary
   - Recommendation: Use the bash `/dev/tcp` approach but document this as a known quirk; monitor qdrant/qdrant GitHub issues for a native healthcheck addition.

3. **Benchmark threshold for go/no-go**
   - What we know: `multilingual-e5-base` achieves ~65.9 avg MRR@10 on Mr. TyDi (multilingual retrieval benchmark)
   - What's unclear: How that translates to Recall@10 on Redmine-domain text
   - Recommendation: Set the initial threshold at Recall@10 ≥ 0.75 for the synthetic benchmark. If the real threshold matters more, adjust after seeing actual benchmark results. Document both WITH and WITHOUT prefix runs to prove the prefix handling works.

---

## Sources

### Primary (HIGH confidence)

- `pkg.go.dev/github.com/qdrant/go-client/qdrant` — `CollectionExists`, `CreateCollection`, `CreateFieldIndex`, `CreateAlias`, `VectorParams`, `FieldType` enum values; all APIs verified directly
- `github.com/qdrant/go-client/releases` — confirmed v1.16.0 is latest (Nov 17, 2024)
- `huggingface.co/docs/text-embeddings-inference/en/quick_tour` — `/embed` endpoint format, batch support, Docker run command
- `github.com/huggingface/text-embeddings-inference/releases` — confirmed v1.9.1 is latest (Feb 17, 2025); `cpu-1.9` is the current CPU tag
- `huggingface.co/intfloat/multilingual-e5-base` — model card: 768 dims, 512 max tokens, `query: `/`passage: ` prefix requirement, Mr. TyDi benchmark results
- `pkg.go.dev/github.com/google/uuid` — `NewSHA1(space UUID, data []byte) UUID` API, v1.6.0
- `go.dev/doc/modules/layout` — official Go project layout recommendations (`cmd/`, `internal/`, no `pkg/`)
- `qdrant.tech/documentation/guides/monitoring/` — `/healthz`, `/livez`, `/readyz` endpoints confirmed; available since Qdrant v1.5.0; return HTTP 200 when healthy
- `github.com/qdrant/qdrant/issues/4250` — confirmed curl absent from Qdrant image; `/dev/tcp` bash healthcheck workaround documented

### Secondary (MEDIUM confidence)

- `haseebmajid.dev/posts/2024-05-19-how-to-use-env-variables-with-viper-config-library-in-go/` — Viper `SetEnvKeyReplacer` + `AutomaticEnv` + `Unmarshal` pattern with `SetDefault` workaround; confirmed against spf13/viper GitHub issues #1895 and #761
- `github.com/spf13/viper/issues/1895` — confirmed `AutomaticEnv` + `Unmarshal` regression in v1.19.0; `SetDefault` workaround is the accepted solution
- `pkg.go.dev/github.com/go-playground/validator/v10` — `WithRequiredStructEnabled()` option; confirmed as recommended initialization pattern for v10+

### Tertiary (LOW confidence — flagged for validation)

- TEI health check via `/health` endpoint on port 80 (internal) — documented in quick tour but not in a dedicated health reference; validate with `curl` against running container
- HF_HUB_CACHE environment variable for model caching in TEI — documented in general HuggingFace Hub docs but not confirmed in TEI Docker context; validate during Docker Compose setup

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all library versions verified against official release pages
- Architecture patterns: HIGH — derived from official Go module layout docs + qdrant/go-client API reference
- Pitfalls: HIGH (Viper regression: confirmed via GitHub issues; Qdrant healthcheck: confirmed via GitHub issue #4250; e5 prefixes: confirmed via model card)
- Benchmark methodology: MEDIUM — Recall@10 approach is standard IR practice; threshold value (0.75) is a recommendation, not a verified minimum for this model/domain

**Research date:** 2026-02-18
**Valid until:** 2026-03-18 (stable libraries; TEI releases frequently but API is stable)
