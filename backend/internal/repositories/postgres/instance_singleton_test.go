package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/internal/repositories"
)

// TestCheckSingletonVersion: the audited singleton upserts turn the shared
// compare-and-set rule into ErrInstanceSettingsVersionConflict. The rule's own
// cases are pinned by repositories.TestInstanceSettingsVersionConflicts.
func TestCheckSingletonVersion(t *testing.T) {
	nothing := repositories.InstanceSettingsNoStoredVersion
	stored := int64(1)

	assert.NoError(t, checkSingletonVersion(nil, &stored), "nil is last-write-wins")
	assert.NoError(t, checkSingletonVersion(&nothing, nil), "nothing stored, as expected")
	assert.NoError(t, checkSingletonVersion(&stored, &stored), "the stored version")
	assert.ErrorIs(t, checkSingletonVersion(&nothing, &stored), repositories.ErrInstanceSettingsVersionConflict,
		"a row is stored")
	assert.ErrorIs(t, checkSingletonVersion(&stored, nil), repositories.ErrInstanceSettingsVersionConflict,
		"no row to match")
}
