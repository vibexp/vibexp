//go:build integration

package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 019 (#1136) adds idx_rae_user_created ON resource_access_events
// (user_id, created_at) for the per-user admin access queries.

// TestMigration019_UpDown migrates a scratch database to 018, up to 019 and
// back down, on its OWN database so migrating down cannot disturb the shared
// integrationDB.
func TestMigration019_UpDown(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	hasIndex := func() bool {
		return countRows(t, db,
			"SELECT COUNT(*) FROM pg_indexes WHERE schemaname = 'public' AND indexname = 'idx_rae_user_created'") == 1
	}

	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(18), "migrate to 018")
	assert.False(t, hasIndex())

	require.NoError(t, m.Migrate(19), "migrate to 019")
	assert.True(t, hasIndex())
	var def string
	require.NoError(t, db.QueryRow(
		"SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_rae_user_created'").Scan(&def))
	assert.Contains(t, def, "(user_id, created_at)")

	require.NoError(t, m.Migrate(18), "migrate down to 018")
	assert.False(t, hasIndex())
}

// TestMigration019_PerUserSeriesUsesTheIndex EXPLAINs the production per-user
// statements. SET LOCAL inside a transaction (not SET against the pool, which
// can land on another connection), and every plan line is read — a bitmap scan
// names its index only in a child node. idx_rae_created_at alone must not
// satisfy the assertion.
func TestMigration019_PerUserSeriesUsesTheIndex(t *testing.T) {
	ctx := context.Background()
	f := seedAdminAccessFixture(t)
	from, to := accessDay.AddDate(0, 0, -1), accessDay.AddDate(0, 0, 2)

	for name, tc := range map[string]struct {
		query string
		args  []any
	}{
		"access by source": {fmt.Sprintf(adminUserAccessBySourceQueryFmt, adminTruncUnit("day")),
			[]any{f.user, from, to}},
		"top accessed": {adminUserTopAccessedQuery, []any{f.user, from, to, 10}},
	} {
		t.Run(name, func(t *testing.T) {
			tx, err := integrationDB.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()

			_, err = tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off")
			require.NoError(t, err)

			rows, err := tx.QueryContext(ctx, "EXPLAIN "+tc.query, tc.args...)
			require.NoError(t, err)
			defer func() { _ = rows.Close() }()

			var plan strings.Builder
			for rows.Next() {
				var line string
				require.NoError(t, rows.Scan(&line))
				plan.WriteString(line + "\n")
			}
			require.NoError(t, rows.Err())

			assert.Contains(t, plan.String(), "idx_rae_user_created",
				"the per-user %s query must use idx_rae_user_created; plan was:\n%s", name, plan.String())
		})
	}
}
