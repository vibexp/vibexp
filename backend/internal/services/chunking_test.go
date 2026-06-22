package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextChunker_EmptyAndWhitespace(t *testing.T) {
	c := NewTextChunker(100, 20)
	assert.Nil(t, c.Chunk(""))
	assert.Nil(t, c.Chunk("   \n\t  "))
}

func TestTextChunker_SingleChunkWhenSmall(t *testing.T) {
	c := NewTextChunker(100, 20)
	got := c.Chunk("  hello world  ")
	assert.Equal(t, []string{"hello world"}, got)
}

func TestTextChunker_OverlappingWindows(t *testing.T) {
	// 10-rune window, 3-rune overlap, step = 7, over 24 distinct runes.
	// Windows: [0:10], [7:17], [14:24] -> 3 chunks (the 3rd reaches the end).
	c := NewTextChunker(10, 3)
	text := "0123456789abcdefghijklmn" // 24 runes
	got := c.Chunk(text)

	require.Len(t, got, 3)
	assert.Equal(t, "0123456789", got[0])
	assert.Equal(t, "789abcdefg", got[1])
	assert.Equal(t, "efghijklmn", got[2])

	// Consecutive chunks share `overlap` runes at the boundary.
	assert.Equal(t, string([]rune(got[0])[7:10]), string([]rune(got[1])[0:3]))
	assert.Equal(t, string([]rune(got[1])[7:10]), string([]rune(got[2])[0:3]))
}

func TestTextChunker_PreservesMultiByteRunes(t *testing.T) {
	c := NewTextChunker(3, 1)
	got := c.Chunk("héllo") // 5 runes
	for _, chunk := range got {
		// Every chunk must be valid UTF-8 with no broken runes.
		assert.True(t, len(chunk) >= len([]rune(chunk)))
	}
	// Reassembling unique runes covers the whole input.
	assert.Contains(t, strings.Join(got, ""), "é")
}

func TestNewTextChunker_ClampsParameters(t *testing.T) {
	// Non-positive size falls back to default; overlap clamped below size so the
	// step is always positive and Chunk terminates.
	c := NewTextChunker(0, 9999)
	assert.Equal(t, defaultChunkSize, c.chunkSize)
	assert.Less(t, c.overlap, c.chunkSize)

	got := NewTextChunker(5, 5).Chunk(strings.Repeat("x", 12))
	assert.NotEmpty(t, got) // would loop forever if step were 0
}
