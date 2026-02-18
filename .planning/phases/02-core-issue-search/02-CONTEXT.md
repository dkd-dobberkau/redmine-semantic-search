# Phase 2: Core Issue Search - Context

**Gathered:** 2026-02-18
**Status:** Ready for planning

<domain>
## Phase Boundary

Users can submit a natural-language query and receive permission-filtered, relevance-ranked Redmine issues. The index stays fresh through incremental sync with deletion reconciliation. Wiki pages, journal entries, and full reindex are separate phases.

</domain>

<decisions>
## Implementation Decisions

### Issue text for indexing
- Embed subject + description only — no custom fields, no journals (journals are Phase 3)
- Metadata (tracker, status, priority, assignee, project) stored as Qdrant payload for filtering, NOT embedded in text
- Long issues split into overlapping chunks (~400 tokens with overlap); each chunk becomes its own vector linked to the parent issue via payload
- Minimal text preprocessing: strip Textile/Markdown formatting, normalize whitespace, keep text as-is — the multilingual model handles mixed DE/EN natively

### Search result shape
- Minimal fields per hit: issue ID, subject, relevance score
- Include a ~150-character text snippet per result showing the matched chunk content
- Facet counts included in response: aggregated counts per tracker, status, project, and author alongside results
- Offset-based pagination: `page` + `per_page` query params, matching Redmine's own API style

### Auth & permission flow
- Pass-through Redmine API key — callers send their Redmine key, RSS validates it against Redmine and resolves permissions
- Header: `X-Redmine-API-Key` (matches Redmine's own convention)
- Permission lookups cached with short TTL (few minutes) to reduce Redmine API calls on repeated searches
- Error responses: 401 for invalid/missing key, 503 when Redmine is unreachable — clear distinction for clients

### Sync & freshness
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

</decisions>

<specifics>
## Specific Ideas

- X-Redmine-API-Key header reuses Redmine's own convention — clients already know it
- Pagination matches Redmine's own API style (page + per_page) for consistency
- Bounded page approach means the service is queryable immediately on first start, even with a partially filled index

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 02-core-issue-search*
*Context gathered: 2026-02-18*
