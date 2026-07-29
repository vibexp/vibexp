//go:build integration

package postgres

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigration011_RemoveQuotaSubscription verifies the remove-quota-subscription
// block of migration 011_consolidated (epic #646, issue #653; originally
// migration 016) against real Postgres, on its OWN scratch database.
//
// It deliberately avoids the shared `integrationDB`: TestMain migrates that one
// straight to head, whereas this test must observe the pre-migration (v0.8.0,
// version 010) state, seed rows the migration has to drop, and only then apply
// 011. Migrating the shared database mid-run would corrupt every other suite in
// the binary — and, because it is shared across worktrees, other checkouts too.
// Same reasoning and helpers as TestMigration011_RemoveAIToolHooks (#614).
//
// The property that matters most is the one a schema-only assertion misses:
// **a user row must still load after the columns are dropped.** Asserting the
// columns are absent proves the DDL ran; selecting a user proves the table is
// still usable, which is what every authenticated request depends on.
func TestMigration011_RemoveQuotaSubscription(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)

	// 1. Bring the schema to the version immediately BEFORE this migration.
	require.NoError(t, m.Migrate(10), "migrate to 010")

	// 2. Seed the pre-migration state: a team subscription, and users carrying
	//    non-default billing values (the defaults are 'basic', so seeding
	//    something else proves the columns really held data).
	userID, teamID := seedQuotaSubscriptionFixtures(t, db)

	// Pre-assert the fixtures are in the pre-migration shape. Without this a
	// silently failed seed would make every post-migration assertion pass
	// vacuously — "the column is gone" is trivially true if it never had data.
	require.True(t, tableExists(t, db, "team_subscriptions"),
		"fixture: team_subscriptions must exist before migrating")
	require.Equal(t, 1, countRows(t, db, `SELECT count(*) FROM team_subscriptions`),
		"fixture: a subscription row must exist before migrating")
	require.Equal(t, 1, countRows(t, db,
		`SELECT count(*) FROM users WHERE id = $1 AND subscription_plan = 'professional'
		   AND subscription_status = 'active' AND stripe_customer_id = 'cus_fixture_123'`, userID),
		"fixture: the user must hold non-default billing values before migrating")
	for _, col := range removedUserColumns() {
		require.True(t, columnExists(t, db, "users", col), "fixture: users.%s must exist before migrating", col)
	}
	require.True(t, indexExists(t, db, "idx_users_stripe_customer_id"))
	require.True(t, indexExists(t, db, "idx_users_subscription_canceled_at"))

	// 3. Apply migration 011.
	require.NoError(t, m.Migrate(11), "migrate to 011")

	t.Run("team_subscriptions and its indexes are gone", func(t *testing.T) {
		assert.False(t, tableExists(t, db, "team_subscriptions"))
		assert.False(t, indexExists(t, db, "idx_team_subscriptions_stripe_customer_id"))
		assert.False(t, indexExists(t, db, "idx_team_subscriptions_team_id"))
	})

	t.Run("the five billing columns and their indexes are gone", func(t *testing.T) {
		for _, col := range removedUserColumns() {
			assert.False(t, columnExists(t, db, "users", col), "users.%s must be dropped", col)
		}
		assert.False(t, indexExists(t, db, "idx_users_stripe_customer_id"))
		assert.False(t, indexExists(t, db, "idx_users_subscription_canceled_at"))
	})

	// The point of the whole slice: users must still be readable. A dropped
	// column that took the table with it would pass every assertion above.
	t.Run("the user row still loads and keeps its surviving fields", func(t *testing.T) {
		var email, name, status string
		var version int64
		require.NoError(t, db.QueryRow(
			`SELECT email, name, status, version FROM users WHERE id = $1`, userID,
		).Scan(&email, &name, &status, &version), "a user must still be selectable after the drop")
		assert.Equal(t, "fixture@migration016.test", email)
		assert.Equal(t, "Migration Fixture User", name)
		assert.Equal(t, "active", status)
		assert.Positive(t, version)
	})

	// The dropped FK was ON DELETE RESTRICT ("prevents team deletion when
	// subscriptions exist"). With the table gone that blocker cannot fire.
	t.Run("the team is deletable now the RESTRICT foreign key is gone", func(t *testing.T) {
		_, err := db.Exec(`DELETE FROM teams WHERE id = $1`, teamID)
		assert.NoError(t, err, "team deletion must not be blocked by a subscription FK")
	})

	// 4. Cycle down → up. Down restores structure only, which is what it claims.
	t.Run("down restores the structure and up re-applies cleanly", func(t *testing.T) {
		require.NoError(t, m.Migrate(10), "migrate down to 010")

		assert.True(t, tableExists(t, db, "team_subscriptions"), "down must recreate the table")
		for _, col := range removedUserColumns() {
			assert.True(t, columnExists(t, db, "users", col), "down must re-add users.%s", col)
		}
		assert.True(t, indexExists(t, db, "idx_users_stripe_customer_id"))
		assert.True(t, indexExists(t, db, "idx_users_subscription_canceled_at"))
		assert.True(t, indexExists(t, db, "idx_team_subscriptions_team_id"))

		// The original defaults must come back, not bare nullable columns.
		assert.Equal(t, "'basic'::character varying", columnDefault(t, db, "users", "subscription_status"))
		assert.Equal(t, "'basic'::character varying", columnDefault(t, db, "users", "subscription_plan"))

		// Structure only: the rows are not recoverable, as the file states.
		assert.Equal(t, 0, countRows(t, db, `SELECT count(*) FROM team_subscriptions`),
			"down restores structure only — the dropped rows must not reappear")

		require.NoError(t, m.Migrate(11), "re-apply 011 after rolling back")
		assert.False(t, tableExists(t, db, "team_subscriptions"))
	})
}

// removedUserColumns is the exact set the quota-subscription block drops from users.
func removedUserColumns() []string {
	return []string{
		"stripe_customer_id",
		"subscription_status",
		"trial_ends_at",
		"subscription_plan",
		"subscription_canceled_at",
	}
}

// seedQuotaSubscriptionFixtures inserts a user holding non-default billing values
// and a team with a subscription row. Returns the user and team ids.
func seedQuotaSubscriptionFixtures(t *testing.T, db *sql.DB) (userID, teamID string) {
	t.Helper()

	userID = uuid.New().String()
	_, err := db.Exec(
		`INSERT INTO users (id, email, name, stripe_customer_id, subscription_status, subscription_plan)
		 VALUES ($1, $2, $3, 'cus_fixture_123', 'active', 'professional')`,
		userID, "fixture@migration016.test", "Migration Fixture User")
	require.NoError(t, err, "seed user")

	teamID = uuid.New().String()
	_, err = db.Exec(
		`INSERT INTO teams (id, name, slug, owner_id) VALUES ($1, $2, $3, $4)`,
		teamID, "Migration Fixture Team", "migration-016-fixture-"+uuid.New().String()[:8], userID)
	require.NoError(t, err, "seed team")

	_, err = db.Exec(
		`INSERT INTO team_subscriptions (
		     team_id, stripe_subscription_id, stripe_customer_id, tier, seat_count,
		     status, billing_interval, current_period_start, current_period_end)
		 VALUES ($1, $2, 'cus_fixture_123', 'professional', 5, 'active', 'month', NOW(), NOW() + interval '30 days')`,
		teamID, "sub_fixture_"+uuid.New().String()[:8])
	require.NoError(t, err, "seed team subscription")

	return userID, teamID
}

func columnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRow(
		`SELECT EXISTS (
		    SELECT 1 FROM information_schema.columns
		     WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
		 )`, table, column).Scan(&exists))
	return exists
}

func indexExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRow(
		`SELECT EXISTS (
		    SELECT 1 FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1
		 )`, name).Scan(&exists))
	return exists
}

func columnDefault(t *testing.T, db *sql.DB, table, column string) string {
	t.Helper()
	var def sql.NullString
	require.NoError(t, db.QueryRow(
		`SELECT column_default FROM information_schema.columns
		  WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2`,
		table, column).Scan(&def))
	return def.String
}
