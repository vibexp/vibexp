package services

import "strings"

// defaultChunkSize and defaultChunkOverlap are used when a TextChunker is
// constructed with non-positive sizing. They mirror the windowing the previous
// external ai-service used so retrieval quality does not regress.
const (
	defaultChunkSize    = 1000
	defaultChunkOverlap = 200
)

// Chunker splits document text into overlapping chunks suitable for embedding.
// Chunking now happens in Go (the OpenAI-compatible /v1/embeddings API embeds
// text→vectors but does not chunk), replacing the chunking the external
// ai-service used to perform.
type Chunker interface {
	// Chunk splits text into one or more chunks. It returns nil for empty or
	// whitespace-only input.
	Chunk(text string) []string
}

// TextChunker is a rune-based sliding-window chunker. It emits windows of at most
// chunkSize runes with chunkOverlap runes shared between consecutive windows.
// Operating on runes (not bytes) keeps multi-byte UTF-8 characters intact, and
// the overlap preserves context across chunk boundaries so a sentence split
// across two windows can still match a query.
type TextChunker struct {
	chunkSize int
	overlap   int
}

// Ensure TextChunker implements Chunker.
var _ Chunker = (*TextChunker)(nil)

// NewTextChunker creates a TextChunker, clamping its parameters so Chunk always
// makes forward progress: a non-positive chunkSize falls back to the default,
// and overlap is clamped to [0, chunkSize-1] so the window step (chunkSize-overlap)
// is always >= 1.
func NewTextChunker(chunkSize, overlap int) *TextChunker {
	if chunkSize < 1 {
		chunkSize = defaultChunkSize
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap > chunkSize-1 {
		overlap = chunkSize - 1
	}
	return &TextChunker{chunkSize: chunkSize, overlap: overlap}
}

// Chunk splits text into overlapping windows. Whitespace-only input yields nil.
// Input that fits in a single window is returned as one trimmed chunk.
func (c *TextChunker) Chunk(text string) []string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return nil
	}
	if len(runes) <= c.chunkSize {
		return []string{string(runes)}
	}

	step := c.chunkSize - c.overlap // >= 1, guaranteed by NewTextChunker
	var chunks []string
	for start := 0; start < len(runes); start += step {
		end := start + c.chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		if chunk := strings.TrimSpace(string(runes[start:end])); chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(runes) {
			break
		}
	}
	return chunks
}
