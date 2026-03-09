package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/oliverpool/redmine-semantic-search/internal/redmine"
)

// contextKey is an unexported type for context keys in this package,
// preventing collisions with keys defined in other packages.
type contextKey string

// ctxUserKey is the context key under which UserPermissions is stored.
const ctxUserKey contextKey = "user"

// UserFromContext extracts the authenticated UserPermissions from the request
// context. Returns nil if no user is set (should never happen behind AuthMiddleware).
func UserFromContext(ctx context.Context) *UserPermissions {
	u, _ := ctx.Value(ctxUserKey).(*UserPermissions)
	return u
}

// AuthMiddleware enforces Redmine API key authentication on wrapped HTTP
// handlers. It extracts the X-Redmine-API-Key header, validates it via the
// permission cache, and injects the resolved UserPermissions into the request
// context. Missing or invalid keys return 401; Redmine unavailability returns 503.
type AuthMiddleware struct {
	cache  *PermissionCache
	logger *slog.Logger
}

// NewAuthMiddleware creates an AuthMiddleware backed by the given permission cache.
func NewAuthMiddleware(cache *PermissionCache, logger *slog.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		cache:  cache,
		logger: logger,
	}
}

// Wrap returns an http.Handler that authenticates the request before
// delegating to next. On authentication failure, it writes a JSON error
// response and returns without calling next.
//
// Response codes:
//   - 401: missing or invalid X-Redmine-API-Key header
//   - 503: Redmine is unreachable or returned an unexpected error
func (m *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-Redmine-API-Key")
		if apiKey == "" {
			writeJSONError(w, http.StatusUnauthorized, "missing X-Redmine-API-Key header")
			return
		}

		perms, err := m.cache.Resolve(r.Context(), apiKey)
		if err != nil {
			if errors.Is(err, redmine.ErrUnauthorized) {
				writeJSONError(w, http.StatusUnauthorized, "invalid API key")
				return
			}
			// Redmine is unreachable — try stale cache entry.
			if stale := m.cache.GetStale(apiKey); stale != nil {
				m.logger.WarnContext(r.Context(), "auth: using stale cache, Redmine unavailable",
					slog.String("error", err.Error()),
				)
				perms = stale
			} else {
				// No cache at all — allow unfiltered access (public instance).
				m.logger.WarnContext(r.Context(), "auth: Redmine unavailable, allowing unfiltered access",
					slog.String("error", err.Error()),
				)
				perms = &UserPermissions{Unfiltered: true}
			}
		}

		// Inject authenticated user into request context for downstream handlers.
		ctx := context.WithValue(r.Context(), ctxUserKey, perms)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// writeJSONError writes a JSON error response with the given HTTP status code
// and message. The Content-Type header is set before WriteHeader to ensure
// proper response formatting.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
