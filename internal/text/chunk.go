package text

// ChunkSize is the maximum chunk size in characters (~400 tokens for multilingual-e5-base).
// This is sized to stay well under typical model context limits while providing
// enough context per chunk for meaningful semantic retrieval.
const ChunkSize = 1600

// ChunkOverlap is the number of characters shared between adjacent chunks (~50 tokens).
// Overlap ensures that sentences spanning a chunk boundary are represented in both
// adjacent chunks, preventing context loss at boundaries.
const ChunkOverlap = 200

// ChunkText splits text into overlapping character-based chunks ready for embedding.
//
// Chunking uses a sliding window over the Unicode rune sequence (not bytes) to
// handle DE/EN mixed content correctly — German characters like ä, ö, ü are
// multi-byte in UTF-8 but single characters semantically.
//
// Algorithm:
//   - If len(runes) <= ChunkSize, returns []string{text} (single chunk, no split).
//   - Sliding window: start=0, end=start+ChunkSize. Advance start by (ChunkSize - ChunkOverlap).
//   - Each chunk is string(runes[start:end]).
//
// Callers should apply StripMarkup before ChunkText to avoid embedding formatting noise.
func ChunkText(text string) []string {
	runes := []rune(text)
	if len(runes) <= ChunkSize {
		return []string{text}
	}

	var chunks []string
	start := 0
	for {
		end := start + ChunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
		if end == len(runes) {
			break
		}
		start = end - ChunkOverlap
	}

	return chunks
}
