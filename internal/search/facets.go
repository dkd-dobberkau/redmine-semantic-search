// Package search provides the HTTP handlers for the semantic search API,
// including the search endpoint, health endpoint, and facet aggregation.
package search

import (
	"context"
	"fmt"
	"sync"

	"github.com/qdrant/go-client/qdrant"

	qdrantpkg "github.com/oliverpool/redmine-semantic-search/internal/qdrant"
)

// FacetResponse holds facet counts for each search dimension.
// Facet counts respect the same permission filter as the main search query,
// ensuring that users only see counts for issues they have access to.
type FacetResponse struct {
	Trackers []FacetValue `json:"trackers"`
	Statuses []FacetValue `json:"statuses"`
	Projects []FacetValue `json:"projects"`
	Authors  []FacetValue `json:"authors"`
}

// FacetValue represents a single facet bucket with its value and document count.
type FacetValue struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// FetchFacets runs four Qdrant Facet calls concurrently (tracker, status, project_id, author)
// using the given permission filter. The same filter is applied to all calls to ensure
// counts reflect only issues accessible to the authenticated user (Pitfall 6 from research).
func FetchFacets(ctx context.Context, client *qdrant.Client, filter *qdrant.Filter) (*FacetResponse, error) {
	type facetResult struct {
		hits []*qdrant.FacetHit
		err  error
	}

	trackerCh := make(chan facetResult, 1)
	statusCh := make(chan facetResult, 1)
	projectCh := make(chan facetResult, 1)
	authorCh := make(chan facetResult, 1)

	var wg sync.WaitGroup
	wg.Add(4)

	runFacet := func(ch chan<- facetResult, key string) {
		defer wg.Done()
		hits, err := client.Facet(ctx, &qdrant.FacetCounts{
			CollectionName: qdrantpkg.AliasName,
			Key:            key,
			Filter:         filter,
			Limit:          qdrant.PtrOf(uint64(50)),
			Exact:          qdrant.PtrOf(false),
		})
		ch <- facetResult{hits: hits, err: err}
	}

	go runFacet(trackerCh, "tracker")
	go runFacet(statusCh, "status")
	go runFacet(projectCh, "project_id")
	go runFacet(authorCh, "author")

	wg.Wait()

	trackerRes := <-trackerCh
	statusRes := <-statusCh
	projectRes := <-projectCh
	authorRes := <-authorCh

	if trackerRes.err != nil {
		return nil, fmt.Errorf("facet tracker: %w", trackerRes.err)
	}
	if statusRes.err != nil {
		return nil, fmt.Errorf("facet status: %w", statusRes.err)
	}
	if projectRes.err != nil {
		return nil, fmt.Errorf("facet project_id: %w", projectRes.err)
	}
	if authorRes.err != nil {
		return nil, fmt.Errorf("facet author: %w", authorRes.err)
	}

	return &FacetResponse{
		Trackers: hitsToFacetValues(trackerRes.hits),
		Statuses: hitsToFacetValues(statusRes.hits),
		Projects: hitsToFacetValues(projectRes.hits),
		Authors:  hitsToFacetValues(authorRes.hits),
	}, nil
}

// hitsToFacetValues converts Qdrant FacetHit results to the API response format.
// Integer values (e.g. project_id) are converted to their string representation.
func hitsToFacetValues(hits []*qdrant.FacetHit) []FacetValue {
	if len(hits) == 0 {
		return []FacetValue{}
	}
	values := make([]FacetValue, 0, len(hits))
	for _, hit := range hits {
		if hit == nil || hit.Value == nil {
			continue
		}
		var strVal string
		switch {
		case hit.Value.GetStringValue() != "":
			strVal = hit.Value.GetStringValue()
		default:
			// For integer facets (project_id), format as string.
			strVal = fmt.Sprintf("%d", hit.Value.GetIntegerValue())
		}
		values = append(values, FacetValue{
			Value: strVal,
			Count: int(hit.Count),
		})
	}
	return values
}
