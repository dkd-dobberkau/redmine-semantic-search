---
phase: 01-foundation
plan: 01
subsystem: infra
tags: [go, viper, validator, qdrant-go-client, config, module-setup]

# Dependency graph
requires: []
provides:
  - Go module github.com/oliverpool/redmine-semantic-search with all Phase 1 dependencies
  - cmd/indexer/main.go entrypoint calling config.Load()
  - internal/config/config.go with viper YAML+env loading and fail-fast validator
  - config.example.yml documenting all parameters with env var names
  - .env.example for quick-start environment variable reference
  - .gitignore with Go defaults and project-specific exclusions
affects: [01-02, 01-03, 01-04, all subsequent plans]

# Tech tracking
tech-stack:
  added:
    - github.com/qdrant/go-client v1.16.2
    - github.com/google/uuid v1.6.0
    - github.com/spf13/viper v1.21.0
    - github.com/go-playground/validator/v10 v10.30.1
    - github.com/cenkalti/backoff/v4 v4.3.0
  patterns:
    - Viper SetDefault for every field (required for AutomaticEnv+Unmarshal in viper v1.19+)
    - validator.New(validator.WithRequiredStructEnabled()) for struct validation
    - SetEnvKeyReplacer to map REDMINE_API_KEY → redmine_api_key style

key-files:
  created:
    - go.mod
    - go.sum
    - cmd/indexer/main.go
    - internal/config/config.go
    - config.example.yml
    - .env.example
    - .gitignore
  modified: []

key-decisions:
  - "Module path: github.com/oliverpool/redmine-semantic-search (no git remote, derived from plan spec)"
  - "Viper v1.21 uses go-viper/mapstructure/v2 internally — mapstructure struct tags still work identically"
  - "Config file is optional: all required fields can be satisfied via environment variables alone"
  - ".gitignore uses /indexer (anchored) not indexer (unanchored) to avoid matching the cmd/indexer/ directory"

patterns-established:
  - "Pattern: SetDefault for all config fields before AutomaticEnv — required for viper v1.19+ env override to work with Unmarshal"
  - "Pattern: formatValidationErrors reports all missing/invalid fields at once, not just the first"
  - "Pattern: API key redacted in startup log via literal [REDACTED] string"

requirements-completed: [OPS-02]

# Metrics
duration: 4min
completed: 2026-02-18
---

# Phase 1 Plan 01: Go Module Setup and Config System Summary

**Go module with viper YAML+env config loading, fail-fast validator listing all missing fields, and all Phase 1 dependencies resolved (qdrant/go-client v1.16.2, uuid, viper v1.21, validator/v10, backoff/v4)**

## Performance

- **Duration:** 4 min
- **Started:** 2026-02-18T13:30:58Z
- **Completed:** 2026-02-18T13:34:58Z
- **Tasks:** 2
- **Files modified:** 7 created

## Accomplishments

- Initialized Go module as `github.com/oliverpool/redmine-semantic-search` with all Phase 1 dependencies
- Implemented config system with viper YAML loading, env var overrides (REDMINE_URL, REDMINE_API_KEY, QDRANT_HOST, QDRANT_PORT, EMBEDDING_URL), and fail-fast validation listing all missing fields at once
- Created project directory structure following Go conventions (cmd/, internal/config, internal/embedder, internal/qdrant, bench/recall, deployments)

## Task Commits

Each task was committed atomically:

1. **Task 1: Initialize Go module with project layout and all dependencies** - `d4a5b4b` (chore)
2. **Task 2: Implement config system with viper YAML + env overrides and fail-fast validation** - `8b8cfa4` (feat)

**Plan metadata:** (docs commit follows)

## Files Created/Modified

- `go.mod` - Module definition with all Phase 1 dependencies
- `go.sum` - Dependency checksums
- `cmd/indexer/main.go` - Application entrypoint: calls config.Load(), logs config summary with API key redacted, exits
- `internal/config/config.go` - Config struct with mapstructure/validate tags; Load() with viper YAML+env and fail-fast validator
- `config.example.yml` - Documented config template with comments explaining every field and env var override names
- `.env.example` - Quick-start environment variable reference
- `.gitignore` - Go defaults plus config.yml, .env, models/, logs/, /indexer, /recall binaries

## Decisions Made

- Module path `github.com/oliverpool/redmine-semantic-search` — no git remote present, used plan specification
- Viper v1.21 (latest) pulled `go-viper/mapstructure/v2` as transitive dep — `mapstructure` struct tags continue to work identically
- Config file is optional: the validator catches missing fields whether they come from YAML or env vars
- Used `/indexer` and `/recall` (anchored patterns) in `.gitignore` to avoid accidentally ignoring the `cmd/indexer/` source directory — caught and fixed during Task 1 verification

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] .gitignore pattern `indexer` matched cmd/indexer/ source directory**
- **Found during:** Task 1 (Initialize Go module)
- **Issue:** The unanchored pattern `indexer` in .gitignore matched the `cmd/indexer/` source directory, making `cmd/indexer/main.go` invisible to git status
- **Fix:** Changed to anchored patterns `/indexer` and `/recall` which only match files at the project root
- **Files modified:** `.gitignore`
- **Verification:** `git check-ignore -v cmd/indexer/main.go` returned "NOT IGNORED"; file appeared in git status
- **Committed in:** `d4a5b4b` (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 — bug)
**Impact on plan:** Essential fix — without it, main.go would never have been committed. No scope creep.

## Issues Encountered

- Viper `AutomaticEnv()` picked up `REDMINE_URL` and `REDMINE_API_KEY` from the test environment (already set on the host machine), causing the initial validation run to show only 2 missing fields instead of 4. This is correct behavior — verified by running with a clean environment (`env -i`) which correctly showed all 4 missing fields.

## User Setup Required

None - no external service configuration required for this plan. Config system is ready to use.

## Next Phase Readiness

- Go module compiles cleanly: `go build ./cmd/indexer` and `go vet ./...` pass
- All Phase 1 dependencies are installed and `go.sum` is up to date
- Config system ready for subsequent plans to use `config.Load()` from `internal/config`
- Directory structure established for: internal/embedder (Plan 02), internal/qdrant (Plan 02), bench/recall (Plan 03), deployments (Plan 02/03)
- No blockers for 01-02 (Docker Compose + Qdrant Collection Init)

## Self-Check: PASSED

- FOUND: go.mod
- FOUND: go.sum
- FOUND: cmd/indexer/main.go
- FOUND: internal/config/config.go
- FOUND: config.example.yml
- FOUND: .env.example
- FOUND: .gitignore
- FOUND commit d4a5b4b: chore(01-01): initialize Go module with project layout and dependencies
- FOUND commit 8b8cfa4: feat(01-01): implement config system with viper YAML + env overrides and fail-fast validation

---
*Phase: 01-foundation*
*Completed: 2026-02-18*
