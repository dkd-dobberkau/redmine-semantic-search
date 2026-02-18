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
// Parameters:
//   - since: Only issues with updated_on >= since are returned.
//   - offset: Pagination offset (0-based).
//   - limit: Maximum number of issues to return per page.
//
// The status_id=* parameter ensures closed issues are included (Redmine default
// is status_id=open which would silently miss closed issues).
//
// url.Values.Set handles URL-encoding of the ">=" operator automatically, so
// the cursor value is transmitted correctly without manual encoding.
func (c *Client) FetchIssuesSince(ctx context.Context, since time.Time, offset, limit int) (*IssueList, error) {
	params := url.Values{}
	// ">=" is URL-encoded automatically by url.Values.Encode() — no manual escaping needed.
	params.Set("updated_on", ">="+since.UTC().Format(time.RFC3339))
	// status_id=* includes all statuses (open and closed). Without this, Redmine
	// defaults to open issues only and silently misses closed/rejected issues.
	params.Set("status_id", "*")
	// Sort ascending by updated_on so the cursor can advance monotonically.
	params.Set("sort", "updated_on:asc")
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("offset", fmt.Sprintf("%d", offset))

	var list IssueList
	if err := c.doJSONWithAdminKey(ctx, "/issues.json", params, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// FetchAllIssueIDs returns the IDs of all issues in Redmine, sorted ascending by ID.
// This is used for deletion reconciliation — comparing stored IDs against Redmine to
// detect issues that have been deleted since the last index run.
//
// All statuses are included (status_id=*) to ensure deleted issues can be detected
// regardless of their final status before deletion.
func (c *Client) FetchAllIssueIDs(ctx context.Context) ([]int, error) {
	const pageLimit = 100
	var all []int
	offset := 0

	for {
		params := url.Values{}
		params.Set("status_id", "*")
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
