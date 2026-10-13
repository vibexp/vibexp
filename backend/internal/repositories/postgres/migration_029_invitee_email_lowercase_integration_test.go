//go:build integration

package postgres

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// "Before" is 027 rather than 028: 028 belongs to a sibling change of the same
// epic (user_credentials) and nothing here depends on it, so the test holds
// whether or not 028 is present.
const (
	migrationInviteeLowercaseBefore = 27
	migrationInviteeLowercaseAfter  = 29
)

// Up/down round trip for the invitee_email lower-casing migration (#1434) on
// its OWN scratch database: it migrates down, which would corrupt the shared
// integrationDB.
func TestMigration029_InviteeEmailLowercase(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()
	m := newMigrator(t, db)

	require.NoError(t, m.Migrate(migrationInviteeLowercaseBefore), "migrate to the version before")
	owner, team := uuid.New().String(), uuid.New().String()
	_, err := db.Exec("INSERT INTO users (id, email, name) VALUES ($1, 'owner@example.com', 'Owner')", owner)
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO teams (id, owner_id, name, slug) VALUES ($1, $2, 'T', 't')", team, owner)
	require.NoError(t, err)
	seed := func(email, status string) {
		t.Helper()
		_, seedErr := db.Exec(
			`INSERT INTO team_invitations (team_id, inviter_id, invitee_email, role, token, status, expires_at)
			 VALUES ($1, $2, $3, 'member', $4, $5, now() + interval '1 day')`,
			team, owner, email, uuid.New().String(), status)
		require.NoError(t, seedErr)
	}
	// Two people's worth of mixed casing, including the same address typed twice
	// in different casing: no unique constraint involves invitee_email, so both
	// rows must survive.
	seed("Bob@Example.com", "pending")
	seed("BOB@example.com", "revoked")
	seed("carol@example.com", "pending")

	require.NoError(t, m.Migrate(migrationInviteeLowercaseAfter), "migrate up")

	assert.Equal(t, 3, countRows(t, db, "SELECT count(*) FROM team_invitations"), "no row is lost or merged")
	assert.Equal(t, 0, countRows(t, db,
		"SELECT count(*) FROM team_invitations WHERE invitee_email <> lower(invitee_email)"))
	assert.Equal(t, 2, countRows(t, db, "SELECT count(*) FROM team_invitations WHERE invitee_email = 'bob@example.com'"))
	_, err = db.Exec(
		`INSERT INTO team_invitations (team_id, inviter_id, invitee_email, role, token, status, expires_at)
		 VALUES ($1, $2, 'Mixed@Example.com', 'member', $3, 'pending', now() + interval '1 day')`,
		team, owner, uuid.New().String())
	requirePQCode(t, err, pqCheckViolation)
	_, err = db.Exec("UPDATE team_invitations SET invitee_email = 'CAROL@example.com' WHERE invitee_email = 'carol@example.com'")
	requirePQCode(t, err, pqCheckViolation)

	require.NoError(t, m.Migrate(migrationInviteeLowercaseBefore), "migrate down over populated rows")
	assert.Equal(t, 3, countRows(t, db, "SELECT count(*) FROM team_invitations"))
	seed("Mixed@Example.com", "pending") // the CHECK is gone
	assert.Equal(t, 2, countRows(t, db, "SELECT count(*) FROM team_invitations WHERE invitee_email = 'bob@example.com'"),
		"addresses stay lower case: the original casing was not kept")

	require.NoError(t, m.Migrate(migrationInviteeLowercaseAfter), "and up again, over a mixed-case row")
	assert.Equal(t, 0, countRows(t, db,
		"SELECT count(*) FROM team_invitations WHERE invitee_email <> lower(invitee_email)"))
}
