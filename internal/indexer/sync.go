// Package indexer provides the incremental sync scheduler that keeps the Qdrant
// index fresh by polling Redmine for updated issues on a configurable interval.
package indexer

import (
	"context"
	"log/slog"
	"time"

	"github.com/oliverpool/redmine-semantic-search/internal/redmine"
)

// Syncer polls Redmine for issues updated since the last cursor and indexes
// them via the Pipeline. It operates as a non-blocking background goroutine
// so the service remains responsive immediately after startup.
//
// Design decisions:
//   - Cursor starts at zero (epoch), so the first run fetches everything.
//   - One page per cycle (bounded by batch size) — avoids long-running cycles
//     that would delay subsequent polls.
//   - Cursor advances only after successful indexing — partial failures retry
//     the same batch next cycle.
type Syncer struct {
	redmine      *redmine.Client
	pipeline     *Pipeline
	interval     time.Duration
	batch        int          // max issues per poll cycle (config SyncBatchSize)
	statusFilter string       // Redmine status_id filter ("open" or "*")
	cursor       time.Time    // updated_on cursor — advances after each successful batch
	logger       *slog.Logger
	cancel       context.CancelFunc
	done         chan struct{}
}

// NewSyncer constructs a Syncer. The cursor starts at the zero time (epoch),
// meaning the first poll fetches all issues from the beginning.
func NewSyncer(
	redmineClient *redmine.Client,
	pipeline *Pipeline,
	interval time.Duration,
	batchSize int,
	statusFilter string,
	logger *slog.Logger,
) *Syncer {
	return &Syncer{
		redmine:      redmineClient,
		pipeline:     pipeline,
		interval:     interval,
		batch:        batchSize,
		statusFilter: statusFilter,
		cursor:       time.Time{},
		logger:       logger,
		done:         make(chan struct{}),
	}
}

// Start begins the incremental polling loop in a background goroutine.
// The first poll runs immediately; subsequent polls run on the configured interval.
// Start is non-blocking and returns immediately after launching the goroutine.
func (s *Syncer) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	go func() {
		defer close(s.done)

		// First poll runs immediately on start.
		s.poll(ctx)

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.poll(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop cancels the polling context and waits for the goroutine to exit cleanly.
// In-flight poll operations are interrupted via context cancellation.
func (s *Syncer) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	<-s.done
}

// poll fetches one page of issues updated since the cursor and indexes them.
//
// Cursor advancement:
//   - Only advances on full success (fetch + index).
//   - If either step fails, the cursor stays at the previous value and the
//     same batch is retried on the next cycle.
//
// Bounded page approach: fetches at most s.batch issues per cycle. If more
// pages exist, they are picked up in subsequent cycles. This keeps each cycle
// fast and the service responsive.
func (s *Syncer) poll(ctx context.Context) {
	s.logger.Info("sync: polling", "since", s.cursor)

	issueList, err := s.redmine.FetchIssuesSince(ctx, s.cursor, 0, s.batch, s.statusFilter)
	if err != nil {
		if ctx.Err() != nil {
			// Context was cancelled — shutdown in progress, exit silently.
			return
		}
		s.logger.Error("sync: fetch failed", "error", err)
		return
	}

	if len(issueList.Issues) == 0 {
		s.logger.Info("sync: no updates")
		return
	}

	if err := s.pipeline.IndexIssues(ctx, issueList.Issues); err != nil {
		if ctx.Err() != nil {
			return
		}
		s.logger.Error("sync: index failed", "error", err)
		// Cursor NOT advanced — retry same batch next cycle.
		return
	}

	// Fetch and index journals for each issue in this batch.
	for _, issue := range issueList.Issues {
		detail, err := s.redmine.FetchIssueWithJournals(ctx, issue.ID)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.logger.Warn("sync: fetch journals failed", "issue_id", issue.ID, "error", err)
			continue // non-fatal: skip journals for this issue
		}
		if len(detail.Journals) == 0 {
			continue
		}
		if err := s.pipeline.IndexJournals(ctx, issue, detail.Journals); err != nil {
			if ctx.Err() != nil {
				return
			}
			s.logger.Warn("sync: index journals failed", "issue_id", issue.ID, "error", err)
			// non-fatal: continue with next issue
		}
	}

	// Advance cursor to the max updated_on timestamp from this batch.
	newCursor := s.maxUpdatedOn(issueList.Issues)
	s.cursor = newCursor

	s.logger.Info("sync: indexed", "count", len(issueList.Issues), "cursor", s.cursor)

	// If more pages exist, log a hint — they will be fetched next cycle.
	if issueList.TotalCount > issueList.Offset+len(issueList.Issues) {
		s.logger.Info("sync: more pages remain, will fetch next cycle",
			"fetched", len(issueList.Issues),
			"total", issueList.TotalCount,
		)
	}
}

// maxUpdatedOn returns the maximum updated_on timestamp across all issues.
// If no issues are present or all timestamps fail to parse, it returns s.cursor
// unchanged to avoid regressing the cursor.
func (s *Syncer) maxUpdatedOn(issues []redmine.Issue) time.Time {
	max := s.cursor
	for _, issue := range issues {
		t, err := time.Parse(time.RFC3339, issue.UpdatedOn)
		if err != nil {
			s.logger.Warn("sync: failed to parse updated_on", "issue_id", issue.ID, "value", issue.UpdatedOn)
			continue
		}
		if t.After(max) {
			max = t
		}
	}
	return max
}
