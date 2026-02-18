---
phase: 02-core-issue-search
plan: 04
subsystem: auth
tags: [singleflight, permission-cache, middleware, redmine, go, sync]

# Dependency graph
requires:
  - phase: 02-core-issue-search
    plan: 01
    provides: "Redmine REST client (GetCurrentUser, ListProjects, ErrUnauthorized sentinel)"

provides:
  - "PermissionCache: TTL-based cache with singleflight dedup mapping API keys to UserPermissions"
  - "UserPermissions struct: UserID, Login, IsAdmin, ProjectIDs (int64) for Qdrant pre-filter"
  - "AuthMiddleware: HTTP middleware enforcing X-Redmine-API-Key, injecting UserPermissions into context"
  - "UserFromContext accessor for downstream search handlers"

affects:
  - 02-core-issue-search
  - search-handler
  - qdrant-filter

# Tech tracking
tech-stack:
  added:
    - "golang.org/x/sync v0.19.0 (singleflight.Group for cache stampede prevention)"
  patterns:
    - "Two-layer auth: PermissionCache pre-filters by project_ids, is_admin flag available for post-filter"
    - "Sentinel error propagation: ErrUnauthorized -> 401, all other errors -> 503"
    - "singleflight.Group keyed on API key prevents duplicate Redmine calls under concurrent load"
    - "Invalid keys never cached; only successful lookups enter TTL-bounded cache"
    - "Unexported contextKey type prevents context key collisions with other packages"

key-files:
  created:
    - "internal/auth/permissions.go"
    - "internal/auth/middleware.go"
  modified:
    - "go.mod"
    - "go.sum"

key-decisions:
  - "ProjectIDs is []int64 (not []int) for direct use in Qdrant NewMatchInt filter without conversion"
  - "singleflight.Group is embedded in PermissionCache struct (not global) so multiple caches could coexist in tests"
  - "writeJSONError sets Content-Type before WriteHeader — Go http.ResponseWriter requires header mutations before WriteHeader call"
  - "errors.Is used for ErrUnauthorized check (not equality) — future-proofs against wrapped error variants"

patterns-established:
  - "Auth pattern: extract header -> cache.Resolve -> errors.Is(ErrUnauthorized) -> 401 | 503 | inject context"
  - "Cache pattern: RWMutex read lock fast path, singleflight.Do on miss, write lock to store result"

requirements-completed: [AUTH-01, AUTH-02, AUTH-03]

# Metrics
duration: 4min
completed: 2026-02-18
---

# Phase 02 Plan 04: Auth Middleware and Permission Cache Summary

**Redmine API key auth middleware with TTL permission cache, singleflight dedup, and UserPermissions context injection for Qdrant pre-filtering**

## Performance

- **Duration:** 4 min
- **Started:** 2026-02-18T16:53:17Z
- **Completed:** 2026-02-18T16:57:23Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- PermissionCache resolves API keys to UserPermissions via Redmine's GetCurrentUser + ListProjects, with configurable TTL and singleflight dedup to prevent cache stampedes
- AuthMiddleware extracts X-Redmine-API-Key, delegates to cache, returns 401 JSON (missing/invalid key) or 503 JSON (Redmine unreachable), injects UserPermissions into context on success
- UserPermissions struct carries int64 ProjectIDs (Qdrant-compatible) and IsAdmin flag ready for Phase 2 search pre-filtering and post-filtering

## Task Commits

Each task was committed atomically:

1. **Task 1: Permission cache with TTL, singleflight dedup, and Redmine-backed resolution** - `9477214` (feat)
2. **Task 2: HTTP auth middleware extracting X-Redmine-API-Key and injecting user context** - `51e40d2` (feat)

**Plan metadata:** (docs commit — see below)

## Files Created/Modified

- `internal/auth/permissions.go` - PermissionCache, UserPermissions struct, cacheEntry, Resolve and fetchFromRedmine logic, Invalidate
- `internal/auth/middleware.go` - AuthMiddleware, Wrap method, UserFromContext accessor, writeJSONError helper
- `go.mod` - Added golang.org/x/sync v0.19.0 (direct dependency via singleflight)
- `go.sum` - Updated with sync package checksums

## Decisions Made

- ProjectIDs typed as `[]int64` (not `[]int`) — Qdrant `NewMatchInt` takes int64; avoids conversion at every search request
- `errors.Is` used for `ErrUnauthorized` check — not direct equality — future-proofs against wrapped variants
- `writeJSONError` sets `Content-Type` before `WriteHeader` — Go's ResponseWriter requires headers to be set before status code
- `singleflight.Group` per cache instance (not global) — keeps concerns isolated, enables independent cache instances in tests
- `Invalidate()` method added even though not called in Phase 2 — documented as useful for future cache-busting without requiring a restart

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Auth middleware is ready to wrap the search handler (Phase 02 Plan 05 or equivalent)
- `UserFromContext(ctx)` returns `*UserPermissions` with `ProjectIDs []int64` and `IsAdmin bool` for the Qdrant query builder
- Default TTL of 5 minutes (per research doc) should be wired from `config.go` when the server is assembled — PermissionCache accepts any `time.Duration`

---
*Phase: 02-core-issue-search*
*Completed: 2026-02-18*

## Self-Check: PASSED

- FOUND: internal/auth/permissions.go
- FOUND: internal/auth/middleware.go
- FOUND: .planning/phases/02-core-issue-search/02-04-SUMMARY.md
- FOUND: commit 9477214 (Task 1 - permission cache)
- FOUND: commit 51e40d2 (Task 2 - auth middleware)
