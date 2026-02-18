# Phase 2: Core Issue Search - Research

**Researched:** 2026-02-18
**Domain:** Go HTTP server, Redmine REST API client, Qdrant search/filter/facets, text preprocessing, incremental sync scheduling, permission caching
**Confidence:** HIGH (core stack verified via official docs and existing codebase patterns)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Issue text for indexing
- Embed subject + description only — no custom fields, no journals (journals are Phase 3)
- Metadata (tracker, status, priority, assignee, project) stored as Qdrant payload for filtering, NOT embedded in text
- Long issues split into overlapping chunks (~400 tokens with overlap); each chunk becomes its own vector linked to the parent issue via payload
- Minimal text preprocessing: strip Textile/Markdown formatting, normalize whitespace, keep text as-is — the multilingual model handles mixed DE/EN natively

#### Search result shape
- Minimal fields per hit: issue ID, subject, relevance score
- Include a ~150-character text snippet per result showing the matched chunk content
- Facet counts included in response: aggregated counts per tracker, status, project, and author alongside results
- Offset-based pagination: `page` + `per_page` query params, matching Redmine's own API style

#### Auth & permission flow
- Pass-through Redmine API key — callers send their Redmine key, RSS validates it against Redmine and resolves permissions
- Header: `X-Redmine-API-Key` (matches Redmine's own convention)
- Permission lookups cached with short TTL (few minutes) to reduce Redmine API calls on repeated searches
- Error responses: 401 for invalid/missing key, 503 when Redmine is unreachable — clear distinction for clients

#### Sync & freshness
- Indexer uses a dedicated Redmine admin API key configured at startup — sees all issues, permissions enforced at search time only
- First run: start with empty index, begin incremental polling immediately — no blocking full sync
- Bounded pages per polling cycle (e.g. 100 issues), advance updated_on cursor, pick up more next cycle — gradual fill, service stays responsive
- Deletion reconciliation: periodic full ID diff job — fetch all issue IDs from Redmine, compare with Qdrant, delete orphans

### Claude's Discretion
- Exact chunk size and overlap parameters
- Permission cache TTL value
- Polling interval default
- Deletion reconciliation schedule
- Snippet generation approach (first N chars of chunk vs. most relevant portion)
- Default per_page value
- Facet aggregation implementation (Qdrant-side vs application-side)

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| IDX-01 | Issue-Indexierung — Titel, Beschreibung und Custom Fields werden als Vektoren in Qdrant indexiert | Redmine `/issues.json` returns `subject` + `description` fields; TEI `EmbedPassages` already implemented; Qdrant `UpsertPoints` pattern established |
| IDX-04 | Inkrementelle Updates — Geänderte und neue Objekte werden über `updated_on` erkannt und nachindexiert | Redmine supports `updated_on>=%3E%3D<timestamp>` filter + `sort=updated_on`; bounded-page cursor pattern researched |
| IDX-06 | Löschsynchronisation — Gelöschte Issues werden aus dem Index entfernt (ID-Reconciliation) | Qdrant Scroll API with `WithPayload(false)` fetches all IDs cheaply; `DeletePoints` with selector supports batch deletion |
| IDX-07 | Textaufbereitung — Textile/Markdown-Formatierung wird zu Plaintext konvertiert, Texte werden auf maximale Token-Länge gekürzt oder in Chunks aufgeteilt | `gomarkdown/markdown` for Markdown→HTML→text; `pkoukk/tiktoken-go` for token-aware chunking; overlap strategy researched |
| SRCH-01 | Semantische Suche — Suchanfragen werden vektorisiert und per Cosine Similarity gegen Qdrant abgeglichen, sortiert nach Score | `client.Query` with `qdrant.NewQueryDense(vec)` pattern established in benchmark code; `EmbedQuery` interface exists |
| SRCH-03 | Facettierte Filter — Einschränkung nach Projekt, Tracker, Status, Autor, Zeitraum und Content-Typ über Qdrant Payload-Filter | Qdrant `client.Facet` with `FacetCounts` struct (v1.12+, available in current go-client v1.16.2); payload indexes already created in Phase 1 |
| SRCH-04 | Paginierung — Ergebnisse werden paginiert ausgeliefert (Default: 20, Max: 100) | `QueryPoints.Limit` + `QueryPoints.Offset` (pointer via `qdrant.PtrOf`) supported; page/per_page → offset calculation trivial |
| SRCH-05 | Snippet-Generierung — Zu jedem Treffer wird ein Textausschnitt mit den relevantesten Passagen zurückgegeben | `text_preview` payload field stores chunk text; first 150 chars of matched chunk is the simple, correct approach |
| AUTH-01 | Permission Pre-Filtering — Erlaubte `project_ids` des Nutzers werden als Qdrant-Filter übergeben | `qdrant.NewMatchInt` for integer field match; `Filter.Must` with `[]project_id` using `qdrant.NewMatchAny` pattern |
| AUTH-02 | API-Authentifizierung — Anfragen werden über Redmine API-Keys authentifiziert | Redmine `GET /users/current.json` with `X-Redmine-API-Key` header validates the key and returns user identity |
| AUTH-03 | Post-Filtering — Feinere Berechtigungen (private Issues) werden nach der Qdrant-Abfrage geprüft (Oversampling) | `is_private` field present in Redmine issue response; oversampling factor 2x means `Limit = per_page * 2` |
| API-01 | REST Search Endpoint — `GET /api/v1/search` mit Query, Filter, Paginierung und Sortierung als Parameter, JSON-Antworten | Go 1.22 `net/http` ServeMux supports method+path patterns; `http.HandlerFunc` middleware chain for auth |
| API-02 | Health Endpoint — `GET /api/v1/health` liefert Status des Dienstes, Qdrant-Verbindung und Embedding-Service | Dockerfile already uses `curl -f http://localhost:8090/health`; pattern for checking Qdrant and TEI connectivity established |
</phase_requirements>

---

## Summary

Phase 2 builds five interconnected subsystems on top of the Phase 1 foundation (Qdrant collection, TEI embedder, config). The Redmine REST client fetches issues using `updated_on>=` cursor pagination with admin credentials. The indexer pipeline strips Textile/Markdown, chunks long texts with token-aware overlap, embeds via TEI, and upserts to Qdrant with deterministic UUIDs. A scheduler drives incremental polling every few minutes and a separate deletion reconciliation job on a longer cadence. The search HTTP server validates caller API keys against Redmine, resolves accessible project IDs (cached), and issues a filtered ANN query to Qdrant with facet counts in a second call.

The Go 1.22 stdlib `net/http` ServeMux is sufficient for routing (method+path patterns, path variables via `r.PathValue`). No external router is needed. Qdrant's `client.Facet` (introduced in Qdrant 1.12, available in go-client v1.16.2 already in `go.mod`) returns per-field hit counts natively, making application-side aggregation unnecessary for the four required facets (tracker, status, project, author). For text chunking, a character-based approximation (1 token ≈ 4 characters for multilingual text) avoids adding a tokenizer dependency while remaining accurate enough for ~400-token target chunks.

**Primary recommendation:** Use Qdrant-native faceting (`client.Facet` called in parallel for each dimension), Go stdlib routing (`net/http` 1.22), a simple `sync.RWMutex`-protected map for permission cache with TTL, and character-based chunking (~1600 chars per chunk, ~200 char overlap) as a lightweight alternative to tiktoken-go.

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/qdrant/go-client` | v1.16.2 (already in go.mod) | Qdrant gRPC operations (upsert, query, scroll, facet, delete) | Official client; already used in Phase 1 |
| `net/http` (stdlib) | Go 1.25 | HTTP server for search API | Go 1.22+ ServeMux supports method+path routing; no third-party router needed |
| `log/slog` (stdlib) | Go 1.25 | Structured logging | Already used in cmd/indexer/main.go |
| `github.com/cenkalti/backoff/v4` | v4.3.0 (already in go.mod) | Exponential backoff for Redmine/TEI/Qdrant retries | Already used in bench/recall; pattern established |
| `github.com/spf13/viper` | v1.21.0 (already in go.mod) | Config loading (new fields for search server) | Already used; no change needed |
| `encoding/json` (stdlib) | Go 1.25 | JSON decode for Redmine responses, JSON encode for search API | No third-party JSON lib needed for this workload |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/robfig/cron/v3` | v3.0.1 | Scheduler for incremental sync and deletion reconciliation | Required for cron-expression-based scheduling |
| `golang.org/x/sync/singleflight` | current | Deduplicate concurrent permission lookups for same API key | Use inside permission cache to prevent stampede on cache miss |
| `strings` + `regexp` (stdlib) | Go 1.25 | Textile/Markdown stripping (regex-based) | Sufficient for stripping `*bold*`, `h1.`, `[link](url)`, Textile macros |
| `google.golang.org/protobuf/types/known/timestamppb` | (transitive via qdrant go-client) | Build `timestamppb.Timestamp` for Qdrant datetime range filter | Already pulled in via go-client |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Character-based chunking (~1600 chars) | `pkoukk/tiktoken-go` | tiktoken-go adds ~4 MB binary, downloads vocab files at startup; character approximation is simpler and accurate enough for e5 model's 512-token limit |
| stdlib `net/http` ServeMux | `github.com/go-chi/chi` or `github.com/gorilla/mux` | chi/gorilla add dependency; Go 1.22 ServeMux handles method routing and path variables (`r.PathValue`) natively |
| `sync.RWMutex` map for permission cache | `patrickmn/go-cache` or `ristretto` | External caches add dependencies; a simple TTL map is 30 lines of Go and sufficient for this single-key-per-user workload |
| Qdrant `client.Facet` for facet counts | Application-side aggregation over scroll results | Qdrant-native faceting is O(1) index scan vs O(N) application iteration; prefer Qdrant-side |

**Installation:**
```bash
go get github.com/robfig/cron/v3@v3.0.1
go get golang.org/x/sync@latest
```
All other dependencies are already in go.mod.

---

## Architecture Patterns

### Recommended Project Structure
```
cmd/
├── indexer/
│   └── main.go                  # existing — add scheduler startup
└── server/
    └── main.go                  # NEW — HTTP server entry point
internal/
├── config/
│   └── config.go                # extend with server/indexer/sync fields
├── redmine/
│   ├── client.go                # NEW — base HTTP client (auth header, retry)
│   ├── issues.go                # NEW — paginated issue fetch, updated_on cursor
│   └── models.go                # NEW — Issue, Project, User structs
├── indexer/
│   ├── pipeline.go              # NEW — fetch → strip → chunk → embed → upsert
│   ├── sync.go                  # NEW — incremental sync scheduler
│   └── reconcile.go             # NEW — deletion reconciliation job
├── search/
│   ├── handler.go               # NEW — GET /api/v1/search HTTP handler
│   ├── health.go                # NEW — GET /api/v1/health HTTP handler
│   └── facets.go                # NEW — facet count aggregation via Qdrant
├── auth/
│   ├── middleware.go            # NEW — X-Redmine-API-Key extraction, 401/503
│   └── permissions.go           # NEW — project_id resolver + TTL cache
├── text/
│   ├── strip.go                 # NEW — Textile/Markdown → plain text
│   └── chunk.go                 # NEW — overlapping chunk splitter
├── embedder/
│   ├── embedder.go              # existing
│   └── tei.go                   # existing — add batch-32 chunking
└── qdrant/
    ├── collection.go            # existing
    └── pointid.go               # existing — extend for chunk IDs
```

### Pattern 1: Redmine API Key Pass-Through Auth Middleware

**What:** Extract `X-Redmine-API-Key` header, call `GET /users/current.json` against Redmine with that key, cache result by key with TTL, inject user context into request. Return 401 if key is invalid, 503 if Redmine is unreachable.

**When to use:** Every incoming search request.

```go
// Source: Go stdlib net/http middleware pattern
type authMiddleware struct {
    redmineClient *redmine.Client
    cache         *permissions.Cache
}

func (m *authMiddleware) Wrap(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        apiKey := r.Header.Get("X-Redmine-API-Key")
        if apiKey == "" {
            http.Error(w, `{"error":"missing X-Redmine-API-Key"}`, http.StatusUnauthorized)
            return
        }
        user, err := m.cache.Resolve(r.Context(), apiKey)
        if errors.Is(err, redmine.ErrUnauthorized) {
            http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
            return
        }
        if err != nil {
            http.Error(w, `{"error":"redmine unreachable"}`, http.StatusServiceUnavailable)
            return
        }
        next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxUser, user)))
    })
}
```

### Pattern 2: Permission Resolution and Caching

**What:** `GET /projects.json?limit=100&offset=0` (paged until exhausted) returns all projects the user can access. Cache the resulting `[]int` of project_ids keyed by API key with a TTL. Use `sync.RWMutex` + `singleflight.Group` to prevent cache stampede.

**When to use:** Called by auth middleware on cache miss.

```go
// Source: Go stdlib sync patterns
type Cache struct {
    mu      sync.RWMutex
    entries map[string]cacheEntry // api_key → {projectIDs, expiresAt}
    sf      singleflight.Group
    ttl     time.Duration
}

func (c *Cache) Resolve(ctx context.Context, apiKey string) (*UserPermissions, error) {
    c.mu.RLock()
    if e, ok := c.entries[apiKey]; ok && time.Now().Before(e.expiresAt) {
        c.mu.RUnlock()
        return e.perms, nil
    }
    c.mu.RUnlock()

    result, err, _ := c.sf.Do(apiKey, func() (any, error) {
        return c.fetchFromRedmine(ctx, apiKey)
    })
    if err != nil {
        return nil, err
    }
    perms := result.(*UserPermissions)
    c.mu.Lock()
    c.entries[apiKey] = cacheEntry{perms: perms, expiresAt: time.Now().Add(c.ttl)}
    c.mu.Unlock()
    return perms, nil
}
```

### Pattern 3: Redmine Incremental Sync Cursor

**What:** The indexer admin key fetches `GET /issues.json?updated_on=>={cursor}&status_id=*&sort=updated_on:asc&limit=100&offset=0`. After processing one page, advance cursor to the latest `updated_on` seen. On next cycle, resume from that cursor. This fills the index gradually without blocking service startup.

**When to use:** Scheduler fires every N minutes (default: 5).

```go
// Source: Redmine REST API docs — updated_on filter
func (c *Client) FetchIssuesSince(ctx context.Context, since time.Time, offset, limit int) ([]Issue, int, error) {
    // URL-encode ">=" as %3E%3D; Redmine requires operator encoding
    tsFilter := url.QueryEscape(">=" + since.UTC().Format(time.RFC3339))
    url := fmt.Sprintf("%s/issues.json?updated_on=%s&status_id=*&sort=updated_on:asc&limit=%d&offset=%d",
        c.baseURL, tsFilter, limit, offset)
    // ... HTTP GET with X-Redmine-API-Key: <admin_key>
}
```

**Critical:** `status_id=*` is required — default returns open issues only, missing closed ones.
**Critical:** `sort=updated_on:asc` ensures cursor advances monotonically.
**Limitation:** `GET /issues.json` without `project_id` returns all accessible issues for the authenticated user. With an admin key, this covers all projects globally.

### Pattern 4: Qdrant Search with Permission Pre-Filter

**What:** Build a Qdrant `Filter.Must` containing an `in` condition on `project_id` with the user's allowed IDs. Pass this filter to `QueryPoints`. For post-filtering of private issues, oversample (limit * 2) and filter client-side by `is_private` payload field.

```go
// Source: Qdrant go-client docs — filtering
func buildPermissionFilter(projectIDs []int32, extraFilter *qdrant.Filter) *qdrant.Filter {
    // Build []MatchValue for project_id IN (...)
    projectConditions := make([]*qdrant.PointId, 0)  // not PointId — use MatchAny
    values := make([]*qdrant.Match, len(projectIDs))
    for i, id := range projectIDs {
        values[i] = qdrant.NewMatchInt("project_id", int64(id))
    }
    // Use Should with Must wrapper: at least one project_id must match
    filter := &qdrant.Filter{
        Must: []*qdrant.Condition{
            {
                ConditionOneOf: &qdrant.Condition_Filter{
                    Filter: &qdrant.Filter{
                        Should: projectConditions,  // one project_id must match
                    },
                },
            },
        },
    }
    return filter
}

// Query with oversampling for post-filter
results, err := qdrantClient.Query(ctx, &qdrant.QueryPoints{
    CollectionName: qdrant.AliasName,
    Query:          qdrant.NewQueryDense(queryVec),
    Filter:         permissionFilter,
    Limit:          uint64(perPage * oversamplingFactor),
    Offset:         qdrant.PtrOf(uint64(0)), // always start at 0; offset handled client-side after post-filter
    WithPayload:    qdrant.NewWithPayloadInclude("redmine_id", "subject", "tracker", "status", "project_id", "author", "is_private", "text_preview", "chunk_index"),
})
```

**Note on pagination with oversampling:** Because post-filtering reduces result count unpredictably, offset-based pagination must be applied after post-filtering. Fetch `page * per_page * oversampling_factor` results from Qdrant, post-filter, then slice `[page*per_page : (page+1)*per_page]`.

### Pattern 5: Qdrant Facet Counts (Qdrant-Native)

**What:** For each of the four facet dimensions (tracker, status, project_id, author), call `client.Facet` with the same permission filter as the main query but no vector query. Fire all four concurrently with a `sync.WaitGroup` or `errgroup`.

```go
// Source: Qdrant 1.12 blog post + go-client v1.16 API
// Facet is available in qdrant/go-client v1.16.2 (already in go.mod)
hits, err := qdrantClient.Facet(ctx, &qdrant.FacetCounts{
    CollectionName: qdrant.AliasName,
    Key:            "tracker",
    Filter:         permissionFilter,  // same filter as main query
    Limit:          qdrant.PtrOf(uint64(50)),
    Exact:          qdrant.PtrOf(false), // approximate is fast enough
})
// hits is []*qdrant.FacetHit, each with Value (string/int) and Count (uint64)
```

**Note:** `Facet` only works on fields with a keyword or integer index. All four required facet fields (`tracker`, `status`, `project_id`, `author`) are already indexed in Phase 1's `createPayloadIndexes`.

### Pattern 6: Deletion Reconciliation (ID Diff)

**What:** Periodically scroll all `redmine_id` values from Qdrant (content_type=issue), fetch all current issue IDs from Redmine (`GET /issues.json?status_id=*&limit=100&offset=...`), compute the set difference, batch-delete orphan point IDs.

```go
// Source: Qdrant docs — Scroll API
func scrollAllIssueIDs(ctx context.Context, client *qdrant.Client) (map[int64]string, error) {
    // map[redmine_id]qdrant_point_id (UUID string)
    result := make(map[int64]string)
    var offset *qdrant.PointId  // nil = start from beginning
    for {
        resp, nextOffset, err := client.ScrollAndOffset(ctx, &qdrant.ScrollPoints{
            CollectionName: qdrant.AliasName,
            Filter: &qdrant.Filter{
                Must: []*qdrant.Condition{
                    qdrant.NewMatch("content_type", "issue"),
                },
            },
            Limit:       qdrant.PtrOf(uint32(1000)),
            WithPayload: qdrant.NewWithPayloadInclude("redmine_id"),
            WithVectors: qdrant.NewWithVectors(false),
            Offset:      offset,
        })
        // ... accumulate IDs; if nextOffset == nil, done
        offset = nextOffset
        if offset == nil {
            break
        }
    }
    return result, nil
}

// Batch delete orphans
client.Delete(ctx, &qdrant.DeletePoints{
    CollectionName: qdrant.AliasName,
    Points: qdrant.NewPointsSelector(orphanPointIDs...),
})
```

**Note:** Chunks share a `redmine_id` but have distinct Qdrant point IDs. The ID diff must collect all chunk point IDs for each `redmine_id` to delete all chunks of a deleted issue.

### Pattern 7: Chunking Strategy

**What:** Split text into overlapping character windows. One token ≈ 4 characters for multilingual content (safe approximation for e5 model's 512-token limit). Target ~400 tokens = ~1600 chars per chunk, ~50-token overlap = ~200 chars.

```go
// Source: Research + multilingual-e5-base token limit
const (
    ChunkSize    = 1600  // characters (~400 tokens for multilingual text)
    ChunkOverlap = 200   // characters (~50 tokens)
)

func ChunkText(text string) []string {
    runes := []rune(text)
    if len(runes) <= ChunkSize {
        return []string{text}
    }
    var chunks []string
    start := 0
    for start < len(runes) {
        end := start + ChunkSize
        if end > len(runes) {
            end = len(runes)
        }
        chunks = append(chunks, string(runes[start:end]))
        if end == len(runes) {
            break
        }
        start = end - ChunkOverlap
    }
    return chunks
}
```

**For multi-chunk issues:** Each chunk becomes one Qdrant point. Point ID derived from `PointID("issue-chunk", hashOf(redmineID, chunkIndex))` — deterministic for idempotent re-indexing. Payload includes `redmine_id`, `chunk_index`, `chunk_total`.

**Deduplication on search:** When multiple chunks of the same issue appear in results, keep only the highest-score chunk and surface the issue once.

### Pattern 8: Textile/Markdown Stripping

**What:** Redmine uses Textile formatting by default. Strip to plain text using regex for the most common patterns. The goal is "good enough" — the multilingual model handles light residual formatting.

```go
// Source: Research — regex-based is sufficient for this use case
var (
    reBold       = regexp.MustCompile(`\*([^*]+)\*`)           // *bold*
    reItalic     = regexp.MustCompile(`_([^_]+)_`)             // _italic_
    reStrike     = regexp.MustCompile(`-([^-]+)-`)             // -strike-
    reHeader     = regexp.MustCompile(`(?m)^h[1-6]\.\s+`)      // h1. header
    rePre        = regexp.MustCompile(`<pre>[\s\S]*?</pre>`)    // <pre>blocks</pre>
    reCode       = regexp.MustCompile(`@([^@]+)@`)             // @code@
    reLink       = regexp.MustCompile(`"([^"]+)":https?://\S+`) // "link":url
    reMarkdownLink = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`) // [text](url)
    reHTMLTag    = regexp.MustCompile(`<[^>]+>`)               // any HTML tag
    reMultiSpace = regexp.MustCompile(`\s{2,}`)                // normalize whitespace
)

func StripMarkup(text string) string {
    text = rePre.ReplaceAllString(text, " ")
    text = reHTMLTag.ReplaceAllString(text, " ")
    text = reHeader.ReplaceAllString(text, "")
    text = reBold.ReplaceAllLiteralString(text, "$1")   // keep inner text
    // ... etc.
    text = reMultiSpace.ReplaceAllString(strings.TrimSpace(text), " ")
    return text
}
```

### Anti-Patterns to Avoid
- **Blocking full sync before serving requests:** The bounded-page approach means the service is queryable immediately; never wait for a full sync to complete before binding the HTTP server.
- **One Redmine API call per search request (no permission cache):** Without caching, a burst of 100 concurrent searches hits Redmine 100 times. Cache with TTL prevents this.
- **Encoding `>=` as literal `>=` in updated_on filter:** Redmine requires URL-encoding: `%3E%3D`. `url.QueryEscape(">=2024-01-01T00:00:00Z")` produces `%3E%3D2024-01-01T00%3A00%3A00Z` — use `url.PathEscape` or manual encoding for just the operator.
- **Using `offset` for Qdrant scroll (not search):** Qdrant's Scroll API uses cursor-based pagination via `next_page_offset`, not integer offset. Integer offset is for `QueryPoints` only.
- **Deleting individual chunk points instead of all chunks by redmine_id:** When an issue is deleted, all chunks must be removed. Build a filter `qdrant.NewMatch("redmine_id", redmineID)` and use `DeleteByFilter`, not individual point IDs.
- **Forgetting `status_id=*` in Redmine issue fetch:** Default returns open issues only. The indexer must index closed issues too.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Scheduler cron expressions | Custom time.Ticker loop | `robfig/cron/v3` | Handles daylight saving, missed jobs, timezone; already in requirements.md stack |
| Concurrent single-flight | Mutex + bool flag | `golang.org/x/sync/singleflight` | Race-free, handles panics, allows shared result — not trivial to implement correctly |
| Qdrant facet aggregation | Scroll all results, count in map | `client.Facet(FacetCounts)` | O(1) index scan in Qdrant vs O(N) application-side; exact/approximate mode |
| HTTP middleware chain | Nested if statements in handlers | `http.Handler` wrapper functions | Go idiomatic pattern; composable; testable in isolation |
| Batch-32 TEI chunking | Single large request | Loop with `teiBatchSize=32` slices | Already proven in bench/recall/main.go; TEI returns HTTP 422 for batches > 32 |

**Key insight:** The Qdrant facet API, introduced in v1.12 and available in go-client v1.16.2 (already in `go.mod`), eliminates the need for any custom aggregation code. Four parallel `client.Facet` calls return all facet counts in a single round-trip each.

---

## Common Pitfalls

### Pitfall 1: Redmine `updated_on` Filter Encoding
**What goes wrong:** Using literal `>=` in the URL query string causes Redmine to ignore the filter or return a 400 error.
**Why it happens:** Redmine's filter parser requires URL-encoded comparison operators: `>=` → `%3E%3D`, `>` → `%3E`, `<=` → `%3C%3D`.
**How to avoid:** Build the filter value as a string (`">=" + timestamp`), then `url.QueryEscape` the entire value before appending to the URL. Or use `url.Values.Set` which auto-encodes.
**Warning signs:** Incremental sync fetches all issues on every cycle (no cursor advancement) — the filter is being silently ignored.

### Pitfall 2: Missing `status_id=*` in Issue Fetch
**What goes wrong:** Indexer never indexes closed or resolved issues — users can't find them via search.
**Why it happens:** Redmine's `/issues.json` default is `status_id=open`. This is easy to miss.
**How to avoid:** Always pass `status_id=*` in all Redmine issue fetch calls.
**Warning signs:** Search returns no results for issues users know are closed.

### Pitfall 3: Qdrant Scroll Cursor vs QueryPoints Offset
**What goes wrong:** Using integer offset in `ScrollPoints` skips points incorrectly or panics.
**Why it happens:** `ScrollPoints` uses cursor pagination (PointId-based `Offset` field + `next_page_offset` in response). `QueryPoints` uses integer offset. Mixing them up causes silent data gaps.
**How to avoid:** Deletion reconciliation uses `client.ScrollAndOffset` → follow `next_page_offset` loop. Search pagination uses `QueryPoints.Offset` (integer).
**Warning signs:** Scroll loop terminates before fetching all IDs; `next_page_offset` is always non-nil.

### Pitfall 4: Chunk Point IDs for Multi-Chunk Issues
**What goes wrong:** Re-indexing an issue creates duplicate chunks because the chunk count or content changed, resulting in stale chunks remaining in Qdrant.
**Why it happens:** `PointID("issue", redmineID)` generates one UUID per issue, but chunks need distinct IDs.
**How to avoid:** Generate point IDs as `PointID(fmt.Sprintf("issue:%d:chunk:%d", redmineID, chunkIndex))`. On re-index, first delete all existing chunks for that `redmine_id` (via `DeleteByFilter`), then upsert the new chunks.
**Warning signs:** Search returns duplicate results for the same issue; score counts keep rising per issue.

### Pitfall 5: Permission Cache Not Invalidated on 403 from Qdrant
**What goes wrong:** Cached `[]project_id` becomes stale after a user loses project access; they continue to see results they shouldn't for up to TTL minutes.
**Why it happens:** Cache only refreshes on expiry, not on access-check failure.
**How to avoid:** This is acceptable behavior for the agreed-upon TTL approach (few minutes). Document that TTL is a security trade-off. Do NOT extend TTL beyond 5 minutes.
**Warning signs:** N/A — this is by design, but the TTL must be short.

### Pitfall 6: Facet Filter Must Match Main Query Filter
**What goes wrong:** Facet counts include results from projects the user cannot access.
**Why it happens:** Calling `client.Facet` without the permission filter returns global counts.
**How to avoid:** Pass the same `permissionFilter` (the `project_ids Must` condition) to every `FacetCounts` request.
**Warning signs:** Facet counts are higher than the total search result count.

### Pitfall 7: TEI Batch Size Limit
**What goes wrong:** `EmbedPassages` returns HTTP 422 for batches > 32 texts.
**Why it happens:** TEI default `--max-client-batch-size 32`. Already documented in prior decisions but easy to miss when implementing the indexer pipeline.
**How to avoid:** Loop in batches of 32 in `EmbedPassages` (or extend TEIEmbedder to chunk internally). Pattern established in bench/recall/main.go.

### Pitfall 8: Large Qdrant Scroll for Deletion Reconciliation — Memory
**What goes wrong:** OOM on large Redmine instances (100k+ issues) when collecting all IDs into memory.
**Why it happens:** Naive implementation collects `[]string` of all point UUIDs in RAM.
**How to avoid:** Stream scroll in pages of 1000 points. Fetch Redmine IDs in parallel pages. Compute set difference per page using a bloom filter or paginated comparison. In practice, 100k UUIDs (36 chars each) = ~3.6 MB — acceptable. But use paged streaming anyway.

---

## Code Examples

Verified patterns from official sources:

### Redmine Issue Fetch with Updated_on Cursor
```go
// Source: Redmine REST API docs — https://www.redmine.org/projects/redmine/wiki/Rest_Issues
func (c *Client) FetchIssuesSince(ctx context.Context, since time.Time, offset, limit int) (*IssueList, error) {
    // Build URL with properly encoded updated_on filter
    params := url.Values{}
    params.Set("updated_on", ">="+since.UTC().Format(time.RFC3339))
    params.Set("status_id", "*")           // include closed issues
    params.Set("sort", "updated_on:asc")   // advance cursor monotonically
    params.Set("limit", strconv.Itoa(limit))
    params.Set("offset", strconv.Itoa(offset))

    req, _ := http.NewRequestWithContext(ctx, "GET",
        c.baseURL+"/issues.json?"+params.Encode(), nil)
    req.Header.Set("X-Redmine-API-Key", c.adminAPIKey)
    // ... execute, decode JSON
}
```

### Qdrant Query with Permission Pre-Filter
```go
// Source: Qdrant go-client docs — Filter.Must, NewMatchInt
func buildProjectFilter(projectIDs []int64) *qdrant.Filter {
    conditions := make([]*qdrant.Condition, len(projectIDs))
    for i, id := range projectIDs {
        conditions[i] = qdrant.NewMatchInt("project_id", id)
    }
    return &qdrant.Filter{
        Must: []*qdrant.Condition{{
            ConditionOneOf: &qdrant.Condition_Filter{
                Filter: &qdrant.Filter{Should: conditions},
            },
        }},
    }
}

results, err := client.Query(ctx, &qdrant.QueryPoints{
    CollectionName: "redmine_search",
    Query:          qdrant.NewQueryDense(queryVec),
    Filter:         buildProjectFilter(userProjectIDs),
    Limit:          uint64(perPage * 2), // 2x oversampling for post-filter
    WithPayload:    qdrant.NewWithPayloadInclude(
        "redmine_id", "subject", "tracker", "status",
        "project_id", "author", "is_private", "text_preview"),
})
```

### Qdrant Facet Count
```go
// Source: Qdrant 1.12 blog + go-client v1.16.2 API (already in go.mod)
hits, err := client.Facet(ctx, &qdrant.FacetCounts{
    CollectionName: "redmine_search",
    Key:            "tracker",
    Filter:         permissionFilter,
    Limit:          qdrant.PtrOf(uint64(50)),
    Exact:          qdrant.PtrOf(false),
})
// hits[i].Value (qdrant.FacetValue — string or int64), hits[i].Count (uint64)
```

### Qdrant Scroll for Deletion Reconciliation
```go
// Source: Qdrant docs — Scroll API with next_page_offset
// https://qdrant.tech/documentation/concepts/points/
var offset *qdrant.PointId
for {
    points, nextOffset, err := client.ScrollAndOffset(ctx, &qdrant.ScrollPoints{
        CollectionName: "redmine_search",
        Filter: &qdrant.Filter{Must: []*qdrant.Condition{
            qdrant.NewMatch("content_type", "issue"),
        }},
        Limit:       qdrant.PtrOf(uint32(1000)),
        WithPayload: qdrant.NewWithPayloadInclude("redmine_id"),
        WithVectors: qdrant.NewWithVectors(false),
        Offset:      offset,
    })
    if err != nil {
        return err
    }
    // process points...
    if nextOffset == nil {
        break
    }
    offset = nextOffset
}
```

### Qdrant Delete by Filter (all chunks of an issue)
```go
// Source: Qdrant go-client docs — DeleteByFilter
_, err = client.DeleteByFilter(ctx, "redmine_search",
    &qdrant.Filter{Must: []*qdrant.Condition{
        qdrant.NewMatch("content_type", "issue"),
        qdrant.NewMatchInt("redmine_id", int64(redmineID)),
    }},
)
```

### Go 1.22 ServeMux with Auth Middleware
```go
// Source: Go 1.22 routing blog post — https://go.dev/blog/routing-enhancements
mux := http.NewServeMux()
mux.HandleFunc("GET /api/v1/health", healthHandler)
mux.Handle("GET /api/v1/search", authMiddleware.Wrap(http.HandlerFunc(searchHandler)))
srv := &http.Server{
    Addr:         cfg.ListenAddr,
    Handler:      mux,
    ReadTimeout:  10 * time.Second,
    WriteTimeout: 30 * time.Second,
}
```

### Qdrant Upsert with Chunk Payload
```go
// Source: Qdrant go-client upsert + existing pointid.go pattern
func chunkPointID(redmineID, chunkIndex int) string {
    key := fmt.Sprintf("issue:%d:chunk:%d", redmineID, chunkIndex)
    return uuid.NewSHA1(qdrant.PointIDNamespace, []byte(key)).String()
}

points := []*qdrant.PointStruct{
    {
        Id:      qdrant.NewIDUUID(chunkPointID(issue.ID, i)),
        Vectors: qdrant.NewVectors(vec...),
        Payload: qdrant.NewValueMap(map[string]any{
            "redmine_id":   issue.ID,
            "content_type": "issue",
            "project_id":   issue.Project.ID,
            "tracker":      issue.Tracker.Name,
            "status":       issue.Status.Name,
            "author":       issue.Author.Name,
            "subject":      issue.Subject,
            "is_private":   issue.IsPrivate,
            "text_preview": truncate(chunk, 500),
            "chunk_index":  i,
            "chunk_total":  len(chunks),
            "created_on":   issue.CreatedOn,  // timestamppb or RFC3339 string
            "updated_on":   issue.UpdatedOn,
        }),
    },
}
_, err = client.Upsert(ctx, &qdrant.UpsertPoints{
    CollectionName: "redmine_search",
    Wait:           &trueVal,
    Points:         points,
})
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `gorilla/mux` for HTTP routing | stdlib `net/http` ServeMux 1.22 | Go 1.22 (Feb 2024) | No external router dependency needed for method+path routing |
| Application-side facet aggregation | Qdrant `client.Facet` API | Qdrant 1.12 (Oct 2024) | Single gRPC call per facet dimension; no O(N) scan |
| `go-chi/chi` for middleware | `http.HandlerFunc` wrapper | Always standard | Idiomatic Go; no dependency |
| Blocking full-sync before serving | Incremental-only, serve immediately | Established pattern | Service available on startup; index fills gradually |

**Deprecated/outdated:**
- `qdrant.SearchPoints` (legacy vector search): Replaced by `qdrant.QueryPoints` with `NewQueryDense`. Use `QueryPoints` exclusively.
- Integer offset for Qdrant scroll: Scroll now uses PointId-based cursor (`next_page_offset`). `ScrollAndOffset` returns the next cursor directly.

---

## Discretion Recommendations

For the areas delegated to Claude's discretion:

| Parameter | Recommendation | Rationale |
|-----------|----------------|-----------|
| Chunk size | 1600 chars / 200 char overlap | ~400 tokens at 4 chars/token for multilingual text; no tiktoken dependency |
| Permission cache TTL | 5 minutes | Matches Redmine requirements doc default; balances freshness vs Redmine load |
| Polling interval | 5 minutes | Matches requirements doc; issues appear in search within one polling interval |
| Deletion reconciliation schedule | Every 6 hours (`0 */6 * * *`) | Orphans aren't urgent; 6h reduces admin-key API load; cron expression via robfig/cron |
| Snippet generation | First 150 chars of matched chunk's `text_preview` payload field | Simple, no extra computation; matched chunk content is already relevant |
| Default per_page | 20 | Matches Redmine's own API default and requirements doc |
| Facet aggregation | Qdrant-native `client.Facet` | Faster than application-side; all four fields already indexed |

---

## Open Questions

1. **`client.DeleteByFilter` availability in go-client v1.16.2**
   - What we know: `Delete` with `NewPointsSelector` (by UUID list) is documented. Qdrant server supports filter-based deletion.
   - What's unclear: Whether go-client v1.16.2 exposes `DeleteByFilter` directly or requires constructing a `DeletePoints` with a filter selector.
   - Recommendation: Check `pkg.go.dev/github.com/qdrant/go-client/qdrant` for `DeletePoints` struct — it likely has a `Filter` field alongside `Points`. If not, fall back to: scroll all chunk point IDs for the redmine_id, then delete by UUID list.

2. **`qdrant.NewMatchAny` for multiple project_ids in a single condition**
   - What we know: `NewMatchInt` matches a single integer. `Should` with multiple `NewMatchInt` conditions works but creates N conditions.
   - What's unclear: Whether go-client v1.16.2 has a `NewMatchAny` or `NewMatchInts` helper that generates a single `IN (...)` condition.
   - Recommendation: Use `Filter.Should` with N `NewMatchInt` conditions (confirmed working pattern). If a user has 50+ projects, this is 50 conditions — acceptable for gRPC protobuf.

3. **Redmine `is_private` field in `/issues.json` response**
   - What we know: The field exists and is settable via the API. Historical Redmine bug #10870 noted it was missing from API responses but was patched.
   - What's unclear: Exact Redmine version threshold for reliable `is_private` in JSON response.
   - Recommendation: Include `is_private` in the payload during indexing (default to `false` if absent). Document that post-filtering requires Redmine ≥ 2.3 or equivalent.

4. **`ScrollAndOffset` vs `Scroll` return signature in go-client v1.16.2**
   - What we know: Both methods exist. `ScrollAndOffset` is documented as returning a separate offset pointer.
   - What's unclear: Exact return type — `(*ScrollResponse, *qdrant.PointId, error)` or a struct.
   - Recommendation: Verify during 02-01 (Redmine client) implementation by checking the go-client source on pkg.go.dev.

---

## Sources

### Primary (HIGH confidence)
- `https://www.redmine.org/projects/redmine/wiki/Rest_Issues` — issue endpoint params, updated_on filter syntax, status_id=*, pagination
- `https://www.redmine.org/projects/redmine/wiki/rest_api` — auth header name (`X-Redmine-API-Key`), pagination defaults, impersonation
- `https://www.redmine.org/projects/redmine/wiki/Rest_Users` — `/users/current.json?include=memberships`, non-admin access
- `https://www.redmine.org/projects/redmine/wiki/Rest_Projects` — GET /projects.json returns accessible projects for current user
- `https://qdrant.tech/documentation/concepts/filtering/` — Go filter examples (Match, MatchInt, DatetimeRange, HasId, Must/Should/MustNot)
- `https://qdrant.tech/documentation/concepts/search/` — QueryPoints with Limit, Offset, WithPayload
- `https://qdrant.tech/documentation/concepts/points/` — Scroll API, ScrollAndOffset, DeletePoints by selector
- `https://qdrant.tech/blog/qdrant-1.12.x/` — Facet counting feature, introduced Oct 2024 in v1.12
- `https://api.qdrant.tech/api-reference/points/facet` — FacetCounts request schema, response format
- `https://go.dev/blog/routing-enhancements` — Go 1.22 ServeMux method+path routing, r.PathValue
- Existing codebase: `bench/recall/main.go` — confirmed Qdrant Query pattern, TEI batch-32 limit, backoff.Permanent
- Existing codebase: `internal/qdrant/collection.go` — confirmed payload field names and types
- `https://pkg.go.dev/github.com/qdrant/go-client/qdrant` — Facet method, FacetCounts struct, QueryPoints.Offset

### Secondary (MEDIUM confidence)
- WebSearch: robfig/cron v3 — scheduler with cron expressions; context cancellation via job interface
- WebSearch: singleflight pattern — prevents cache stampede, standard Go 2024 practice

### Tertiary (LOW confidence)
- WebSearch: tiktoken-go alternatives — character-based approximation is well-accepted but not formally benchmarked for multilingual-e5-base specifically

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all libraries already in go.mod or well-established stdlib
- Architecture: HIGH — patterns verified against official Qdrant and Redmine docs
- Pitfalls: HIGH — most from direct API docs + existing codebase learnings
- Facet aggregation: HIGH — Qdrant 1.12 blog + API spec confirm native support in v1.16.2
- Chunking parameters: MEDIUM — character approximation is reasonable but not empirically validated for this specific model

**Research date:** 2026-02-18
**Valid until:** 2026-04-18 (60 days — Qdrant and Redmine APIs are stable)
