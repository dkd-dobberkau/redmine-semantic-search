# Redmine Semantic Search

Find relevant Redmine content through meaning, not just keywords. Users can search issues (and soon wikis, journals) using natural language — even when they don't know the exact wording — while Redmine's permission model is fully respected.

## How It Works

```
Redmine ──polling──> Indexer ──embed──> Qdrant
                                          |
User ──query──> Search Server ──ANN search──┘
                    |
            permission filter
            (pre + post)
```

1. The **Indexer** polls Redmine for new/updated issues, strips Textile/Markdown formatting, splits text into overlapping chunks, generates embeddings via [Ollama](https://ollama.com/) or [TEI](https://github.com/huggingface/text-embeddings-inference), and upserts them into [Qdrant](https://qdrant.tech/).
2. The **Search Server** takes a natural-language query, embeds it, runs a filtered approximate nearest neighbor search against Qdrant, and returns permission-filtered, ranked results with snippets and facets.
3. **Permissions** are enforced in two phases: pre-filtering by the user's accessible project IDs, then post-filtering for fine-grained rules (e.g. private issues).
4. A built-in **Web UI** at the server root provides a simple search interface with direct links to Redmine issues.

## Features

- **Semantic search** — find issues by meaning, not exact text match
- **Multilingual** — works with German and English content out of the box
- **Permission-aware** — respects Redmine project memberships and private issue visibility
- **Incremental sync** — only re-indexes changed issues (configurable polling interval)
- **Deletion reconciliation** — automatically removes deleted issues from the index
- **Faceted filtering** — filter by project, tracker, status, author, date range, content type
- **Pagination & snippets** — paginated results with relevant text excerpts
- **Web UI** — minimal browser-based search interface served by the search server
- **Pluggable embeddings** — switch between Ollama (default, native ARM64) and TEI

## Architecture

| Component | Technology |
|-----------|-----------|
| Language | Go |
| Vector DB | Qdrant (gRPC) |
| Embeddings | Ollama (nomic-embed-text, 768d) or TEI (multilingual-e5-base, 768d) |
| Config | Viper (YAML + env vars) |
| Frontend | Single-file HTML/JS (served by Go) |
| Deployment | Docker Compose |

### Project Structure

```
cmd/
  indexer/          # Indexer process (sync + reconciliation)
  server/           # Search HTTP API server + web UI
internal/
  auth/             # API key validation, permission cache
  config/           # Configuration loading (YAML + env)
  embedder/         # Embedding interface (Ollama + TEI implementations)
  indexer/          # Pipeline, sync scheduler, reconciler
  qdrant/           # Collection setup, point ID generation
  redmine/          # Redmine REST API client
  search/           # Search handler, health endpoint
  text/             # Textile/Markdown stripping, chunking
web/
  index.html        # Search frontend
deployments/
  docker-compose.yml
  Dockerfile
```

## Quick Start

### Prerequisites

- Docker and Docker Compose
- [Ollama](https://ollama.com/) installed with an embedding model (`ollama pull nomic-embed-text`)
- A Redmine instance with REST API enabled
- A Redmine admin API key (for the indexer to read all issues)

### 1. Configure

```bash
cp .env.example deployments/.env
```

Edit `deployments/.env`:

```env
REDMINE_URL=https://your-redmine.example.com
REDMINE_API_KEY=your-admin-api-key
EMBEDDING_URL=http://host.docker.internal:11434
```

All other settings have sensible defaults. See `config.example.yml` for the full list.

### 2. Start

```bash
cd deployments
docker compose up --build
```

This starts:
- **Qdrant** — vector database on ports 6333 (HTTP) / 6334 (gRPC)
- **Indexer** — connects to your local Ollama and begins indexing issues immediately

The search server runs inside the indexer container on port **8090**.

### 3. Search

Open **http://localhost:8090** in your browser for the web UI.

Or use the API directly:

```bash
curl -H "X-Redmine-API-Key: YOUR_USER_API_KEY" \
  "http://localhost:8090/api/v1/search?q=deployment+problem"
```

The user's API key determines which projects they can see in results.

## API

### `GET /api/v1/search`

Search for issues using natural language.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `q` | string | required | Search query |
| `project_id` | int | — | Filter by project |
| `tracker` | string | — | Filter by tracker |
| `status` | string | — | Filter by status |
| `author_id` | int | — | Filter by author |
| `content_type` | string | — | Filter by content type |
| `limit` | int | 20 | Results per page (max 100) |
| `offset` | int | 0 | Pagination offset |

**Header:** `X-Redmine-API-Key` (required)

### `GET /api/v1/health`

Returns the status of Qdrant and the embedding service. No authentication required.

### `GET /api/v1/config`

Returns the configured Redmine URL and API key (used by the web UI).

## Configuration

All settings can be configured via environment variables or `config.yml`:

| Setting | Env Var | Default | Description |
|---------|---------|---------|-------------|
| Redmine URL | `REDMINE_URL` | — | Base URL of your Redmine instance |
| Redmine API Key | `REDMINE_API_KEY` | — | Admin API key for indexing |
| Qdrant Host | `QDRANT_HOST` | `localhost` | Qdrant gRPC hostname |
| Qdrant Port | `QDRANT_PORT` | `6334` | Qdrant gRPC port |
| Embedding Provider | `EMBEDDING_PROVIDER` | `ollama` | `ollama` or `tei` |
| Embedding URL | `EMBEDDING_URL` | — | Embedding service URL |
| Embedding Model | `EMBEDDING_MODEL` | `nomic-embed-text` | Ollama model name (ignored for TEI) |
| Status Filter | `SYNC_STATUS_FILTER` | `open` | `open` or `*` (all statuses) |
| Sync Interval | `SYNC_INTERVAL` | `5` | Polling interval in minutes |
| Sync Batch Size | `SYNC_BATCH_SIZE` | `100` | Max issues per polling cycle |
| Reconcile Schedule | `RECONCILE_SCHEDULE` | `0 */6 * * *` | Cron schedule for deletion reconciliation |
| Listen Address | `LISTEN_ADDR` | `:8090` | HTTP server bind address |
| Permission Cache TTL | `PERMISSION_CACHE_TTL` | `5` | Permission cache TTL in minutes |

### Embedding Providers

**Ollama** (default) — native ARM64 support, simple setup, runs locally:
```env
EMBEDDING_PROVIDER=ollama
EMBEDDING_URL=http://localhost:11434
EMBEDDING_MODEL=nomic-embed-text
```

**TEI** — HuggingFace Text Embeddings Inference, amd64 only:
```env
EMBEDDING_PROVIDER=tei
EMBEDDING_URL=http://localhost:8080
```

## Roadmap

- [x] **Phase 1** — Foundation (Docker Compose, embedder, Qdrant setup, model benchmark)
- [x] **Phase 2** — Core Issue Search (indexer pipeline, incremental sync, permission-filtered search API)
- [ ] **Phase 3** — Content Breadth & Operations (wiki/journal indexing, zero-downtime reindex, OpenAPI spec)
- [ ] **Phase 4** — Hybrid Search (BM25/SPLADE sparse vectors, configurable fusion weighting)
- [ ] **Phase 5** — API Completeness (similar issues endpoint, admin reindex endpoint)

## License

[MIT](LICENSE)
