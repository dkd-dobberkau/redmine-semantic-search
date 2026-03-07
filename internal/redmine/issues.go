package redmine

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// FetchIssuesSince returns a page of issues updated at or after the given time,
// ordered ascending by updated_on (cursor-based incremental fetch).
//
// The statusFilter parameter controls which statuses are included:
// "open" (Redmine default) indexes only open issues, "*" indexes all statuses.
func (c *Client) FetchIssuesSince(ctx context.Context, since time.Time, offset, limit int, statusFilter string) (*IssueList, error) {
	params := url.Values{}
	params.Set("updated_on", ">="+since.UTC().Format(time.RFC3339))
	params.Set("status_id", statusFilter)
	params.Set("sort", "updated_on:asc")
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("offset", fmt.Sprintf("%d", offset))

	var list IssueList
	if err := c.doJSONWithAdminKey(ctx, "/issues.json", params, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// FetchIssueWithJournals returns a single issue with its journal entries.
// Only journals with non-empty notes are included in the result.
func (c *Client) FetchIssueWithJournals(ctx context.Context, issueID int) (*IssueDetail, error) {
	params := url.Values{}
	params.Set("include", "journals")

	var resp IssueDetailResponse
	path := fmt.Sprintf("/issues/%d.json", issueID)
	if err := c.doJSONWithAdminKey(ctx, path, params, &resp); err != nil {
		return nil, err
	}

	// Filter out journals without notes (pure status-change entries).
	filtered := resp.Issue.Journals[:0]
	for _, j := range resp.Issue.Journals {
		if j.Notes != "" {
			filtered = append(filtered, j)
		}
	}
	resp.Issue.Journals = filtered

	return &resp.Issue, nil
}

// FetchAllIssueIDs returns the IDs of all issues matching the given statusFilter,
// sorted ascending by ID. Used for deletion reconciliation.
func (c *Client) FetchAllIssueIDs(ctx context.Context, statusFilter string) ([]int, error) {
	const pageLimit = 100
	var all []int
	offset := 0

	for {
		params := url.Values{}
		params.Set("status_id", statusFilter)
		params.Set("sort", "id:asc")
		params.Set("limit", fmt.Sprintf("%d", pageLimit))
		params.Set("offset", fmt.Sprintf("%d", offset))

		var page IssueList
		if err := c.doJSONWithAdminKey(ctx, "/issues.json", params, &page); err != nil {
			return nil, err
		}

		for _, issue := range page.Issues {
			all = append(all, issue.ID)
		}

		offset += len(page.Issues)
		if offset >= page.TotalCount {
			break
		}
	}

	return all, nil
}
