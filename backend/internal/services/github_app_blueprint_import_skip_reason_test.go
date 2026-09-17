package services

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/internal/external"
)

func TestCompanionPreflightSkip_EmptyFile(t *testing.T) {
	reason, skip := companionPreflightSkip(&external.GitHubFile{Path: "skills/demo/empty.txt"})

	assert.True(t, skip)
	assert.Equal(t, "Empty file", reason)
}

func TestCompanionSkipReason_EmptyAttachment(t *testing.T) {
	err := fmt.Errorf("store companion: %w", ErrAttachmentEmpty)

	assert.Equal(t, "Empty file", companionSkipReason(err))
}
