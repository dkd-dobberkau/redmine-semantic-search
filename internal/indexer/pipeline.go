// Package indexer implements the issue indexing pipeline: it transforms Redmine
// issues into Qdrant vectors by stripping markup, chunking text, embedding via
// TEI, and upserting points with full payload metadata.
package indexer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/oliverpool/redmine-semantic-search/internal/embedder"
	"github.com/oliverpool/redmine-semantic-search/internal/redmine"
	"github.com/oliverpool/redmine-semantic-search/internal/text"
	qdrantpkg "github.com/oliverpool/redmine-semantic-search/internal/qdrant"
	"github.com/qdrant/go-client/qdrant"
)

const (
	// upsertBatchSize is the maximum number of Qdrant points per upsert call.
	// Qdrant handles large batches well, but this bound limits peak memory usage.
	upsertBatchSize = 100
)

// chunkEntry holds all data for a single chunk derived from a Redmine issue.
// It is used to correlate embedding results back to their source issue and
// chunk position after the batch embedding call returns.
type chunkEntry struct {
	issue      redmine.Issue
	chunkIndex int
	chunkTotal int
	text       string
}

// journalChunkEntry holds all data for a single chunk derived from a journal.
type journalChunkEntry struct {
	issue      redmine.Issue
	journal    redmine.Journal
	chunkIndex int
	chunkTotal int
	text       string
}

// Pipeline transforms Redmine issues into Qdrant vectors.
// Dependencies are injected via the struct to allow easy testing.
type Pipeline struct {
	embedder embedder.Embedder
	qdrant   *qdrant.Client
	logger   *slog.Logger
}

// NewPipeline constructs a Pipeline with the given dependencies.
func NewPipeline(emb embedder.Embedder, qdrantClient *qdrant.Client, logger *slog.Logger) *Pipeline {
	return &Pipeline{
		embedder: emb,
		qdrant:   qdrantClient,
		logger:   logger,
	}
}

// IndexIssues indexes a batch of Redmine issues into Qdrant.
//
// For each issue the pipeline:
//  1. Builds embeddable text: subject + stripped description (or subject only if no description).
//  2. Splits the text into overlapping chunks via text.ChunkText.
//  3. Deletes all existing Qdrant points for this issue (handles re-indexing when chunk count changes).
//  4. Embeds all chunks from all issues in a single batched call to EmbedPassages.
//  5. Upserts the resulting points to Qdrant in batches of upsertBatchSize.
//
// The function returns the first error encountered, if any.
func (p *Pipeline) IndexIssues(ctx context.Context, issues []redmine.Issue) error {
	if len(issues) == 0 {
		return nil
	}

	// Step 1 & 2: Build chunk entries for all issues.
	var entries []chunkEntry
	for _, issue := range issues {
		fullText := buildFullText(issue)
		chunks := text.ChunkText(fullText)

		// Step 3: Delete all existing chunks for this issue before re-upserting.
		// This prevents stale chunk orphans when the chunk count changes on re-index.
		if err := p.DeleteIssueChunks(ctx, issue.ID); err != nil {
			return fmt.Errorf("delete existing chunks for issue %d: %w", issue.ID, err)
		}

		chunkTotal := len(chunks)
		for i, chunk := range chunks {
			entries = append(entries, chunkEntry{
				issue:      issue,
				chunkIndex: i,
				chunkTotal: chunkTotal,
				text:       chunk,
			})
		}
	}

	if len(entries) == 0 {
		return nil
	}

	// Step 4: Extract chunk texts for batch embedding.
	chunkTexts := make([]string, len(entries))
	for i, e := range entries {
		chunkTexts[i] = e.text
	}

	// EmbedPassages handles TEI's batch-32 limit internally (already chunked in tei.go).
	embeddings, err := p.embedder.EmbedPassages(ctx, chunkTexts)
	if err != nil {
		return fmt.Errorf("embed passages: %w", err)
	}
	if len(embeddings) != len(entries) {
		return fmt.Errorf("embedding count mismatch: got %d, want %d", len(embeddings), len(entries))
	}

	// Step 5: Build PointStruct slice and upsert in batches.
	points := make([]*qdrant.PointStruct, len(entries))
	for i, e := range entries {
		chunkID := qdrantpkg.ChunkPointID(e.issue.ID, e.chunkIndex)
		points[i] = &qdrant.PointStruct{
			Id:      qdrant.NewIDUUID(chunkID),
			Vectors: qdrant.NewVectors(embeddings[i]...),
			Payload: qdrant.NewValueMap(map[string]any{
				"redmine_id":   e.issue.ID,
				"content_type": "issue",
				"project_id":   e.issue.Project.ID,
				"tracker":      e.issue.Tracker.Name,
				"status":       e.issue.Status.Name,
				"author":       e.issue.Author.Name,
				"author_id":    e.issue.Author.ID,
				"subject":      e.issue.Subject,
				"is_private":   e.issue.IsPrivate,
				"text_preview": truncate(e.text, 500),
				"chunk_index":  e.chunkIndex,
				"chunk_total":  e.chunkTotal,
				"created_on":   e.issue.CreatedOn,
				"updated_on":   e.issue.UpdatedOn,
			}),
		}
	}

	trueVal := true
	for start := 0; start < len(points); start += upsertBatchSize {
		end := start + upsertBatchSize
		if end > len(points) {
			end = len(points)
		}
		batch := points[start:end]
		if _, err := p.qdrant.Upsert(ctx, &qdrant.UpsertPoints{
			CollectionName: qdrantpkg.AliasName,
			Wait:           &trueVal,
			Points:         batch,
		}); err != nil {
			return fmt.Errorf("upsert points (batch %d-%d): %w", start, end-1, err)
		}
	}

	p.logger.Info("indexed issues", "count", len(issues), "chunks", len(entries))
	return nil
}

// DeleteIssueChunks removes all existing Qdrant points for the given Redmine issue ID.
// This is called before re-upserting chunks to prevent stale chunk orphans when
// the number of chunks changes during re-indexing.
func (p *Pipeline) DeleteIssueChunks(ctx context.Context, redmineID int) error {
	trueVal := true
	_, err := p.qdrant.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: qdrantpkg.AliasName,
		Wait:           &trueVal,
		Points: qdrant.NewPointsSelectorFilter(&qdrant.Filter{
			Must: []*qdrant.Condition{
				qdrant.NewMatch("content_type", "issue"),
				qdrant.NewMatchInt("redmine_id", int64(redmineID)),
			},
		}),
	})
	if err != nil {
		return fmt.Errorf("delete chunks for redmine_id %d: %w", redmineID, err)
	}
	return nil
}

// IndexJournals indexes journal entries for a single issue into Qdrant.
// Each journal with non-empty notes becomes its own set of vector chunks with
// content_type "journal", linked back to the parent issue via redmine_id.
func (p *Pipeline) IndexJournals(ctx context.Context, issue redmine.Issue, journals []redmine.Journal) error {
	if len(journals) == 0 {
		return nil
	}

	// Delete all existing journal chunks for this issue before re-upserting.
	if err := p.DeleteJournalChunks(ctx, issue.ID); err != nil {
		return fmt.Errorf("delete existing journal chunks for issue %d: %w", issue.ID, err)
	}

	var entries []journalChunkEntry
	for _, j := range journals {
		// Prefix with issue subject for context when searching.
		noteText := text.StripMarkup(j.Notes)
		if noteText == "" {
			continue
		}
		fullText := issue.Subject + "\n\n" + noteText
		chunks := text.ChunkText(fullText)
		chunkTotal := len(chunks)
		for i, chunk := range chunks {
			entries = append(entries, journalChunkEntry{
				issue:      issue,
				journal:    j,
				chunkIndex: i,
				chunkTotal: chunkTotal,
				text:       chunk,
			})
		}
	}

	if len(entries) == 0 {
		return nil
	}

	chunkTexts := make([]string, len(entries))
	for i, e := range entries {
		chunkTexts[i] = e.text
	}

	embeddings, err := p.embedder.EmbedPassages(ctx, chunkTexts)
	if err != nil {
		return fmt.Errorf("embed journal passages: %w", err)
	}
	if len(embeddings) != len(entries) {
		return fmt.Errorf("journal embedding count mismatch: got %d, want %d", len(embeddings), len(entries))
	}

	points := make([]*qdrant.PointStruct, len(entries))
	for i, e := range entries {
		chunkID := qdrantpkg.JournalChunkPointID(e.journal.ID, e.chunkIndex)
		points[i] = &qdrant.PointStruct{
			Id:      qdrant.NewIDUUID(chunkID),
			Vectors: qdrant.NewVectors(embeddings[i]...),
			Payload: qdrant.NewValueMap(map[string]any{
				"redmine_id":    e.issue.ID,
				"journal_id":    e.journal.ID,
				"content_type":  "journal",
				"project_id":    e.issue.Project.ID,
				"tracker":       e.issue.Tracker.Name,
				"status":        e.issue.Status.Name,
				"author":        e.journal.User.Name,
				"author_id":     e.journal.User.ID,
				"subject":       e.issue.Subject,
				"is_private":    e.journal.PrivateNotes,
				"text_preview":  truncate(e.text, 500),
				"chunk_index":   e.chunkIndex,
				"chunk_total":   e.chunkTotal,
				"created_on":    e.journal.CreatedOn,
				"updated_on":    e.journal.CreatedOn,
			}),
		}
	}

	trueVal := true
	for start := 0; start < len(points); start += upsertBatchSize {
		end := start + upsertBatchSize
		if end > len(points) {
			end = len(points)
		}
		if _, err := p.qdrant.Upsert(ctx, &qdrant.UpsertPoints{
			CollectionName: qdrantpkg.AliasName,
			Wait:           &trueVal,
			Points:         points[start:end],
		}); err != nil {
			return fmt.Errorf("upsert journal points (batch %d-%d): %w", start, end-1, err)
		}
	}

	p.logger.Info("indexed journals", "issue_id", issue.ID, "journals", len(journals), "chunks", len(entries))
	return nil
}

// DeleteJournalChunks removes all existing journal Qdrant points for a given issue.
func (p *Pipeline) DeleteJournalChunks(ctx context.Context, redmineID int) error {
	trueVal := true
	_, err := p.qdrant.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: qdrantpkg.AliasName,
		Wait:           &trueVal,
		Points: qdrant.NewPointsSelectorFilter(&qdrant.Filter{
			Must: []*qdrant.Condition{
				qdrant.NewMatch("content_type", "journal"),
				qdrant.NewMatchInt("redmine_id", int64(redmineID)),
			},
		}),
	})
	if err != nil {
		return fmt.Errorf("delete journal chunks for redmine_id %d: %w", redmineID, err)
	}
	return nil
}

// buildFullText constructs the embeddable text for an issue.
// If the description is non-empty, it prepends the subject and appends the
// stripped description. Otherwise, only the subject is used.
func buildFullText(issue redmine.Issue) string {
	stripped := text.StripMarkup(issue.Description)
	if stripped == "" {
		return issue.Subject
	}
	return issue.Subject + "\n\n" + stripped
}

// truncate returns the first maxLen runes of s.
// If s is shorter than or equal to maxLen runes, it is returned unchanged.
// This is used to generate the text_preview payload field.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}
