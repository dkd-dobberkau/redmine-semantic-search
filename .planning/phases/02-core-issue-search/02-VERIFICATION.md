---
phase: 02-core-issue-search
verified: 2026-02-18T18:30:00Z
status: passed
score: 18/18 must-haves verified
re_verification: false
gaps: []
human_verification:
  - test: "Submit a real search query via GET /api/v1/search?q=... with a valid X-Redmine-API-Key and verify results are ordered by relevance score"
    expected: "JSON response with results array sorted descending by score, each result has issue_id, subject, snippet, tracker, status, project_id, author"
    why_human: "Requires live Redmine, Qdrant, and TEI embedding service to assess actual ranking quality"
  - test: "Submit a search query as a non-admin user who does NOT own a private issue; verify that private issue is absent from results"
    expected: "Private issue authored by a different user_id does not appear in results"
    why_human: "Requires live services and actual private issue data to test post-filter path"
  - test: "Trigger a Redmine issue deletion, wait for reconciliation cycle; verify the issue's Qdrant points are removed"
    expected: "After reconciliation run, deleted issue's chunks are absent from Qdrant"
    why_human: "Requires live Redmine + Qdrant and waiting on the cron schedule (or manually triggering reconcile)"
---

# Phase 2: Core Issue Search Verification Report

**Phase Goal:** Users can submit a natural-language query and receive permission-filtered, relevance-ranked Redmine issues — and the index stays fresh through incremental sync with deletion reconciliation
**Verified:** 2026-02-18T18:30:00Z
**Status:** PASSED
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|---------|
| 1 | Redmine REST client fetches paginated issues with updated_on cursor, status_id=*, and sort=updated_on:asc | VERIFIED | `internal/redmine/issues.go:26-33` — `params.Set("updated_on", ">="+since.UTC().Format(time.RFC3339))`, `params.Set("status_id", "*")`, `params.Set("sort", "updated_on:asc")` |
| 2 | Redmine REST client validates an API key via GET /users/current.json and returns user identity + memberships | VERIFIED | `internal/redmine/client.go:98-107` — `GetCurrentUser` calls `/users/current.json?include=memberships` using user-supplied key |
| 3 | Redmine REST client fetches all projects accessible to a given API key via GET /projects.json | VERIFIED | `internal/redmine/client.go:113-136` — `ListProjects` paginates through all pages with `limit=100`, `offset` advancing until `offset >= total_count` |
| 4 | Textile/Markdown formatting is stripped to plain text preserving readable content | VERIFIED | `internal/text/strip.go:60-88` — 10 compiled regexps applied in order (pre blocks, HTML tags, headers, bold/italic/strike/code, links, whitespace normalization, TrimSpace) |
| 5 | Long texts are split into overlapping character chunks of ~1600 chars with ~200 char overlap | VERIFIED | `internal/text/chunk.go:6-46` — `ChunkSize=1600`, `ChunkOverlap=200`, rune-based sliding window, single chunk returned when text fits |
| 6 | Issue subject and description are stripped, chunked, embedded, and upserted to Qdrant with all required payload fields | VERIFIED | `internal/indexer/pipeline.go:61-152` — full strip→chunk→EmbedPassages→Upsert pipeline with 14 payload fields including `author_id`, `is_private`, `text_preview` |
| 7 | Multi-chunk issues produce one Qdrant point per chunk with deterministic UUID v5 IDs based on issue ID and chunk index | VERIFIED | `internal/qdrant/pointid.go:44-47` — `ChunkPointID` uses `uuid.NewSHA1(PointIDNamespace, "issue:<id>:chunk:<index>")` |
| 8 | Re-indexing an issue deletes all existing chunks before upserting new ones, preventing stale chunk orphans | VERIFIED | `internal/indexer/pipeline.go:74-76` — `DeleteIssueChunks(ctx, issue.ID)` called before each issue's chunks are added; uses `NewPointsSelectorFilter` with `content_type=issue AND redmine_id=N` |
| 9 | Config struct includes all new fields for sync polling, server listen address, and deletion reconciliation schedule | VERIFIED | `internal/config/config.go:36-54` — `SyncInterval`, `SyncBatchSize`, `ReconcileSchedule`, `ListenAddr`, `PermissionCacheTTL` all present with defaults wired at lines 81-87 |
| 10 | Incremental sync polls Redmine every N minutes, fetching only issues updated since the last cursor, and indexes them via the pipeline | VERIFIED | `internal/indexer/sync.go:59-141` — `Start` launches goroutine with immediate first poll, then ticker; `poll` calls `FetchIssuesSince` with cursor then `IndexIssues` |
| 11 | The updated_on cursor advances monotonically after each successful batch | VERIFIED | `internal/indexer/sync.go:130-131` — cursor advances only after both fetch AND index succeed; if either fails, cursor stays at prior value |
| 12 | Deletion reconciliation runs on a cron schedule, compares all Qdrant issue IDs against Redmine, and deletes orphaned points | VERIFIED | `internal/indexer/reconcile.go:82-185` — `FetchAllIssueIDs` → build set → `ScrollAndOffset` cursor-based scroll → batch delete orphans with `NewPointsSelectorIDs` |
| 13 | The indexer process starts serving immediately with an empty or partial index — it does NOT block on a full sync | VERIFIED | `cmd/indexer/main.go:87-88` — `syncer.Start(ctx)` is non-blocking; first poll runs in background goroutine; process is immediately ready |
| 14 | A request with a valid X-Redmine-API-Key header passes through with user identity and project_ids injected into context | VERIFIED | `internal/auth/middleware.go:51-76` — extracts header, calls `cache.Resolve`, injects `UserPermissions` into context via `context.WithValue` |
| 15 | A request without an API key or with an invalid key receives a 401 JSON error response | VERIFIED | `internal/auth/middleware.go:54-56, 61-64` — empty header → 401 "missing X-Redmine-API-Key header"; `ErrUnauthorized` → 401 "invalid API key" |
| 16 | Permission lookups are cached with configurable TTL and concurrent requests coalesce via singleflight | VERIFIED | `internal/auth/permissions.go:66-93` — RWMutex fast-path cache hit check; `sf.Do(apiKey, ...)` coalesces concurrent misses; write lock stores result with `expiresAt` |
| 17 | GET /api/v1/search embeds the query, runs permission-filtered ANN, post-filters private issues, deduplicates multi-chunk results, and returns paginated JSON with facet counts and snippets | VERIFIED | `internal/search/handler.go:72-239` — full pipeline present: embed→Query with project_id filter→post-filter is_private by author_id→dedup by redmine_id→paginate→FetchFacets→JSON encode |
| 18 | GET /api/v1/health returns JSON status of Qdrant and embedding service connectivity | VERIFIED | `internal/search/health.go:57-81` — checks `qdrant.HealthCheck` and HTTP GET to `embeddingURL+"/health"` with 5s timeout; returns "ok"/"degraded" with 200/503 |

**Score:** 18/18 truths verified

### Required Artifacts

| Artifact | Provided By | Lines | Status | Details |
|----------|------------|-------|--------|---------|
| `internal/redmine/client.go` | Plan 02-01 | 137 | VERIFIED | Base HTTP client with ErrUnauthorized/ErrNotFound sentinels, doJSON with per-call apiKey parameter, GetCurrentUser, ListProjects |
| `internal/redmine/issues.go` | Plan 02-01 | 76 | VERIFIED | FetchIssuesSince with cursor+status_id=*+sort, FetchAllIssueIDs paginating all issues |
| `internal/redmine/models.go` | Plan 02-01 | 72 | VERIFIED | IDRef, Issue, IssueList, User, Membership, UserResponse, Project, ProjectList structs |
| `internal/text/strip.go` | Plan 02-01 | 88 | VERIFIED | StripMarkup with 10 compiled regexps, correct processing order |
| `internal/text/chunk.go` | Plan 02-01 | 46 | VERIFIED | ChunkText with ChunkSize=1600, ChunkOverlap=200, rune-based Unicode handling |
| `internal/indexer/pipeline.go` | Plan 02-02 | 195 | VERIFIED | IndexIssues (strip+chunk+embed+upsert), DeleteIssueChunks (filter delete), 14 payload fields including author_id |
| `internal/qdrant/pointid.go` | Plan 02-02 | 47 | VERIFIED | ChunkPointID added alongside existing PointID, UUID v5 key format "issue:<id>:chunk:<N>" |
| `internal/config/config.go` | Plan 02-02 | 143 | VERIFIED | 5 new fields (SyncInterval, SyncBatchSize, ReconcileSchedule, ListenAddr, PermissionCacheTTL) with defaults |
| `config.example.yml` | Plan 02-02 | present | VERIFIED | Documented new sync/server fields per SUMMARY |
| `internal/indexer/sync.go` | Plan 02-03 | 160 | VERIFIED | Syncer with Start/Stop lifecycle, poll with bounded page, cursor advancement only on success |
| `internal/indexer/reconcile.go` | Plan 02-03 | 185 | VERIFIED | Reconciler with cron, ScrollAndOffset cursor pagination, batch orphan deletion |
| `cmd/indexer/main.go` | Plan 02-03 | 105 | VERIFIED | Full wiring: config→Qdrant→EnsureCollection→embedder→redmine→pipeline→syncer→reconciler→signals |
| `internal/auth/permissions.go` | Plan 02-04 | 138 | VERIFIED | PermissionCache with RWMutex+singleflight+TTL, UserPermissions with int64 ProjectIDs |
| `internal/auth/middleware.go` | Plan 02-04 | 86 | VERIFIED | AuthMiddleware.Wrap extracting X-Redmine-API-Key, 401/503 error responses, UserFromContext accessor |
| `internal/search/handler.go` | Plan 02-05 | 380 | VERIFIED | Full search flow with NewMatchInts permission pre-filter, post-filter on author_id, chunk dedup, pagination |
| `internal/search/facets.go` | Plan 02-05 | 118 | VERIFIED | FetchFacets with 4 concurrent Facet calls all using same permission filter |
| `internal/search/health.go` | Plan 02-05 | 123 | VERIFIED | HealthHandler checking Qdrant (gRPC HealthCheck) and TEI (HTTP GET /health, 5s timeout) |
| `cmd/server/main.go` | Plan 02-05 | 109 | VERIFIED | HTTP server: GET /api/v1/search behind auth, GET /api/v1/health public, 15s graceful shutdown |

### Key Link Verification

| From | To | Via | Status | Evidence |
|------|-----|-----|--------|---------|
| `internal/redmine/issues.go` | `internal/redmine/client.go` | `doJSONWithAdminKey` | WIRED | `issues.go:36` — `c.doJSONWithAdminKey(ctx, "/issues.json", params, &list)` |
| `internal/indexer/pipeline.go` | `internal/text/strip.go` | `text.StripMarkup` | WIRED | `pipeline.go:179` — `stripped := text.StripMarkup(issue.Description)` |
| `internal/indexer/pipeline.go` | `internal/text/chunk.go` | `text.ChunkText` | WIRED | `pipeline.go:70` — `chunks := text.ChunkText(fullText)` |
| `internal/indexer/pipeline.go` | `internal/embedder/embedder.go` | `embedder.EmbedPassages` | WIRED | `pipeline.go:100` — `embeddings, err := p.embedder.EmbedPassages(ctx, chunkTexts)` |
| `internal/indexer/pipeline.go` | `internal/qdrant/pointid.go` | `qdrantpkg.ChunkPointID` | WIRED | `pipeline.go:111` — `chunkID := qdrantpkg.ChunkPointID(e.issue.ID, e.chunkIndex)` |
| `internal/indexer/sync.go` | `internal/redmine/issues.go` | `redmineClient.FetchIssuesSince` | WIRED | `sync.go:105` — `s.redmine.FetchIssuesSince(ctx, s.cursor, 0, s.batch)` |
| `internal/indexer/sync.go` | `internal/indexer/pipeline.go` | `pipeline.IndexIssues` | WIRED | `sync.go:120` — `s.pipeline.IndexIssues(ctx, issueList.Issues)` |
| `internal/indexer/reconcile.go` | `internal/redmine/issues.go` | `redmineClient.FetchAllIssueIDs` | WIRED | `reconcile.go:86` — `r.redmine.FetchAllIssueIDs(ctx)` |
| `cmd/indexer/main.go` | `internal/indexer/sync.go` | `syncer.Start` | WIRED | `main.go:87,101` — `syncer.Start(ctx)` and `syncer.Stop()` |
| `internal/auth/permissions.go` | `internal/redmine/client.go` | `redmineClient.GetCurrentUser` | WIRED | `permissions.go:103` — `c.redmine.GetCurrentUser(ctx, apiKey)` |
| `internal/auth/permissions.go` | `internal/redmine/client.go` | `redmineClient.ListProjects` | WIRED | `permissions.go:113` — `c.redmine.ListProjects(ctx, apiKey)` |
| `internal/auth/middleware.go` | `internal/auth/permissions.go` | `cache.Resolve` | WIRED | `middleware.go:59` — `m.cache.Resolve(r.Context(), apiKey)` |
| `internal/search/handler.go` | `internal/embedder/embedder.go` | `embedder.EmbedQuery` | WIRED | `handler.go:130` — `h.embedder.EmbedQuery(ctx, q)` |
| `internal/search/handler.go` | `internal/auth/middleware.go` | `auth.UserFromContext` | WIRED | `handler.go:119` — `user := auth.UserFromContext(ctx)` |
| `internal/search/facets.go` | `internal/qdrant/collection.go` | `client.Facet` with `AliasName` | WIRED | `facets.go:50-55` — `client.Facet(ctx, &qdrant.FacetCounts{CollectionName: qdrantpkg.AliasName, ...})` |
| `cmd/server/main.go` | `internal/auth/middleware.go` | `authMiddleware.Wrap` | WIRED | `main.go:74` — `mux.Handle("GET /api/v1/search", authMiddleware.Wrap(searchHandler))` |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|---------|
| IDX-01 | 02-02 | Issue-Indexierung — Titel, Beschreibung und Custom Fields als Vektoren indexiert | SATISFIED | `pipeline.go` strips+chunks+embeds+upserts issues with 14 payload fields; `go build ./...` passes |
| IDX-04 | 02-03 | Inkrementelle Updates — Geänderte Objekte über `updated_on` erkannt und nachindexiert | SATISFIED | `sync.go` polls with `updated_on>=cursor`, advances cursor after successful index |
| IDX-06 | 02-03 | Löschsynchronisation — Gelöschte Issues aus dem Index entfernt (ID-Reconciliation) | SATISFIED | `reconcile.go` full ID diff: scrolls Qdrant, fetches all Redmine IDs, batch-deletes orphans |
| IDX-07 | 02-01 | Textaufbereitung — Textile/Markdown zu Plaintext, Texte in Chunks aufgeteilt | SATISFIED | `strip.go` (10 regexp patterns), `chunk.go` (1600-char/200-overlap rune-based chunking) |
| SRCH-01 | 02-05 | Semantische Suche — Anfragen vektorisiert, per Cosine Similarity gegen Qdrant abgeglichen | SATISFIED | `handler.go` embeds query via `EmbedQuery`, queries Qdrant with `NewQueryDense(queryVec)`, results sorted by score |
| SRCH-03 | 02-05 | Facettierte Filter — Einschränkung nach Projekt, Tracker, Status, Autor, Zeitraum | SATISFIED | `handler.go` `buildPermissionFilter` handles tracker/status/project/author keyword filters and `date_from`/`date_to` DatetimeRange; `facets.go` returns tracker/status/project/author counts |
| SRCH-04 | 02-05 | Paginierung — Default 20, Max 100 | SATISFIED | `handler.go:81-91` — `defaultPerPage=20`, `maxPerPage=100`, clamped; sliced after dedup |
| SRCH-05 | 02-05 | Snippet-Generierung — Textausschnitt mit relevantesten Passagen | SATISFIED | `handler.go:216` — `truncateSnippet(extractPayloadString(pt, "text_preview"), snippetMaxLen)` where `snippetMaxLen=150` |
| AUTH-01 | 02-04, 02-05 | Permission Pre-Filtering — `project_ids` als Qdrant-Filter übergeben | SATISFIED | `handler.go:254` — `qdrant.NewMatchInts("project_id", projectIDs...)` in Must filter |
| AUTH-02 | 02-04 | API-Authentifizierung — Anfragen über Redmine API-Keys authentifiziert | SATISFIED | `middleware.go` extracts `X-Redmine-API-Key`, validates via `PermissionCache` which calls Redmine |
| AUTH-03 | 02-04, 02-05 | Post-Filtering — private Issues nach Qdrant-Abfrage geprüft (Oversampling) | SATISFIED | `handler.go:157-166` — post-filters `is_private=true` issues where `author_id != user.UserID` and `!user.IsAdmin`; oversampling via `fetchLimit = page * perPage * 2` |
| API-01 | 02-05 | REST Search Endpoint — `GET /api/v1/search` mit Query, Filter, Paginierung, JSON | SATISFIED | `cmd/server/main.go:74` — registered; `handler.go` implements full spec |
| API-02 | 02-05 | Health Endpoint — `GET /api/v1/health` liefert Status Qdrant + Embedding-Service | SATISFIED | `cmd/server/main.go:75` — registered (public); `health.go` checks both components |

**All 13 requirement IDs from Phase 2 plans are accounted for and satisfied.**

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/text/chunk.go:28` | 28 | `return []string{text}` | INFO | Correct optimization: returns single chunk when text fits in one window |
| `internal/search/facets.go:97` | 97 | `return []FacetValue{}` | INFO | Correct guard: returns empty slice (not nil) when no facet hits |

No blockers or warnings found. Both flagged patterns are correct implementations.

### Human Verification Required

#### 1. End-to-End Semantic Search Quality

**Test:** Send `GET /api/v1/search?q=<natural language query>` with a valid `X-Redmine-API-Key` header against a live Redmine+Qdrant+TEI stack.
**Expected:** Results sorted descending by cosine similarity score; each result contains `issue_id`, `subject`, `snippet`, `tracker`, `status`, `project_id`, `author`, and `facets` object.
**Why human:** Requires all live services and actual indexed data. Ranking quality cannot be verified by static code inspection.

#### 2. Private Issue Post-Filtering

**Test:** Index a private Redmine issue authored by user A. Query as user B (non-admin, different user ID). Verify the private issue does not appear.
**Expected:** Private issue absent from results for user B; visible to user A and admin users.
**Why human:** Requires live Redmine with real private issues and multiple user API keys to exercise the `author_id != user.UserID` post-filter path.

#### 3. Deletion Reconciliation End-to-End

**Test:** Delete an issue in Redmine that is indexed in Qdrant. Wait for reconciliation cycle (or manually shorten schedule) and verify Qdrant points are removed.
**Expected:** After reconciliation, `GET /api/v1/search` no longer returns the deleted issue.
**Why human:** Requires live Redmine deletion, live Qdrant, and waiting on cron schedule.

### Build Verification

```
go build ./...   # PASSED — no output (clean)
go vet ./...     # PASSED — no output (clean)
```

Both commands executed without errors or warnings across the full project.

### Gaps Summary

No gaps. All 18 observable truths verified. All 15 artifacts present, substantive, and wired. All 16 key links confirmed in source code. All 13 requirement IDs from Phase 2 plans are satisfied. No blocker or warning anti-patterns detected. Build and vet pass clean.

---

_Verified: 2026-02-18T18:30:00Z_
_Verifier: Claude (gsd-verifier)_
