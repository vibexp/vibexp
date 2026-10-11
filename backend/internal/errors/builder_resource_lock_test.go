package errors

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewResourceWriteLockedError(t *testing.T) {
	lock := ResourceWriteLock{
		ResourceType: "artifact", Field: "status",
		LockedByID: "user-1", LockedByName: "Ada Lovelace", Comment: "Used by the release agent",
	}

	t.Run("a refused update carries the lock and the frozen field", func(t *testing.T) {
		err := NewResourceWriteLockedError("cannot change status", lock)

		assert.Equal(t, http.StatusLocked, err.Status)
		assert.Equal(t, CodeResourceWriteLocked, err.Code)
		assert.Equal(t, "RESOURCE_WRITE_LOCKED", err.Code, "clients key on this value")
		assert.NotEqual(t, CodeResourceLocked, err.Code, "RESOURCE_LOCKED is the 409 for locking twice")
		assert.Equal(t, "Resource Write Locked", err.Title)
		assert.Equal(t, "cannot change status", err.Detail)
		assert.Equal(t, map[string]any{
			"resource_type": "artifact",
			"field":         "status",
			"locked_by":     map[string]any{"user_id": "user-1", "display_name": "Ada Lovelace"},
			"comment":       "Used by the release agent",
		}, err.Metadata)
	})

	t.Run("a refused delete names no field", func(t *testing.T) {
		lock.Field = ""

		err := NewResourceWriteLockedError("cannot delete", lock)

		assert.NotContains(t, err.Metadata, "field")
		assert.Equal(t, "Used by the release agent", err.Metadata["comment"])
	})
}
