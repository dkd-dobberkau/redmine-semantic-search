---
phase: 02-core-issue-search
plan: 01
subsystem: redmine-client-text-preprocessing
tags: [redmine, rest-api, http-client, text-processing, chunking, pagination]
dependency_graph:
  requires: []
  provides:
    - internal/redmine: HTTP client for Redmine REST API (issue fetch, user validation, project listing)
    - internal/text: Textile/Markdown stripping and overlapping character chunking
  affects:
    - internal/indexer: depends on FetchIssuesSince + StripMarkup + ChunkText
    - internal/auth: depends on GetCurrentUser + ListProjects
tech_stack:
  added:
    - net/http with 10s timeout for Redmine REST API
    - regexp for Textile/Markdown stripping (package-level compiled)
  patterns:
    - Sentinel errors (ErrUnauthorized, ErrNotFound) for typed HTTP error handling
    - Admin key vs user key dual-path in doJSON (single method, caller chooses key)
    - url.Values.Set for automatic URL-encoding of ">=" cursor operator
    - Rune-based chunking for correct Unicode handling with DE/EN mixed content
key_files:
  created:
    - internal/redmine/models.go
    - internal/redmine/client.go
    - internal/redmine/issues.go
    - internal/text/strip.go
    - internal/text/chunk.go
  modified: []
decisions:
  - Dual apiKey parameter in doJSON allows admin and user keys to share one implementation without wrapper structs
  - ChunkSize=1600 / ChunkOverlap=200 chars per research discretion (~400/~50 tokens for multilingual-e5-base)
  - url.Values.Set encodes ">=" automatically — no manual percent-encoding needed for updated_on cursor
  - status_id=* always passed to include closed issues (Redmine default open-only silently misses closed)
metrics:
  duration: 3 min
  completed: 2026-02-18
  tasks_completed: 2
  files_created: 5
  files_modified: 0
---

# Phase 2 Plan 01: Redmine REST Client and Text Preprocessing Summary

**One-liner:** Authenticated Redmine REST client with updated_on cursor pagination, sentinel errors, user/project resolution, Textile/Markdown stripping via compiled regexps, and 1600-char overlapping rune-based chunking.

## What Was Built

### internal/redmine/ — Redmine REST API Client

**models.go** — Go structs matching Redmine JSON API shapes:
- `IDRef` — common reference type (project, tracker, status, author, etc.)
- `Issue` — full issue shape with all standard fields including `assigned_to` as `*IDRef`
- `IssueList` — paginated envelope (`issues`, `total_count`, `offset`, `limit`)
- `User`, `Membership`, `UserResponse` — user identity and project memberships for `/users/current.json`
- `Project`, `ProjectList` — project listing with pagination

**client.go** — HTTP client implementation:
- `Client` struct with `baseURL`, `apiKey` (admin), and 10s timeout `http.Client`
- `NewClient(baseURL, apiKey)` trims trailing slash
- `doJSON(ctx, apiKey, path, params, target)` — core authenticated GET; apiKey parameter allows caller to pass either admin or user key
- `doJSONWithAdminKey` — convenience wrapper using stored admin key
- `ErrUnauthorized` (401/403) and `ErrNotFound` (404) sentinel errors
- `GetCurrentUser(ctx, apiKey)` — validates user API key, returns identity + memberships
- `ListProjects(ctx, apiKey)` — paginates through all accessible projects for a user key

**issues.go** — Issue fetching:
- `FetchIssuesSince(ctx, since, offset, limit)` — cursor-based incremental fetch with `updated_on>=RFC3339`, `status_id=*`, `sort=updated_on:asc`
- `FetchAllIssueIDs(ctx)` — paginates all issue IDs for deletion reconciliation

### internal/text/ — Text Preprocessing

**strip.go** — `StripMarkup(text string) string`:
- Package-level compiled regexps for all Textile and Markdown patterns
- Processing order: pre blocks → HTML tags → Textile headers → inline formatting (bold/italic/strike/code) → links (Textile and Markdown) → image alt extraction → whitespace normalization → TrimSpace
- Returns empty string for empty input

**chunk.go** — `ChunkText(text string) []string`:
- Constants: `ChunkSize = 1600` chars, `ChunkOverlap = 200` chars
- Converts to `[]rune` before slicing for correct Unicode handling (DE/EN mixed content)
- Returns single-element slice if text fits in one chunk
- Sliding window advances by `ChunkSize - ChunkOverlap` per step

## Verification

All verification criteria passed:

```
go build ./internal/redmine/... ./internal/text/...  # OK
go vet ./internal/redmine/... ./internal/text/...    # OK
go build ./...                                        # OK (no breakage)
```

## Commits

| Task | Commit | Description |
|------|--------|-------------|
| Task 1: Redmine REST client | 561bda0 | feat(02-01): Redmine REST client with paginated issue fetch, user validation, and project listing |
| Task 2: Text preprocessing | 3e4a580 | feat(02-01): Text preprocessing — Textile/Markdown stripping and overlapping character chunking |

## Deviations from Plan

None — plan executed exactly as written.

## Self-Check: PASSED

- internal/redmine/models.go: FOUND
- internal/redmine/client.go: FOUND
- internal/redmine/issues.go: FOUND
- internal/text/strip.go: FOUND
- internal/text/chunk.go: FOUND
- Commit 561bda0: FOUND
- Commit 3e4a580: FOUND
