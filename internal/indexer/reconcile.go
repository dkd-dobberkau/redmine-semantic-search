package indexer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/oliverpool/redmine-semantic-search/internal/redmine"
	qdrantpkg "github.com/oliverpool/redmine-semantic-search/internal/qdrant"
	"github.com/qdrant/go-client/qdrant"
	"github.com/robfig/cron/v3"
)

const (
	// scrollPageSize is the number of Qdrant points fetched per scroll page
	// during deletion reconciliation.
	scrollPageSize = 1000

	// deleteBatchSize is the maximum number of orphaned points deleted per
	// Delete call during reconciliation.
	deleteBatchSize = 500
)

// Reconciler runs periodic deletion reconciliation between Qdrant and Redmine.
//
// On each run it:
//  1. Fetches all Redmine issue IDs (admin key, all statuses).
//  2. Scrolls all Qdrant points with content_type=issue, fetching only the
//     redmine_id payload field.
//  3. Deletes any Qdrant points whose redmine_id does not appear in Redmine
//     (these are orphans from deleted issues).
//
// The reconciler is driven by a cron schedule (default: "0 */6 * * *" = every
// 6 hours) so it runs infrequently and does not compete with incremental sync.
type Reconciler struct {
	redmine      *redmine.Client
	qdrant       *qdrant.Client
	statusFilter string
	cron         *cron.Cron
	logger       *slog.Logger
}

// NewReconciler creates a Reconciler with the given cron schedule expression.
// The reconciler does NOT start until Start() is called.
func NewReconciler(
	redmineClient *redmine.Client,
	qdrantClient *qdrant.Client,
	schedule string,
	statusFilter string,
	logger *slog.Logger,
) (*Reconciler, error) {
	r := &Reconciler{
		redmine:      redmineClient,
		qdrant:       qdrantClient,
		statusFilter: statusFilter,
		cron:         cron.New(),
		logger:       logger,
	}

	if _, err := r.cron.AddFunc(schedule, func() {
		r.reconcile(context.Background())
	}); err != nil {
		return nil, fmt.Errorf("invalid reconcile schedule %q: %w", schedule, err)
	}

	return r, nil
}

// Start begins the cron scheduler. The reconciler runs on the configured
// schedule until Stop is called.
func (r *Reconciler) Start() {
	r.cron.Start()
}

// Stop halts the cron scheduler and waits for any in-progress reconciliation
// run to finish before returning.
func (r *Reconciler) Stop() context.Context {
	return r.cron.Stop()
}

// reconcile performs a full ID diff between Qdrant and Redmine, deleting any
// orphaned Qdrant points that no longer exist in Redmine.
func (r *Reconciler) reconcile(ctx context.Context) {
	r.logger.Info("reconcile: starting ID diff")

	// Step 1: Fetch all current Redmine issue IDs.
	redmineIDs, err := r.redmine.FetchAllIssueIDs(ctx, r.statusFilter)
	if err != nil {
		r.logger.Error("reconcile: failed to fetch Redmine issue IDs", "error", err)
		return
	}

	redmineSet := make(map[int]struct{}, len(redmineIDs))
	for _, id := range redmineIDs {
		redmineSet[id] = struct{}{}
	}

	// Step 2: Scroll all Qdrant issue points, collecting orphan point IDs.
	var orphans []*qdrant.PointId
	var offset *qdrant.PointId
	qdrantCount := 0

	for {
		limit := uint32(scrollPageSize)
		points, nextOffset, err := r.qdrant.ScrollAndOffset(ctx, &qdrant.ScrollPoints{
			CollectionName: qdrantpkg.AliasName,
			Filter: &qdrant.Filter{
				Must: []*qdrant.Condition{
					qdrant.NewMatch("content_type", "issue"),
				},
			},
			Offset:      offset,
			Limit:       &limit,
			WithPayload: qdrant.NewWithPayloadInclude("redmine_id"),
			WithVectors: qdrant.NewWithVectors(false),
		})
		if err != nil {
			r.logger.Error("reconcile: failed to scroll Qdrant points", "error", err)
			return
		}

		qdrantCount += len(points)

		for _, pt := range points {
			redmineIDVal, ok := pt.GetPayload()["redmine_id"]
			if !ok {
				// Point missing redmine_id — treat as orphan.
				r.logger.Warn("reconcile: point missing redmine_id payload, marking as orphan", "point_id", pt.GetId())
				orphans = append(orphans, pt.GetId())
				continue
			}

			redmineID := int(redmineIDVal.GetIntegerValue())
			if _, exists := redmineSet[redmineID]; !exists {
				orphans = append(orphans, pt.GetId())
			}
		}

		// nil nextOffset means no more pages.
		if nextOffset == nil {
			break
		}
		offset = nextOffset
	}

	// Step 3: Delete orphaned points in batches.
	orphanCount := len(orphans)
	if orphanCount == 0 {
		r.logger.Info("reconcile: complete, no orphans found",
			"total_qdrant", qdrantCount,
			"total_redmine", len(redmineIDs),
		)
		return
	}

	trueVal := true
	deleted := 0
	for start := 0; start < orphanCount; start += deleteBatchSize {
		end := start + deleteBatchSize
		if end > orphanCount {
			end = orphanCount
		}
		batch := orphans[start:end]

		if _, err := r.qdrant.Delete(ctx, &qdrant.DeletePoints{
			CollectionName: qdrantpkg.AliasName,
			Wait:           &trueVal,
			Points:         qdrant.NewPointsSelectorIDs(batch),
		}); err != nil {
			r.logger.Error("reconcile: failed to delete orphaned points",
				"batch_start", start,
				"batch_end", end-1,
				"error", err,
			)
			// Continue with remaining batches — partial deletion is better than none.
			continue
		}
		deleted += len(batch)
	}

	r.logger.Info("reconcile: complete",
		"orphans_deleted", deleted,
		"total_qdrant", qdrantCount,
		"total_redmine", len(redmineIDs),
	)
}
