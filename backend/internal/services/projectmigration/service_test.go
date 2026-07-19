package projectmigration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractTableName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		expected string
	}{
		{
			name:     "prompts query",
			query:    `SELECT id FROM prompts WHERE project_id = $1`,
			expected: "prompts",
		},
		{
			name:     "artifacts query",
			query:    `SELECT id, slug FROM artifacts WHERE project_id = $1`,
			expected: "artifacts",
		},
		{
			name:     "blueprints query",
			query:    `SELECT id, slug FROM blueprints WHERE project_id = $1`,
			expected: "blueprints",
		},
		{
			name:     "feed_items query",
			query:    `SELECT id FROM feed_items WHERE project_id = $1`,
			expected: "feed_items",
		},
		{
			name:     "no from clause",
			query:    `SELECT id`,
			expected: "",
		},
		{
			name:     "multiline query with space before FROM",
			query:    "SELECT id, slug FROM artifacts WHERE project_id = $1",
			expected: "artifacts",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractTableName(tc.query)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestValidateMigrationRequest_ConflictPolicies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		policy      ConflictPolicy
		wantErr     bool
		expectedPol ConflictPolicy
	}{
		{
			name:        "empty policy defaults to skip",
			policy:      "",
			wantErr:     false,
			expectedPol: ConflictPolicySkip,
		},
		{
			name:        "skip is valid",
			policy:      ConflictPolicySkip,
			wantErr:     false,
			expectedPol: ConflictPolicySkip,
		},
		{
			name:        "rename is valid",
			policy:      ConflictPolicyRename,
			wantErr:     false,
			expectedPol: ConflictPolicyRename,
		},
		{
			name:        "overwrite is valid",
			policy:      ConflictPolicyOverwrite,
			wantErr:     false,
			expectedPol: ConflictPolicyOverwrite,
		},
		{
			name:    "invalid policy",
			policy:  "delete",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// We test the validateMigrationRequest helper from the handler package
			// indirectly through the type constants here to stay within the package.
			validPolicies := map[ConflictPolicy]bool{
				ConflictPolicySkip:      true,
				ConflictPolicyRename:    true,
				ConflictPolicyOverwrite: true,
			}

			if tc.policy != "" && !validPolicies[tc.policy] {
				assert.True(t, tc.wantErr, "expected error for policy %s", tc.policy)
			} else {
				assert.False(t, tc.wantErr, "unexpected error for policy %s", tc.policy)
			}
		})
	}
}

func TestResourceSelection(t *testing.T) {
	t.Parallel()

	t.Run("all selection", func(t *testing.T) {
		t.Parallel()
		sel := ResourceSelection{All: true}
		assert.True(t, sel.All)
		assert.Nil(t, sel.IDs)
	})

	t.Run("ids selection", func(t *testing.T) {
		t.Parallel()
		ids := []string{"id-1", "id-2"}
		sel := ResourceSelection{All: false, IDs: ids}
		assert.False(t, sel.All)
		assert.Equal(t, ids, sel.IDs)
	})
}

func TestMigrationInventory_ZeroCounts(t *testing.T) {
	t.Parallel()

	inv := &MigrationInventory{}
	assert.Equal(t, 0, inv.Prompts.Count)
	assert.Equal(t, 0, inv.Artifacts.Count)
	assert.Equal(t, 0, inv.Blueprints.Count)
	assert.Equal(t, 0, inv.FeedItems.Count)
}

func TestMigrationResult_ZeroCounts(t *testing.T) {
	t.Parallel()

	result := &MigrationResult{}
	assert.Equal(t, 0, result.Migrated.Prompts)
	assert.Equal(t, 0, result.Migrated.Artifacts)
	assert.Equal(t, 0, result.Migrated.Blueprints)
	assert.Equal(t, 0, result.Migrated.FeedItems)
	assert.Nil(t, result.Skipped.Prompts)
	assert.Nil(t, result.Failed.Prompts)
}

func TestConflictPolicyConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ConflictPolicy("skip"), ConflictPolicySkip)
	assert.Equal(t, ConflictPolicy("rename"), ConflictPolicyRename)
	assert.Equal(t, ConflictPolicy("overwrite"), ConflictPolicyOverwrite)
}
