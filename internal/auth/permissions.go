// Package auth provides HTTP authentication middleware and permission caching
// for the Redmine semantic search API. It validates Redmine API keys and
// resolves user permissions to enforce project-level access control.
package auth

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/oliverpool/redmine-semantic-search/internal/redmine"
)

// UserPermissions holds the resolved identity and access rights for an
// authenticated Redmine user. ProjectIDs uses int64 for direct compatibility
// with Qdrant filter values (NewMatchInt takes int64).
type UserPermissions struct {
	UserID     int
	Login      string
	IsAdmin    bool
	ProjectIDs []int64 // projects accessible to the user — used for pre-filtering
}

// cacheEntry holds a cached UserPermissions value and its expiry time.
type cacheEntry struct {
	perms     *UserPermissions
	expiresAt time.Time
}

// PermissionCache resolves Redmine API keys to UserPermissions, caching
// successful lookups for a configurable TTL to reduce Redmine API traffic.
// Concurrent requests for the same key are coalesced into a single Redmine
// call via singleflight. Invalid keys are never cached.
type PermissionCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry // api_key -> cached permissions
	sf      singleflight.Group
	ttl     time.Duration
	redmine *redmine.Client
	logger  *slog.Logger
}

// NewPermissionCache creates a PermissionCache that validates API keys against
// the given Redmine client and caches results for the specified TTL duration.
func NewPermissionCache(redmineClient *redmine.Client, ttl time.Duration, logger *slog.Logger) *PermissionCache {
	return &PermissionCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
		redmine: redmineClient,
		logger:  logger,
	}
}

// Resolve returns the UserPermissions for the given API key. On a cache hit
// (valid entry within TTL), the cached value is returned immediately. On a
// miss, the permissions are fetched from Redmine — concurrent requests for
// the same key are deduplicated via singleflight.
//
// Sentinel errors:
//   - redmine.ErrUnauthorized when the key is invalid or revoked (do not cache).
//   - wrapped errors for network failures or Redmine unavailability.
func (c *PermissionCache) Resolve(ctx context.Context, apiKey string) (*UserPermissions, error) {
	// Fast path: check cache under read lock.
	c.mu.RLock()
	if e, ok := c.entries[apiKey]; ok && time.Now().Before(e.expiresAt) {
		c.mu.RUnlock()
		return e.perms, nil
	}
	c.mu.RUnlock()

	// Cache miss: use singleflight to coalesce concurrent requests for the same key.
	v, err, _ := c.sf.Do(apiKey, func() (any, error) {
		return c.fetchFromRedmine(ctx, apiKey)
	})
	if err != nil {
		return nil, err
	}

	perms := v.(*UserPermissions)

	// Write the result to cache.
	c.mu.Lock()
	c.entries[apiKey] = cacheEntry{
		perms:     perms,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()

	return perms, nil
}

// fetchFromRedmine calls the Redmine API to validate the key and resolve
// permissions. It is called at most once per in-flight key via singleflight.
//
// On redmine.ErrUnauthorized the error is returned immediately without caching.
// On network or other errors a wrapped error is returned for 503 propagation.
func (c *PermissionCache) fetchFromRedmine(ctx context.Context, apiKey string) (*UserPermissions, error) {
	// Validate the key and fetch user identity.
	user, err := c.redmine.GetCurrentUser(ctx, apiKey)
	if err != nil {
		if err == redmine.ErrUnauthorized {
			// Invalid key — do not cache; propagate sentinel for 401 response.
			return nil, err
		}
		return nil, fmt.Errorf("auth: fetch current user: %w", err)
	}

	// Fetch all projects accessible to this user.
	projects, err := c.redmine.ListProjects(ctx, apiKey)
	if err != nil {
		return nil, fmt.Errorf("auth: list projects: %w", err)
	}

	projectIDs := make([]int64, len(projects))
	for i, p := range projects {
		projectIDs[i] = int64(p.ID)
	}

	return &UserPermissions{
		UserID:     user.ID,
		Login:      user.Login,
		IsAdmin:    user.Admin,
		ProjectIDs: projectIDs,
	}, nil
}

// Invalidate removes the cached permissions for the given API key, forcing
// the next request to re-validate against Redmine. Useful for cache-busting
// when a key is known to have changed.
func (c *PermissionCache) Invalidate(apiKey string) {
	c.mu.Lock()
	delete(c.entries, apiKey)
	c.mu.Unlock()
}
