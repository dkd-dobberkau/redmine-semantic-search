---
phase: 01-foundation
plan: 02
subsystem: infra
tags: [docker, docker-compose, dockerfile, multi-stage, qdrant, tei, go]

# Dependency graph
requires:
  - go.mod (from 01-01)
  - go.sum (from 01-01)
  - config.example.yml (from 01-01)
provides:
  - deployments/docker-compose.yml — full Docker Compose stack with Qdrant, TEI, and Go indexer
  - deployments/Dockerfile — multi-stage Go build with non-root user and health check
  - .dockerignore — build context exclusions for secrets and dev artifacts
affects: [01-03, 01-04, all deployment workflows]

# Tech tracking
tech-stack:
  added:
    - Docker Compose v2 (no version field — services: directly)
    - qdrant/qdrant:latest — vector database container
    - ghcr.io/huggingface/text-embeddings-inference:cpu-1.9 — TEI embedding service
    - golang:1.25-alpine — Go builder stage
    - alpine:3.20 — minimal runtime stage
  patterns:
    - bash /dev/tcp health check for Qdrant (no curl in Qdrant image)
    - Multi-stage Docker build with CGO_ENABLED=0 static binary
    - Non-root user (appuser, uid 1000) in runtime stage
    - Named volume for Qdrant persistence (qdrant_data)
    - HF_HUB_CACHE=/data for TEI model caching on restart

key-files:
  created:
    - deployments/docker-compose.yml
    - deployments/Dockerfile
    - .dockerignore
  modified:
    - go.mod (indirect: google/uuid and qdrant/go-client now tracked in go.sum)
    - go.sum (checksums for indirect deps added)

key-decisions:
  - "Dockerfile builder stage uses golang:1.25-alpine (not 1.23 as in research) to match go.mod go 1.25.0 directive — auto-fixed during build verification"
  - "Qdrant health check uses bash /dev/tcp since Qdrant image has no curl (GitHub issue #4250)"
  - "TEI start_period 120s accounts for model download on first container start"
  - "HF_HUB_CACHE=/data added to TEI service so model is cached in the ./models volume mount"
  - "Indexer service has no docker-compose healthcheck since health endpoint is Phase 2; Dockerfile HEALTHCHECK is present but will fail gracefully until Phase 2"

requirements-completed: [OPS-01]

# Metrics
duration: 3min
completed: 2026-02-18
---

# Phase 1 Plan 02: Docker Compose Stack and Dockerfile Summary

**Docker Compose stack with Qdrant (bash /dev/tcp health check), TEI cpu-1.9 (120s start_period for model download), and Go indexer service; multi-stage Dockerfile producing a minimal Alpine image with non-root user**

## Performance

- **Duration:** 3 min
- **Started:** 2026-02-18T13:38:45Z
- **Completed:** 2026-02-18T13:41:09Z
- **Tasks:** 2
- **Files modified:** 3 created, 2 updated

## Accomplishments

- Created complete Docker Compose stack (`deployments/docker-compose.yml`) with all three services: Qdrant vector DB, TEI embedding service, and Go indexer
- Implemented bash `/dev/tcp` health check for Qdrant (avoids curl dependency issue documented in qdrant/qdrant#4250)
- Set TEI `start_period: 120s` to account for HuggingFace model download on first start; added `HF_HUB_CACHE=/data` for caching
- Created multi-stage `deployments/Dockerfile` producing a minimal Alpine-based image with non-root user (`appuser`, uid 1000)
- Created `.dockerignore` excluding secrets, dev artifacts, and non-essential directories from the Docker build context

## Task Commits

Each task was committed atomically:

1. **Task 1: Create Docker Compose stack with all three services** - `3600bfc` (feat)
2. **Task 2: Create multi-stage Dockerfile and .dockerignore** - `34be7c6` (feat)

**Plan metadata:** (docs commit follows)

## Files Created/Modified

- `deployments/docker-compose.yml` - Docker Compose stack with Qdrant, TEI, and Go indexer services; named volume `qdrant_data`; custom network `redmine-search-net`; health checks for all services; no hardcoded secrets
- `deployments/Dockerfile` - Multi-stage build: `golang:1.25-alpine` builder with CGO_ENABLED=0 static binary; `alpine:3.20` runtime with `appuser` uid 1000; HEALTHCHECK for Phase 2 `/health` endpoint
- `.dockerignore` - Excludes `.git`, `.planning`, `.codegraph`, `.claude`, `.env`, `config.yml`, `models/`, `logs/`, `*.md`, `deployments/`, `bench/` from Docker build context
- `go.mod` / `go.sum` - Indirect dependencies (`google/uuid`, `qdrant/go-client`) now present in go.sum (were downloaded in 01-01 but not staged for the indirect entries)

## Decisions Made

- Updated `golang:1.23-alpine` to `golang:1.25-alpine` in Dockerfile — the research specified 1.23 but `go.mod` was generated with Go 1.26 locally, producing `go 1.25.0` directive that `golang:1.23-alpine` rejects with GOTOOLCHAIN=local enforcement
- Qdrant health check uses `bash -c 'exec 3<>/dev/tcp/...'` pattern from the research document — confirmed Qdrant image has no curl
- TEI `start_period: 120s` (plan spec was 120s) — research doc also shows 60s in examples; using 120s from plan spec to be conservative about model download time
- `HF_HUB_CACHE=/data` added to TEI service (not in plan but recommended in research Open Questions #1 to prevent model re-download after container restart)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Updated Go builder image from golang:1.23-alpine to golang:1.25-alpine**
- **Found during:** Task 2 verification (`docker build`)
- **Issue:** `go.mod` contains `go 1.25.0` directive (generated by local Go 1.26.0). `golang:1.23-alpine` runs Go 1.23.12 which rejects this with `GOTOOLCHAIN=local` enforcement, failing `go mod download`
- **Fix:** Changed Dockerfile FROM line to `golang:1.25-alpine` (image confirmed available on Docker Hub)
- **Files modified:** `deployments/Dockerfile`
- **Commit:** `34be7c6` (Task 2 commit)

**2. [Rule 2 - Missing functionality] Added HF_HUB_CACHE=/data to TEI service**
- **Found during:** Task 1 (creating docker-compose.yml)
- **Issue:** Without `HF_HUB_CACHE=/data`, TEI would download the model on each container restart since the volume mount alone isn't sufficient — the cache location must be explicitly set
- **Fix:** Added `environment: HF_HUB_CACHE=/data` to TEI service definition (research Open Questions #1 recommended this)
- **Files modified:** `deployments/docker-compose.yml`
- **Commit:** `3600bfc` (Task 1 commit)

---

**Total deviations:** 2 auto-fixed (Rule 1 — bug; Rule 2 — missing functionality)
**Impact on plan:** Both are correctness fixes. Without fix #1 the Docker build fails entirely. Without fix #2 the model re-downloads on every container restart.

## User Setup Required

Before running `docker compose up`, users must:
1. Copy `.env.example` to `deployments/.env` and fill in their Redmine URL and API key
2. Copy `config.example.yml` to `deployments/config.yml` and configure Qdrant/embedding settings

No additional manual steps beyond env file setup.

## Next Phase Readiness

- Docker Compose stack validates: `docker compose -f deployments/docker-compose.yml config` passes
- Docker image builds: `docker build -f deployments/Dockerfile .` succeeds from project root
- All three services have correct naming (`redmine-search-*` prefix)
- Named volume `qdrant_data` will persist across container restarts
- TEI model caching configured via `HF_HUB_CACHE=/data`
- Ready for 01-03 (Qdrant collection init + Embedder interface)

## Self-Check: PASSED

- FOUND: deployments/docker-compose.yml
- FOUND: deployments/Dockerfile
- FOUND: .dockerignore
- FOUND commit 3600bfc: feat(01-02): create Docker Compose stack with all three services
- FOUND commit 34be7c6: feat(01-02): create multi-stage Dockerfile and .dockerignore

---
*Phase: 01-foundation*
*Completed: 2026-02-18*
