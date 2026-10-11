package repositories

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// A LockedResourcesError is found with errors.As through any wrapping, and its
// reason with errors.Is, which is how the services and handlers branch on it.
func TestLockedResourcesError(t *testing.T) {
	refs := []models.LockedResourceRef{
		{ResourceType: models.RelationResourceTypeArtifact, ID: "artifact-1", Slug: "runbook", Title: "Runbook"},
	}
	err := fmt.Errorf("Migrate: %w", &LockedResourcesError{Reason: ErrTypeUsedByLockedResource, Resources: refs})

	assert.Equal(t, "Migrate: type is used by a locked resource (1 listed)", err.Error())
	require.ErrorIs(t, err, ErrTypeUsedByLockedResource)
	assert.False(t, errors.Is(err, ErrProjectHasLockedResources))
	assert.False(t, errors.Is(err, ErrTeamStatusUsedByLockedResource))

	var locked *LockedResourcesError
	require.ErrorAs(t, err, &locked)
	assert.Equal(t, refs, locked.Resources)
}
