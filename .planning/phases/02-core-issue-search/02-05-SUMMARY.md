---
phase: 02-core-issue-search
plan: 05
subsystem: search-api
tags: [http-server, search-endpoint, facets, health-check, auth-middleware, pagination, qdrant, embedder]
dependency_graph:
  requires: [02-02, 02-04]
  provides: [search-http-api, health-endpoint, server-entrypoint]
  affects: [docker-compose, dockerfile-server]
tech_stack:
  added: []
  patterns: [http-handler-interface, permission-pre-filter, post-filter-private, chunk-dedup, parallel-facets, graceful-shutdown]
key_files:
  created:
    - internal/search/handler.go
    - internal/search/facets.go
    - internal/search/health.go
    - cmd/server/main.go
  modified:
    - .gitignore
decisions:
  - "NewMatchInts used for project_id permission pre-filter (not N separate Should conditions) — cleaner single condition covering all accessible project IDs"
  - "Insertion sort for deduped results after map iteration — result sets are bounded by fetch limit so O(n^2) is fine"
  - "Facet errors are non-fatal — log and return nil facets rather than failing the whole search response"
  - "embeddingHealthTimeout (5s) set on a dedicated http.Client in HealthHandler to avoid sharing the default client's no-timeout setting"
metrics:
  duration: 3 min
  completed: 2026-02-18
  tasks_completed: 2
  files_created: 4
  files_modified: 1
---

# Phase 2 Plan 5: Search HTTP Server Summary

**One-liner:** Go 1.22 HTTP server with permission-filtered ANN search, private-issue post-filtering, chunk dedup, parallel facets, and public health endpoint.

## What Was Built

### `internal/search/handler.go` — SearchHandler

The core search endpoint implementation:

1. **Parse query params:** `q` (required), `page`/`per_page` (pagination), `tracker`/`status`/`project`/`author` (keyword filters), `date_from`/`date_to` (ISO date range).
2. **Permission pre-filter:** `buildPermissionFilter` builds a `qdrant.Filter` with `Must` conditions using `NewMatchInts("project_id", projectIDs...)` for the user's accessible projects. Optional keyword and date range conditions are added to the same `Must` array.
3. **Embed query:** `embedder.EmbedQuery` vectorizes the search text.
4. **ANN with oversampling:** Fetches `page * perPage * 2` results from Qdrant for post-filtering headroom.
5. **Post-filter private issues:** Compares numeric `author_id` from payload against `user.UserID` (not display name string) — non-admin users who are not the author are excluded.
6. **Chunk dedup:** Groups by `redmine_id`, keeps highest-scoring chunk per issue, then re-sorts by score.
7. **Pagination:** Slices the deduped result list by `(page-1)*perPage` to `page*perPage`.
8. **Facets:** Calls `FetchFacets` with the same permission filter; errors are non-fatal.
9. **JSON response:** `SearchResponse` with results, total, page, per_page, and facets.

### `internal/search/facets.go` — FetchFacets

Runs four Qdrant `Facet` calls concurrently using `sync.WaitGroup`:
- `tracker`, `status`, `project_id`, `author` — each with the same permission filter (Pitfall 6 from research: facets must use the same filter as the main query).
- `hitsToFacetValues` converts `FacetHit` to `FacetValue`, handling both string and integer variants.

### `internal/search/health.go` — HealthHandler

- **Qdrant check:** `client.HealthCheck(ctx)` via gRPC.
- **TEI check:** HTTP GET to `embeddingURL + "/health"` with a 5-second timeout via a dedicated `http.Client`.
- **Response:** `HealthResponse` with `status` ("ok"/"degraded"), per-component statuses, and appropriate HTTP code (200/503).

### `cmd/server/main.go` — HTTP server entrypoint

Wires all components:
- `config.Load()` → Qdrant client → TEI embedder → Redmine client → `PermissionCache` → `AuthMiddleware`
- Routes: `GET /api/v1/search` (behind auth middleware), `GET /api/v1/health` (public)
- `ReadTimeout: 10s`, `WriteTimeout: 30s`
- Graceful shutdown on SIGINT/SIGTERM with 15-second deadline via `signal.NotifyContext`

## Date Range Filtering

The `updated_on` field has a `FieldType_FieldTypeDatetime` index (created in Phase 1). Date range filtering uses `qdrant.DatetimeRange` with `timestamppb.Timestamp` values:
- `date_from` → `Gte` (start of day, as parsed)
- `date_to` → `Lte` (end of day: 23:59:59 UTC, appended automatically)

## Decisions Made

| Decision | Rationale |
|----------|-----------|
| `NewMatchInts` for project permission filter | Single condition covering all project IDs — avoids N separate Should conditions |
| Insertion sort after dedup | Result sets bounded by fetch limit; O(n^2) acceptable |
| Facet errors non-fatal | Search results are still useful without facets; avoids full request failure |
| Dedicated `http.Client` in HealthHandler | Isolates 5s timeout from default global client |
| `/server` added to .gitignore | Consistent with `/indexer` binary exclusion pattern |

## Deviations from Plan

None — plan executed exactly as written.

## Self-Check

Files created:
- [x] `internal/search/handler.go` — `go build ./internal/search/...` passes
- [x] `internal/search/facets.go` — same package, same build
- [x] `internal/search/health.go` — same package
- [x] `cmd/server/main.go` — `go build ./cmd/server/...` passes
- [x] `.gitignore` updated

Commits:
- [x] 27c0e7f — feat(02-05): implement search handler and facet aggregation
- [x] c72acbb — feat(02-05): health endpoint and HTTP server entrypoint

`go build ./...` and `go vet ./...` both pass across the full project.

## Self-Check: PASSED
