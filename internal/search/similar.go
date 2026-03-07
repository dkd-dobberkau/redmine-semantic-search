package search

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/qdrant/go-client/qdrant"

	"github.com/oliverpool/redmine-semantic-search/internal/auth"
	qdrantpkg "github.com/oliverpool/redmine-semantic-search/internal/qdrant"
)

// SimilarHandler serves GET /api/v1/similar?issue_id=123 and returns issues
// most similar to the given issue using Qdrant's recommend API.
type SimilarHandler struct {
	qdrant *qdrant.Client
	logger *slog.Logger
}

func NewSimilarHandler(qdrantClient *qdrant.Client, logger *slog.Logger) *SimilarHandler {
	return &SimilarHandler{qdrant: qdrantClient, logger: logger}
}

func (h *SimilarHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	issueID := parseIntParam(r, "issue_id", 0)
	if issueID == 0 {
		writeJSONError(w, http.StatusBadRequest, "missing required parameter: issue_id")
		return
	}

	limit := parseIntParam(r, "limit", 10)
	if limit < 1 {
		limit = 1
	}
	if limit > maxPerPage {
		limit = maxPerPage
	}

	user := auth.UserFromContext(ctx)
	if user == nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	permFilter := buildPermissionFilter(user.ProjectIDs, "", "", "", "", nil, nil)

	// Use chunk 0 of the issue as the positive example for recommendation.
	pointID := qdrantpkg.ChunkPointID(issueID, 0)
	fetchLimit := uint64(limit * oversampleFactor)

	scoredPoints, err := h.qdrant.Query(ctx, &qdrant.QueryPoints{
		CollectionName: qdrantpkg.AliasName,
		Query: qdrant.NewQueryRecommend(&qdrant.RecommendInput{
			Positive: []*qdrant.VectorInput{
				qdrant.NewVectorInputID(qdrant.NewIDUUID(pointID)),
			},
		}),
		Filter: permFilter,
		Limit:  &fetchLimit,
		WithPayload: qdrant.NewWithPayloadInclude(
			"redmine_id", "subject", "tracker", "status",
			"project_id", "author", "author_id", "is_private",
			"text_preview", "chunk_index", "chunk_total",
		),
	})
	if err != nil {
		if strings.Contains(err.Error(), "Not found") {
			writeJSONError(w, http.StatusNotFound, "issue not found in index")
			return
		}
		h.logger.ErrorContext(ctx, "similar: Qdrant query failed", slog.String("error", err.Error()))
		writeJSONError(w, http.StatusInternalServerError, "search service error")
		return
	}

	// Post-filter private issues.
	filtered := make([]*qdrant.ScoredPoint, 0, len(scoredPoints))
	for _, pt := range scoredPoints {
		if extractPayloadBool(pt, "is_private") {
			if !user.IsAdmin && extractPayloadInt(pt, "author_id") != user.UserID {
				continue
			}
		}
		filtered = append(filtered, pt)
	}

	// Deduplicate chunks and exclude the source issue itself.
	type issueEntry struct {
		point *qdrant.ScoredPoint
		score float32
	}
	seen := make(map[int]issueEntry)
	for _, pt := range filtered {
		rid := extractPayloadInt(pt, "redmine_id")
		if rid == issueID {
			continue // skip the source issue
		}
		if existing, ok := seen[rid]; !ok || pt.Score > existing.score {
			seen[rid] = issueEntry{point: pt, score: pt.Score}
		}
	}

	deduped := make([]*qdrant.ScoredPoint, 0, len(seen))
	for _, entry := range seen {
		deduped = append(deduped, entry.point)
	}
	sortByScore(deduped)

	if len(deduped) > limit {
		deduped = deduped[:limit]
	}

	results := make([]SearchResult, 0, len(deduped))
	for _, pt := range deduped {
		results = append(results, SearchResult{
			IssueID:   extractPayloadInt(pt, "redmine_id"),
			Subject:   extractPayloadString(pt, "subject"),
			Score:     pt.Score,
			Snippet:   truncateSnippet(extractPayloadString(pt, "text_preview"), snippetMaxLen),
			Tracker:   extractPayloadString(pt, "tracker"),
			Status:    extractPayloadString(pt, "status"),
			ProjectID: extractPayloadInt(pt, "project_id"),
			Author:    extractPayloadString(pt, "author"),
		})
	}

	encodeFinalResponse(w, SearchResponse{
		Results: results,
		Total:   len(results),
		Page:    1,
		PerPage: limit,
	})
}
