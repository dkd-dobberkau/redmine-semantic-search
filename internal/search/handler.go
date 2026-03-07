package search

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/oliverpool/redmine-semantic-search/internal/auth"
	"github.com/oliverpool/redmine-semantic-search/internal/embedder"
	qdrantpkg "github.com/oliverpool/redmine-semantic-search/internal/qdrant"
)

const (
	defaultPage    = 1
	defaultPerPage = 20
	maxPerPage     = 100
	snippetMaxLen  = 150
	// oversampleFactor is the multiplier applied to the fetch limit to provide
	// headroom for post-filtering private issues after the ANN query.
	oversampleFactor = 2
)

// SearchHandler serves the GET /api/v1/search endpoint. It embeds the query,
// queries Qdrant with a permission-based pre-filter, post-filters private issues,
// deduplicates multi-chunk results, paginates, and returns JSON with facet counts.
type SearchHandler struct {
	embedder embedder.Embedder
	qdrant   *qdrant.Client
	logger   *slog.Logger
}

// NewSearchHandler creates a SearchHandler backed by the given embedder and Qdrant client.
func NewSearchHandler(emb embedder.Embedder, qdrantClient *qdrant.Client, logger *slog.Logger) *SearchHandler {
	return &SearchHandler{
		embedder: emb,
		qdrant:   qdrantClient,
		logger:   logger,
	}
}

// SearchResponse is the top-level JSON response returned by the search endpoint.
type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Total   int            `json:"total"`    // total deduped results before pagination
	Page    int            `json:"page"`
	PerPage int            `json:"per_page"`
	Facets  *FacetResponse `json:"facets"`
}

// SearchResult represents a single search hit in the API response.
type SearchResult struct {
	IssueID     int     `json:"issue_id"`
	Subject     string  `json:"subject"`
	Score       float32 `json:"score"`
	Snippet     string  `json:"snippet"`
	Tracker     string  `json:"tracker,omitempty"`
	Status      string  `json:"status,omitempty"`
	ProjectID   int     `json:"project_id"`
	Author      string  `json:"author,omitempty"`
	ContentType string  `json:"content_type"`
	JournalID   int     `json:"journal_id,omitempty"`
}

// ServeHTTP handles GET /api/v1/search.
//
// Required query parameter: q — the search query text.
// Optional parameters: page, per_page, tracker, status, project, author, date_from, date_to.
func (h *SearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSONError(w, http.StatusBadRequest, "missing required parameter: q")
		return
	}

	// Parse pagination params.
	page := parseIntParam(r, "page", defaultPage)
	if page < 1 {
		page = 1
	}
	perPage := parseIntParam(r, "per_page", defaultPerPage)
	if perPage < 1 {
		perPage = 1
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}

	// Parse optional filter params.
	tracker := r.URL.Query().Get("tracker")
	status := r.URL.Query().Get("status")
	project := r.URL.Query().Get("project")
	author := r.URL.Query().Get("author")

	// Parse optional date range params.
	var dateFrom, dateTo *time.Time
	if rawFrom := r.URL.Query().Get("date_from"); rawFrom != "" {
		t, err := time.Parse("2006-01-02", rawFrom)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid date_from: %s (expected YYYY-MM-DD)", rawFrom))
			return
		}
		dateFrom = &t
	}
	if rawTo := r.URL.Query().Get("date_to"); rawTo != "" {
		t, err := time.Parse("2006-01-02", rawTo)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid date_to: %s (expected YYYY-MM-DD)", rawTo))
			return
		}
		dateTo = &t
	}

	// Get authenticated user from context (injected by AuthMiddleware).
	user := auth.UserFromContext(ctx)
	if user == nil {
		h.logger.ErrorContext(ctx, "search: no user in context — misconfigured middleware")
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Build the combined permission + filter pre-filter.
	permFilter := buildPermissionFilter(user.ProjectIDs, tracker, status, project, author, dateFrom, dateTo)

	// Embed the query vector.
	queryVec, err := h.embedder.EmbedQuery(ctx, q)
	if err != nil {
		h.logger.ErrorContext(ctx, "search: embed query failed", slog.String("error", err.Error()))
		writeJSONError(w, http.StatusInternalServerError, "embedding service error")
		return
	}

	// Fetch more results than needed for post-filtering headroom.
	fetchLimit := uint64(page * perPage * oversampleFactor)

	scoredPoints, err := h.qdrant.Query(ctx, &qdrant.QueryPoints{
		CollectionName: qdrantpkg.AliasName,
		Query:          qdrant.NewQueryDense(queryVec),
		Filter:         permFilter,
		Limit:          &fetchLimit,
		WithPayload: qdrant.NewWithPayloadInclude(
			"redmine_id", "journal_id", "content_type", "subject", "tracker", "status",
			"project_id", "author", "author_id", "is_private",
			"text_preview", "chunk_index", "chunk_total",
		),
	})
	if err != nil {
		h.logger.ErrorContext(ctx, "search: Qdrant query failed", slog.String("error", err.Error()))
		writeJSONError(w, http.StatusInternalServerError, "search service error")
		return
	}

	// Post-filter private issues: non-admin users may only see private issues they authored.
	filtered := make([]*qdrant.ScoredPoint, 0, len(scoredPoints))
	for _, pt := range scoredPoints {
		if extractPayloadBool(pt, "is_private") {
			if !user.IsAdmin && extractPayloadInt(pt, "author_id") != user.UserID {
				continue // exclude: private issue, not admin, not the author
			}
		}
		filtered = append(filtered, pt)
	}

	// Deduplicate multi-chunk results: keep only the highest-scoring chunk per
	// (issue, content_type) pair. This means an issue hit and a journal hit for
	// the same issue are both kept — they represent different content.
	type dedupKey struct {
		issueID     int
		contentType string
		journalID   int
	}
	type dedupEntry struct {
		point *qdrant.ScoredPoint
		score float32
	}
	seen := make(map[dedupKey]dedupEntry)
	for _, pt := range filtered {
		key := dedupKey{
			issueID:     extractPayloadInt(pt, "redmine_id"),
			contentType: extractPayloadString(pt, "content_type"),
			journalID:   extractPayloadInt(pt, "journal_id"),
		}
		if existing, ok := seen[key]; !ok || pt.Score > existing.score {
			seen[key] = dedupEntry{point: pt, score: pt.Score}
		}
	}

	// Flatten deduped map to ordered slice (preserve score-based ordering).
	deduped := make([]*qdrant.ScoredPoint, 0, len(seen))
	for _, entry := range seen {
		deduped = append(deduped, entry.point)
	}
	// Sort deduped results by score descending.
	sortByScore(deduped)

	total := len(deduped)

	// Paginate.
	start := (page - 1) * perPage
	if start >= total {
		// Return empty results for out-of-range pages.
		encodeFinalResponse(w, SearchResponse{
			Results: []SearchResult{},
			Total:   total,
			Page:    page,
			PerPage: perPage,
		})
		return
	}
	end := start + perPage
	if end > total {
		end = total
	}
	pageResults := deduped[start:end]

	// Build search results.
	results := make([]SearchResult, 0, len(pageResults))
	for _, pt := range pageResults {
		contentType := extractPayloadString(pt, "content_type")
		if contentType == "" {
			contentType = "issue" // backward compat for pre-journal points
		}
		results = append(results, SearchResult{
			IssueID:     extractPayloadInt(pt, "redmine_id"),
			Subject:     extractPayloadString(pt, "subject"),
			Score:       pt.Score,
			Snippet:     truncateSnippet(extractPayloadString(pt, "text_preview"), snippetMaxLen),
			Tracker:     extractPayloadString(pt, "tracker"),
			Status:      extractPayloadString(pt, "status"),
			ProjectID:   extractPayloadInt(pt, "project_id"),
			Author:      extractPayloadString(pt, "author"),
			ContentType: contentType,
			JournalID:   extractPayloadInt(pt, "journal_id"),
		})
	}

	// Fetch facets with the same permission filter.
	facets, err := FetchFacets(ctx, h.qdrant, permFilter)
	if err != nil {
		// Facet errors are non-fatal — log and continue with nil facets.
		h.logger.WarnContext(ctx, "search: facet fetch failed", slog.String("error", err.Error()))
		facets = nil
	}

	encodeFinalResponse(w, SearchResponse{
		Results: results,
		Total:   total,
		Page:    page,
		PerPage: perPage,
		Facets:  facets,
	})
}

// buildPermissionFilter constructs a Qdrant filter that:
// 1. Restricts results to issues in the user's accessible projects (pre-filter).
// 2. Optionally restricts by tracker, status, project, author keyword matches.
// 3. Optionally restricts by date range on the updated_on field.
func buildPermissionFilter(
	projectIDs []int64,
	tracker, status, project, author string,
	dateFrom, dateTo *time.Time,
) *qdrant.Filter {
	var mustConditions []*qdrant.Condition

	// Permission pre-filter: project_id must be in the user's accessible projects.
	// NewMatchInts matches any of the given integer values — ideal for set membership.
	mustConditions = append(mustConditions, qdrant.NewMatchInts("project_id", projectIDs...))

	// Optional keyword filters.
	if tracker != "" {
		mustConditions = append(mustConditions, qdrant.NewMatch("tracker", tracker))
	}
	if status != "" {
		mustConditions = append(mustConditions, qdrant.NewMatch("status", status))
	}
	if project != "" {
		mustConditions = append(mustConditions, qdrant.NewMatch("project_id", project))
	}
	if author != "" {
		mustConditions = append(mustConditions, qdrant.NewMatch("author", author))
	}

	// Optional date range filter on updated_on (Datetime index).
	if dateFrom != nil || dateTo != nil {
		dtRange := &qdrant.DatetimeRange{}
		if dateFrom != nil {
			dtRange.Gte = timestamppb.New(*dateFrom)
		}
		if dateTo != nil {
			// Use end-of-day for date_to to include all issues updated on that day.
			endOfDay := time.Date(dateTo.Year(), dateTo.Month(), dateTo.Day(), 23, 59, 59, 0, time.UTC)
			dtRange.Lte = timestamppb.New(endOfDay)
		}
		mustConditions = append(mustConditions, qdrant.NewDatetimeRange("updated_on", dtRange))
	}

	return &qdrant.Filter{Must: mustConditions}
}

// extractPayloadString safely retrieves a string value from a Qdrant point payload.
// Returns empty string if the key is absent or the value is not a string.
func extractPayloadString(pt *qdrant.ScoredPoint, key string) string {
	if pt == nil || pt.Payload == nil {
		return ""
	}
	v, ok := pt.Payload[key]
	if !ok || v == nil {
		return ""
	}
	return v.GetStringValue()
}

// extractPayloadInt safely retrieves an integer value from a Qdrant point payload.
// Returns 0 if the key is absent or the value is not numeric.
func extractPayloadInt(pt *qdrant.ScoredPoint, key string) int {
	if pt == nil || pt.Payload == nil {
		return 0
	}
	v, ok := pt.Payload[key]
	if !ok || v == nil {
		return 0
	}
	return int(v.GetIntegerValue())
}

// extractPayloadBool safely retrieves a boolean value from a Qdrant point payload.
// Returns false if the key is absent or the value is not boolean.
func extractPayloadBool(pt *qdrant.ScoredPoint, key string) bool {
	if pt == nil || pt.Payload == nil {
		return false
	}
	v, ok := pt.Payload[key]
	if !ok || v == nil {
		return false
	}
	return v.GetBoolValue()
}

// truncateSnippet returns the first maxLen runes of s.
// If s is shorter than maxLen, it is returned unchanged.
func truncateSnippet(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}

// parseIntParam reads a query parameter by name and parses it as an integer.
// Returns defaultVal if the parameter is missing or cannot be parsed.
func parseIntParam(r *http.Request, name string, defaultVal int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return defaultVal
	}
	return v
}

// sortByScore sorts a slice of ScoredPoints in descending order of score.
// This restores ordering after deduplication via map iteration (which is random).
func sortByScore(pts []*qdrant.ScoredPoint) {
	// Simple insertion sort — result sets are small (bounded by fetch limit).
	for i := 1; i < len(pts); i++ {
		key := pts[i]
		j := i - 1
		for j >= 0 && pts[j].Score < key.Score {
			pts[j+1] = pts[j]
			j--
		}
		pts[j+1] = key
	}
}

// encodeFinalResponse sets the Content-Type header and encodes the response as JSON.
func encodeFinalResponse(w http.ResponseWriter, resp SearchResponse) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		// Header already sent — nothing we can do except log.
		slog.Error("search: failed to encode response", "error", err)
	}
}

// writeJSONError writes a JSON error response. Defined in handler.go to keep
// the error-writing logic centralized; shared with health.go via package scope.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
