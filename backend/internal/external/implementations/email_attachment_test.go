package implementations

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAttachmentContentType(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{"known pdf extension", "report.pdf", "application/pdf"},
		{"known png extension", "logo.png", "image/png"},
		{"no extension falls back to octet-stream", "noextension", "application/octet-stream"},
		{"unknown extension falls back to octet-stream", "data.zzqq", "application/octet-stream"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, attachmentContentType(tc.filename))
		})
	}
}
