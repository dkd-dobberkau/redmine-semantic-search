package redmine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Sentinel errors returned by Client methods.
var (
	// ErrUnauthorized is returned when Redmine responds with HTTP 401 or 403.
	ErrUnauthorized = errors.New("redmine: unauthorized")
	// ErrNotFound is returned when Redmine responds with HTTP 404.
	ErrNotFound = errors.New("redmine: not found")
)

// Client is a Redmine REST API client that authenticates requests using an
// admin API key. Individual user API keys can also be passed to specific methods.
type Client struct {
	baseURL    string
	httpClient *http.Client
	apiKey     string
}

// NewClient creates a new Redmine Client targeting the given base URL using the
// given admin API key. The trailing slash is trimmed from baseURL for consistency.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		apiKey: apiKey,
	}
}

// doJSON performs an authenticated GET request to baseURL+path with the given
// query parameters and decodes the JSON response into target.
//
// The apiKey parameter is used for the X-Redmine-API-Key header, allowing
// callers to pass either the admin key (via doJSONWithAdminKey) or a user key
// (e.g. GetCurrentUser, ListProjects).
//
// Sentinel errors:
//   - ErrUnauthorized on HTTP 401 or 403
//   - ErrNotFound on HTTP 404
//   - fmt.Errorf with status code for other non-2xx responses
func (c *Client) doJSON(ctx context.Context, apiKey string, path string, params url.Values, target any) error {
	rawURL := c.baseURL + path
	if len(params) > 0 {
		rawURL += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("redmine: build request: %w", err)
	}
	req.Header.Set("X-Redmine-API-Key", apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("redmine: http request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// success — decode below
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrNotFound
	default:
		return fmt.Errorf("redmine: unexpected status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("redmine: decode response: %w", err)
	}
	return nil
}

// doJSONWithAdminKey is a convenience wrapper around doJSON that uses the
// admin API key configured at client startup.
func (c *Client) doJSONWithAdminKey(ctx context.Context, path string, params url.Values, target any) error {
	return c.doJSON(ctx, c.apiKey, path, params, target)
}

// GetCurrentUser validates a user API key by calling GET /users/current.json
// and returns the authenticated user's identity and project memberships.
//
// The provided apiKey is the user's Redmine API key (not the admin key).
func (c *Client) GetCurrentUser(ctx context.Context, apiKey string) (*User, error) {
	params := url.Values{}
	params.Set("include", "memberships")

	var resp UserResponse
	if err := c.doJSON(ctx, apiKey, "/users/current.json", params, &resp); err != nil {
		return nil, err
	}
	return &resp.User, nil
}

// ListProjects returns all projects accessible to the given user API key by
// paginating through GET /projects.json until all pages are fetched.
//
// The provided apiKey is the user's Redmine API key (not the admin key).
func (c *Client) ListProjects(ctx context.Context, apiKey string) ([]Project, error) {
	const pageLimit = 100
	var all []Project
	offset := 0

	for {
		params := url.Values{}
		params.Set("limit", fmt.Sprintf("%d", pageLimit))
		params.Set("offset", fmt.Sprintf("%d", offset))

		var page ProjectList
		if err := c.doJSON(ctx, apiKey, "/projects.json", params, &page); err != nil {
			return nil, err
		}

		all = append(all, page.Projects...)

		offset += len(page.Projects)
		if offset >= page.TotalCount {
			break
		}
	}

	return all, nil
}
