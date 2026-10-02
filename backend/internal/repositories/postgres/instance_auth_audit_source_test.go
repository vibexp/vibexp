package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/repositories"
)

func TestStampAuditSource(t *testing.T) {
	cli := repositories.WithAuditSource(context.Background(), repositories.AuditSourceCLI)
	doc := func(s string) json.RawMessage { return json.RawMessage(s) }

	t.Run("no source in the context leaves both snapshots untouched", func(t *testing.T) {
		before, after, err := stampAuditSource(context.Background(), doc(`{"a":1}`), doc(`{"a":2}`))
		require.NoError(t, err)
		assert.JSONEq(t, `{"a":1}`, string(before))
		assert.JSONEq(t, `{"a":2}`, string(after))
	})

	t.Run("an upsert is marked on after only", func(t *testing.T) {
		before, after, err := stampAuditSource(cli, doc(`{"a":1}`), doc(`{"a":2}`))
		require.NoError(t, err)
		assert.JSONEq(t, `{"a":1}`, string(before))
		assert.JSONEq(t, `{"a":2,"source":"cli"}`, string(after))
	})

	t.Run("a first write has no before", func(t *testing.T) {
		before, after, err := stampAuditSource(cli, nil, doc(`{"a":2}`))
		require.NoError(t, err)
		assert.Nil(t, before)
		assert.JSONEq(t, `{"a":2,"source":"cli"}`, string(after))
	})

	t.Run("a delete keeps a nil after and is marked on before", func(t *testing.T) {
		before, after, err := stampAuditSource(cli, doc(`{"a":1}`), nil)
		require.NoError(t, err)
		assert.JSONEq(t, `{"a":1,"source":"cli"}`, string(before))
		assert.Nil(t, after)
	})

	t.Run("nothing to mark", func(t *testing.T) {
		before, after, err := stampAuditSource(cli, nil, nil)
		require.NoError(t, err)
		assert.Nil(t, before)
		assert.Nil(t, after)
	})

	t.Run("a snapshot that is not an object is an error, not a silent skip", func(t *testing.T) {
		_, _, err := stampAuditSource(cli, nil, doc(`[1]`))
		require.Error(t, err)
		_, _, err = stampAuditSource(cli, doc(`[1]`), nil)
		require.Error(t, err)
	})
}
