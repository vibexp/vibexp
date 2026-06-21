package postgres

import (
	"testing"

	"github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTeamAccessHelpers pins the exact SQL text and argument order produced by
// the team access-control helpers. The text is load-bearing: the sqlmock-based
// repository tests match it via regexp, and team_access_guardrail_test.go
// derives its canonical forms from the same predicate shape.
func TestTeamAccessHelpers(t *testing.T) {
	tests := []struct {
		name     string
		sqlizer  squirrel.Sqlizer
		wantSQL  string
		wantArgs []interface{}
	}{
		{
			name:    "read access with bound team ID",
			sqlizer: teamReadAccess("team-1", "user-1"),
			wantSQL: "(EXISTS (SELECT 1 FROM teams WHERE id = ? AND owner_id = ?) " +
				"OR EXISTS (SELECT 1 FROM team_members WHERE team_id = ? AND user_id = ?))",
			wantArgs: []interface{}{"team-1", "user-1", "team-1", "user-1"},
		},
		{
			name:    "row-correlated read access on a.team_id",
			sqlizer: teamRowReadAccess("a.team_id", "user-1"),
			wantSQL: "(EXISTS (SELECT 1 FROM teams t WHERE t.id = a.team_id AND t.owner_id = ?) " +
				"OR EXISTS (SELECT 1 FROM team_members tm WHERE tm.team_id = a.team_id AND tm.user_id = ?))",
			wantArgs: []interface{}{"user-1", "user-1"},
		},
		{
			name:    "row-correlated read access on mem.team_id",
			sqlizer: teamRowReadAccess("mem.team_id", "user-2"),
			wantSQL: "(EXISTS (SELECT 1 FROM teams t WHERE t.id = mem.team_id AND t.owner_id = ?) " +
				"OR EXISTS (SELECT 1 FROM team_members tm WHERE tm.team_id = mem.team_id AND tm.user_id = ?))",
			wantArgs: []interface{}{"user-2", "user-2"},
		},
		{
			name:    "write access with bound team ID",
			sqlizer: teamWriteAccess("team-1", "user-1"),
			wantSQL: "(EXISTS (SELECT 1 FROM teams WHERE id = ? AND owner_id = ?) " +
				"OR EXISTS (SELECT 1 FROM team_members " +
				"WHERE team_id = ? AND user_id = ? AND role IN ('owner', 'admin')))",
			wantArgs: []interface{}{"team-1", "user-1", "team-1", "user-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSQL, gotArgs, err := tt.sqlizer.ToSql()
			require.NoError(t, err)
			assert.Equal(t, tt.wantSQL, gotSQL)
			assert.Equal(t, tt.wantArgs, gotArgs)
		})
	}
}
