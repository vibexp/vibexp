package errors

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHasLockedResourcesError(t *testing.T) {
	t.Run("a 423 that carries the locked resources in metadata", func(t *testing.T) {
		err := NewHasLockedResourcesError("Unlock them first.", []LockedResource{
			{ResourceType: "artifact", ID: "artifact-1", Slug: "runbook", Title: "Runbook"},
		})

		assert.Equal(t, http.StatusLocked, err.Status)
		assert.Equal(t, CodeHasLockedResources, err.Code)
		assert.Equal(t, "Has Locked Resources", err.Title)
		assert.Equal(t, "Unlock them first.", err.Detail)

		raw, marshalErr := json.Marshal(err.Metadata)
		require.NoError(t, marshalErr)
		assert.JSONEq(t,
			`{"locked_resources":[{"resource_type":"artifact","id":"artifact-1","slug":"runbook","title":"Runbook"}]}`,
			string(raw))
	})

	// locked_resources is a required array of the documented problem: it must
	// never serialize as null.
	t.Run("no resources is an empty array, never null", func(t *testing.T) {
		raw, marshalErr := json.Marshal(NewHasLockedResourcesError("Unlock them first.", nil).Metadata)
		require.NoError(t, marshalErr)
		assert.JSONEq(t, `{"locked_resources":[]}`, string(raw))
	})
}
