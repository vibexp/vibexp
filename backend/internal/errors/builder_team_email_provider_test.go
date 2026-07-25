package errors

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The team email provider builders (#503). Each pins its status code, because the
// status is the contract the spec documents and a silent change would break
// clients branching on it.
func TestTeamEmailProviderErrorBuilders(t *testing.T) {
	t.Run("not configured is a 409, not a 404", func(t *testing.T) {
		// The endpoint exists and the team is addressable — it is simply not in a
		// state that can serve the request. A 404 would suggest a bad URL.
		err := NewTeamEmailProviderNotConfiguredError()
		assert.Equal(t, http.StatusConflict, err.Status)
		assert.Equal(t, CodeTeamEmailProviderNotConfigured, err.Code)
		assert.Equal(t, "Team Email Provider Not Configured", err.Title)
	})

	t.Run("validation carries the offending fields", func(t *testing.T) {
		err := NewTeamEmailProviderValidationError("bad config", []ValidationError{
			{Field: "settings.smtp.port", Message: "must be a number between 1 and 65535"},
		})
		assert.Equal(t, http.StatusBadRequest, err.Status)
		assert.Equal(t, CodeTeamEmailProviderValidationFailed, err.Code)
		require.Len(t, err.ValidationErrors, 1)
		assert.Equal(t, "settings.smtp.port", err.ValidationErrors[0].Field)
	})

	t.Run("update failed", func(t *testing.T) {
		err := NewTeamEmailProviderUpdateFailedError("nope")
		assert.Equal(t, http.StatusInternalServerError, err.Status)
		assert.Equal(t, CodeTeamEmailProviderUpdateFailed, err.Code)
	})

	t.Run("delete failed", func(t *testing.T) {
		err := NewTeamEmailProviderDeleteFailedError("nope")
		assert.Equal(t, http.StatusInternalServerError, err.Status)
		assert.Equal(t, CodeTeamEmailProviderDeleteFailed, err.Code)
	})
}
