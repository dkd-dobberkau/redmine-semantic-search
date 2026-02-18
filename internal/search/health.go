package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/qdrant/go-client/qdrant"
)

const embeddingHealthTimeout = 5 * time.Second

// HealthHandler serves the GET /api/v1/health endpoint. It checks both the
// Qdrant gRPC service and the TEI embedding HTTP service, and returns a JSON
// response with component-level statuses and an overall health status.
//
// This endpoint is intentionally unauthenticated so that monitoring systems
// and Docker health checks can poll it without credentials.
type HealthHandler struct {
	qdrant       *qdrant.Client
	embeddingURL string
	logger       *slog.Logger
	httpClient   *http.Client
}

// NewHealthHandler creates a HealthHandler that checks the given Qdrant client
// and TEI embedding service URL.
func NewHealthHandler(qdrantClient *qdrant.Client, embeddingURL string, logger *slog.Logger) *HealthHandler {
	return &HealthHandler{
		qdrant:       qdrantClient,
		embeddingURL: embeddingURL,
		logger:       logger,
		httpClient:   &http.Client{Timeout: embeddingHealthTimeout},
	}
}

// HealthResponse is the top-level JSON response for the health endpoint.
type HealthResponse struct {
	Status    string          `json:"status"`    // "ok" if all checks pass, "degraded" otherwise
	Qdrant    ComponentHealth `json:"qdrant"`
	Embedding ComponentHealth `json:"embedding"`
}

// ComponentHealth represents the health status of a single dependency.
type ComponentHealth struct {
	Status  string `json:"status"`            // "ok" or "error"
	Message string `json:"message,omitempty"` // error message when status is "error"
}

// ServeHTTP handles GET /api/v1/health.
//
// Returns HTTP 200 with status "ok" when all dependencies are healthy,
// or HTTP 503 with status "degraded" when any dependency is unreachable.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	qdrantHealth := h.checkQdrant(ctx)
	embeddingHealth := h.checkEmbedding(ctx)

	overallStatus := "ok"
	httpStatus := http.StatusOK
	if qdrantHealth.Status == "error" || embeddingHealth.Status == "error" {
		overallStatus = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}

	resp := HealthResponse{
		Status:    overallStatus,
		Qdrant:    qdrantHealth,
		Embedding: embeddingHealth,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.WarnContext(ctx, "health: failed to encode response", slog.String("error", err.Error()))
	}
}

// checkQdrant verifies Qdrant connectivity by calling the gRPC health check.
func (h *HealthHandler) checkQdrant(ctx context.Context) ComponentHealth {
	_, err := h.qdrant.HealthCheck(ctx)
	if err != nil {
		return ComponentHealth{
			Status:  "error",
			Message: fmt.Sprintf("Qdrant unreachable: %v", err),
		}
	}
	return ComponentHealth{Status: "ok"}
}

// checkEmbedding verifies TEI embedding service connectivity by issuing a GET
// request to the /health endpoint with a 5-second timeout.
func (h *HealthHandler) checkEmbedding(ctx context.Context) ComponentHealth {
	url := h.embeddingURL + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ComponentHealth{
			Status:  "error",
			Message: fmt.Sprintf("build health request: %v", err),
		}
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return ComponentHealth{
			Status:  "error",
			Message: fmt.Sprintf("embedding service unreachable: %v", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ComponentHealth{
			Status:  "error",
			Message: fmt.Sprintf("embedding service returned status %d", resp.StatusCode),
		}
	}
	return ComponentHealth{Status: "ok"}
}
