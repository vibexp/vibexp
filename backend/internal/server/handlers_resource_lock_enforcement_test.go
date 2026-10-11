package server

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Lock enforcement over REST (issue #1289): the update and delete operations
// of prompts, blueprints and artifacts answer a write the service refused
// because of a lock with 423 and code RESOURCE_WRITE_LOCKED, carrying the lock
// in the problem's metadata. Every body is validated against
// backend/openapi.yaml. What the service refuses is covered in
// services/resource_lock_enforcement_test.go.

// lockRefusal is the service's refusal of a write on f's resource, as
// heldLock() would produce it. field is empty for a delete.
func lockRefusal(f *lockFixture, field string) *services.ResourceLockedError {
	return &services.ResourceLockedError{
		ResourceType: f.resourceType, Field: field,
		LockedBy: lockTestLockerID, LockedByName: lockTestLockerName, Comment: lockTestComment,
	}
}

func TestResourceLock_UpdateChangingAFrozenFieldAnswers423(t *testing.T) {
	for _, d := range lockDomains() {
		t.Run(d.name, func(t *testing.T) {
			f := d.setup(t)
			refusal := lockRefusal(f, "slug")
			f.expectUpdate(refusal)

			req, w := f.do(http.MethodPut, f.url, `{"slug":"renamed"}`)

			require.Equal(t, http.StatusLocked, w.Code, w.Body.String())
			assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
			specconformance.AssertConformsToSpec(t, req, w)
			body := decodeBody(t, w)
			assert.Equal(t, apierrors.CodeResourceWriteLocked, body["code"])
			assert.InDelta(t, http.StatusLocked, body["status"], 0)
			assert.Equal(t, refusal.Error(), body["detail"])
			assert.Contains(t, body["detail"], lockTestComment)
			assert.Equal(t, map[string]any{
				"resource_type": f.resourceType,
				"field":         "slug",
				"locked_by":     map[string]any{"user_id": lockTestLockerID, "display_name": lockTestLockerName},
				"comment":       lockTestComment,
			}, body["metadata"])
		})
	}
}

// A refused delete names no field, and nothing that follows a delete runs: no
// embedding or attachment cleanup and no activity (the mocks hold no
// expectation for any of them).
func TestResourceLock_DeleteAnswers423(t *testing.T) {
	for _, d := range lockDomains() {
		t.Run(d.name, func(t *testing.T) {
			f := d.setup(t)
			f.expectLoad(heldLock(), nil)
			f.expectDelete(lockRefusal(f, ""))

			req, w := f.do(http.MethodDelete, f.url, "")

			require.Equal(t, http.StatusLocked, w.Code, w.Body.String())
			specconformance.AssertConformsToSpec(t, req, w)
			body := decodeBody(t, w)
			assert.Equal(t, apierrors.CodeResourceWriteLocked, body["code"])
			assert.Contains(t, body["detail"], lockTestComment)
			assert.Equal(t, map[string]any{
				"resource_type": f.resourceType,
				"locked_by":     map[string]any{"user_id": lockTestLockerID, "display_name": lockTestLockerName},
				"comment":       lockTestComment,
			}, body["metadata"])
		})
	}
}

// A lock refusal wrapped by a caller is still recognized; any other error is
// left to the handler's own mapping.
func TestWriteResourceWriteLocked_OnlyHandlesLockRefusals(t *testing.T) {
	f := promptLockFixture(t)

	assert.False(t, writeResourceWriteLocked(nil, nil, nil))
	assert.False(t, writeResourceWriteLocked(nil, nil, errors.New("boom")))
	assert.False(t, writeResourceWriteLocked(nil, nil, services.ErrPermissionDenied))

	f.expectUpdate(errors.Join(errors.New("update failed"), lockRefusal(f, "status")))
	_, w := f.do(http.MethodPut, f.url, `{"status":"draft"}`)
	require.Equal(t, http.StatusLocked, w.Code, w.Body.String())
	assert.Equal(t, "status", decodeBody(t, w)["metadata"].(map[string]any)["field"])
}
