package repositories

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuditSource(t *testing.T) {
	ctx := context.Background()

	assert.Empty(t, AuditSourceFromContext(ctx), "a plain context names no source")
	assert.Equal(t, AuditSourceCLI, AuditSourceFromContext(WithAuditSource(ctx, AuditSourceCLI)))
	assert.Equal(t, ctx, WithAuditSource(ctx, ""), "an empty source leaves the context untouched")
}
