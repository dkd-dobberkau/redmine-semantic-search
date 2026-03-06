# Redmine Semantic Search

Find relevant Redmine content through meaning, not just keywords. Users can search issues (and soon wikis, journals) using natural language — even when they don't know the exact wording — while Redmine's permission model is fully respected.

## How It Works

```
Redmine ──polling──> Indexer ──embed──> Qdrant
                                          |
User ──query──> Search API ──ANN search──┘
                    |
            permission filter
            (pre + post)
```

1. The **Indexer** polls Redmine for new/updated issues, strips Textile/Markdown formatting, splits text into overlapping chunks, generates embeddings via [TEI](https://github.com/huggingface/text-embeddings-inference) (multilingual-e5-base), and upserts them into [Qdrant](https://qdrant.tech/).
2. The **Search Server** takes a natural-language query, embeds it, runs a filtered approximate nearest neighbor search against Qdrant, and returns permission-filtered, ranked results with snippets and facets.
3. **Permissions** are enforced in two phases: pre-filtering by the user's accessible project IDs, then post-filtering for fine-grained rules (e.g. private issues).

## Features

- **Semantic search** — find issues by meaning, not exact text match
- **Multilingual** — works with German and English content (multilingual-e5-base)
- **Permission-aware** — respects Redmine project memberships and private issue visibility
- **Incremental sync** — only re-indexes changed issues (configurable polling interval)
- **Deletion reconciliation** — automatically removes deleted issues from the index
- **Faceted filtering** — filter by project, tracker, status, author, date range, content type
- **Pagination & snippets** — paginated results with relevant text excerpts

## Architecture

| Component | Technology |
|-----------|-----------|
| Language | Go |
| Vector DB | Qdrant (gRPC) |
| Embeddings | Text Embeddings Inference (multilingual-e5-base, 768d) |
| Config | Viper (YAML + env vars) |
| Deployment | Docker Compose |

### Project Structure

```
cmd/
  indexer/          # Indexer process (sync + reconciliation)
  server/           # Search HTTP API server
internal/
  auth/             # API key validation, permission cache
  config/           # Configuration loading (YAML + env)
  embedder/         # Embedding interface + TEI implementation
  indexer/          # Pipeline, sync scheduler, reconciler
  qdrant/           # Collection setup, point ID generation
  redmine/          # Redmine REST API client
  search/           # Search handler, health endpoint
  text/             # Textile/Markdown stripping, chunking
deployments/
  docker-compose.yml
  Dockerfile
```

## Quick Start

### Prerequisites

- Docker and Docker Compose
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
```

All other settings have sensible defaults. See `config.example.yml` for the full list.

### 2. Start

```bash
cd deployments
docker compose up --build
```

This starts:
- **Qdrant** — vector database on ports 6333 (HTTP) / 6334 (gRPC)
- **TEI** — embedding server on port 8080 (downloads the model on first start)
- **Indexer** — begins indexing issues immediately

The search server runs inside the indexer container on port **8090**.

### 3. Search

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

## Configuration

All settings can be configured via environment variables or `config.yml`:

| Setting | Env Var | Default | Description |
|---------|---------|---------|-------------|
| Redmine URL | `REDMINE_URL` | — | Base URL of your Redmine instance |
| Redmine API Key | `REDMINE_API_KEY` | — | Admin API key for indexing |
| Qdrant Host | `QDRANT_HOST` | `localhost` | Qdrant gRPC hostname |
| Qdrant Port | `QDRANT_PORT` | `6334` | Qdrant gRPC port |
| Embedding URL | `EMBEDDING_URL` | `http://localhost:8080` | TEI service URL |
| Sync Interval | `SYNC_INTERVAL` | `5` | Polling interval in minutes |
| Sync Batch Size | `SYNC_BATCH_SIZE` | `100` | Max issues per polling cycle |
| Reconcile Schedule | `RECONCILE_SCHEDULE` | `0 */6 * * *` | Cron schedule for deletion reconciliation |
| Listen Address | `LISTEN_ADDR` | `:8090` | HTTP server bind address |
| Permission Cache TTL | `PERMISSION_CACHE_TTL` | `5` | Permission cache TTL in minutes |

## Roadmap

- [x] **Phase 1** — Foundation (Docker Compose, embedder, Qdrant setup, model benchmark)
- [x] **Phase 2** — Core Issue Search (indexer pipeline, incremental sync, permission-filtered search API)
- [ ] **Phase 3** — Content Breadth & Operations (wiki/journal indexing, zero-downtime reindex, OpenAPI spec)
- [ ] **Phase 4** — Hybrid Search (BM25/SPLADE sparse vectors, configurable fusion weighting)
- [ ] **Phase 5** — API Completeness (similar issues endpoint, admin reindex endpoint)

## License

[MIT](LICENSE)
