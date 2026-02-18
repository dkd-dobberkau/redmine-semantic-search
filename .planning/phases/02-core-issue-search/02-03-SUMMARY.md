---
phase: 02-core-issue-search
plan: 03
subsystem: indexer
tags: [go, qdrant, redmine, cron, sync, reconcile, incremental-indexing]

# Dependency graph
requires:
  - phase: 02-02
    provides: Pipeline.IndexIssues, DeleteIssueChunks, ChunkPointID
  - phase: 02-01
    provides: redmine.Client.FetchIssuesSince, FetchAllIssueIDs

provides:
  - Syncer with Start/Stop polling Redmine incrementally every N minutes with cursor advancement
  - Reconciler scrolling Qdrant for orphan points and deleting them on a cron schedule
  - cmd/indexer/main.go fully wired entrypoint with graceful shutdown

affects: [02-05-search-server, 03-docker-deployment, 05-ops]

# Tech tracking
tech-stack:
  added: [github.com/robfig/cron/v3 v3.0.1]
  patterns:
    - Bounded-page incremental sync — one page per cycle, cursor advances only on success
    - Cursor-based Qdrant scroll for full-collection ID scan (no integer offset)
    - Cron-scheduled reconciliation using robfig/cron/v3
    - Context-cancel + done channel for clean goroutine lifecycle (Syncer)
    - Graceful shutdown via signal.NotifyContext waiting on both syncer and reconciler

key-files:
  created:
    - internal/indexer/sync.go
    - internal/indexer/reconcile.go
  modified:
    - cmd/indexer/main.go
    - go.mod
    - go.sum

key-decisions:
  - "Cursor initialized to zero (epoch) — first sync fetches all issues from the beginning without a blocking pre-run"
  - "One page per sync cycle (bounded by SyncBatchSize) — more pages log a hint and are picked up next cycle"
  - "Cursor advances only after both fetch and index succeed — partial failures retry the same batch"
  - "Reconciler uses ScrollAndOffset (cursor-based) not integer-offset pagination — aligns with Qdrant pitfall documentation"
  - "Orphan deletion is batched at 500 points per Delete call — bounds single-call payload size"
  - "reconciler.Stop() returns context.Context, awaited via <-ctx.Done() in main shutdown sequence"

patterns-established:
  - "Syncer lifecycle: Start(ctx) creates derived context, goroutine closes done on exit, Stop() cancels + waits"
  - "Context-check after errors: if ctx.Err() != nil return silently (shutdown path)"
  - "JSON slog logger threaded from main to all subsystems"

requirements-completed: [IDX-04, IDX-06]

# Metrics
duration: 2min
completed: 2026-02-18
---

# Phase 2 Plan 03: Incremental Sync Scheduler and Deletion Reconciliation Summary

**Cron-driven Qdrant reconciliation (ScrollAndOffset cursor-based) + bounded-page incremental Redmine sync with monotonic cursor advancement, wired into a fully functional indexer entrypoint with graceful shutdown**

## Performance

- **Duration:** 2 min
- **Started:** 2026-02-18T17:00:57Z
- **Completed:** 2026-02-18T17:02:57Z
- **Tasks:** 2
- **Files modified:** 5 (sync.go created, reconcile.go created, main.go rewritten, go.mod+go.sum updated)

## Accomplishments

- Syncer polls Redmine every N minutes, fetches one bounded page of issues, indexes via pipeline, and advances cursor only on success
- Reconciler scrolls all Qdrant issue points with cursor-based pagination, diffs against Redmine IDs, and batch-deletes orphans
- cmd/indexer/main.go wires all components with signal.NotifyContext for clean SIGINT/SIGTERM shutdown

## Task Commits

Each task was committed atomically:

1. **Task 1: Incremental sync scheduler with bounded page fetch and cursor advancement** - `8f96d2a` (feat)
2. **Task 2: Deletion reconciliation job and indexer main.go wiring** - `8c18e17` (feat)

**Plan metadata:** (docs commit follows)

## Files Created/Modified

- `internal/indexer/sync.go` - Syncer struct: Start/Stop goroutine lifecycle, poll() with cursor advancement, maxUpdatedOn helper
- `internal/indexer/reconcile.go` - Reconciler struct: cron-scheduled full ID diff, ScrollAndOffset pagination, batch orphan deletion
- `cmd/indexer/main.go` - Full indexer entrypoint: config load, Qdrant connect, EnsureCollection, pipeline, syncer, reconciler, signal handling
- `go.mod` - Added github.com/robfig/cron/v3 v3.0.1
- `go.sum` - Updated checksums

## Decisions Made

- Cursor at zero (epoch) on first run — fetches all issues immediately without a blocking pre-scan
- One page per cycle (SyncBatchSize, default 100) — subsequent pages logged as hint, fetched next cycle
- Cursor advances only after both fetch and index succeed — guaranteed at-least-once processing
- ScrollAndOffset used for Qdrant pagination (cursor-based, not integer offset) — avoids skip-scanning problem on large collections
- Orphan batches of 500 — bounds single Delete call payload; partial failure continues remaining batches
- reconciler.Stop() returns context.Context (from robfig/cron), awaited via `<-ctx.Done()` in main

## Deviations from Plan

None — plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- Indexer is a fully functional background process — ready for Docker deployment in Phase 3
- Plan 02-04 (auth middleware, permission cache) was already completed out of order
- Plan 02-05 (search server HTTP endpoint) is the remaining plan in Phase 2

---
*Phase: 02-core-issue-search*
*Completed: 2026-02-18*
