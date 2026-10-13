//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/repositories"
)

// Real-Postgres coverage for revoking a user's API access (#1449): the one
// statement that sets the bearer-token cut-off and deletes the user's API keys
// and stored OAuth rows, scoped to that user and atomic.

// oauthSubjectTables are the four request tables that store the user id as
// their subject.
var oauthSubjectTables = []string{
	"oauth_access_tokens", "oauth_refresh_tokens", "oauth_authorization_codes", "oauth_pkce_sessions",
}

// seedAPIAccess gives userID one API key (with an integration permission), one
// row in each OAuth request table for clientID, and one pending login session.
func seedAPIAccess(t *testing.T, userID, clientID string) {
	t.Helper()
	ctx := context.Background()
	keyID := uuid.New().String()
	_, err := integrationDB.ExecContext(ctx,
		"INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix) VALUES ($1, $2, 'k', $3, 'vxp_')",
		keyID, userID, "hash-"+keyID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx,
		`INSERT INTO api_key_integration_permissions (api_key_id, integration_code)
		 SELECT $1, integration_code FROM api_key_integrations_catalog LIMIT 1`, keyID)
	require.NoError(t, err)
	for _, table := range oauthSubjectTables {
		_, err = integrationDB.ExecContext(ctx, // #nosec G202 -- test-only, fixed identifiers
			"INSERT INTO "+table+" (signature, request_id, client_id, subject) VALUES ($1, $2, $3, $4)",
			uuid.New().String(), uuid.New().String(), clientID, userID)
		require.NoError(t, err, table)
	}
	_, err = integrationDB.ExecContext(ctx,
		`INSERT INTO oauth_login_sessions (id, authorize_query, provider, idp_state, user_id, expires_at)
		 VALUES ($1, 'q', 'google', 's', $2, now() + interval '10 minutes')`, uuid.New().String(), userID)
	require.NoError(t, err)
}

// apiAccessRows counts everything seedAPIAccess creates for userID.
func apiAccessRows(t *testing.T, userID string) (keys, grants int) {
	t.Helper()
	keys = countRows(t, integrationDB.DB, "SELECT count(*) FROM api_keys WHERE user_id = $1", userID)
	for _, table := range oauthSubjectTables {
		grants += countRows(t, integrationDB.DB, // #nosec G202 -- test-only, fixed identifiers
			"SELECT count(*) FROM "+table+" WHERE subject = $1", userID)
	}
	grants += countRows(t, integrationDB.DB, "SELECT count(*) FROM oauth_login_sessions WHERE user_id = $1", userID)
	return keys, grants
}

func resetAPIAccessTables(t *testing.T) {
	t.Helper()
	resetIntegrationTables(t)
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE "+strings.Join(oauthSubjectTables, ", ")+", oauth_login_sessions") // #nosec G202 -- fixed identifiers
	require.NoError(t, err)
}

func TestIntegrationAPIAccess_RevokeAll(t *testing.T) {
	resetAPIAccessTables(t)
	ctx := context.Background()
	repo := NewAPIAccessRepository(integrationDB)
	users := NewUserRepository(integrationDB)
	target := insertTestUser(t)
	bystander := insertTestUser(t)
	const sharedClient = "client-shared"
	seedAPIAccess(t, target, sharedClient)
	seedAPIAccess(t, target, "client-other")
	seedAPIAccess(t, bystander, sharedClient)

	before, err := users.GetByID(ctx, target)
	require.NoError(t, err)
	require.Nil(t, before.APITokensValidAfter, "a user whose access was never revoked has no cut-off")

	revoked, err := repo.RevokeAll(ctx, target)

	require.NoError(t, err)
	assert.EqualValues(t, 2, revoked.APIKeys)
	assert.EqualValues(t, 10, revoked.OAuthGrants, "two rows in each of four request tables, plus two login sessions")
	assert.WithinDuration(t, time.Now(), revoked.TokensValidAfter, time.Minute)

	keys, grants := apiAccessRows(t, target)
	assert.Zero(t, keys, "every API key of the user is gone")
	assert.Zero(t, grants, "every stored OAuth row of the user is gone")
	assert.Equal(t, 1, countRows(t, integrationDB.DB, "SELECT count(*) FROM api_key_integration_permissions"),
		"the deleted keys' permissions went with them; the bystander's stays")

	after, err := users.GetByID(ctx, target)
	require.NoError(t, err)
	require.NotNil(t, after.APITokensValidAfter, "the cut-off is what ends tokens already issued")
	assert.True(t, revoked.TokensValidAfter.Equal(*after.APITokensValidAfter))
	assert.Equal(t, before.SessionEpoch, after.SessionEpoch, "cookie sessions are not this revocation's business")
	assert.Equal(t, before.Version, after.Version)

	t.Run("another user is untouched, same OAuth client included", func(t *testing.T) {
		keys, grants := apiAccessRows(t, bystander)
		assert.Equal(t, 1, keys)
		assert.Equal(t, 5, grants)
		other, lookupErr := users.GetByID(ctx, bystander)
		require.NoError(t, lookupErr)
		assert.Nil(t, other.APITokensValidAfter)
	})

	t.Run("calling it again succeeds and moves the cut-off forward", func(t *testing.T) {
		_, backdateErr := integrationDB.ExecContext(ctx,
			"UPDATE users SET api_tokens_valid_after = now() - interval '1 hour' WHERE id = $1", target)
		require.NoError(t, backdateErr)

		again, againErr := repo.RevokeAll(ctx, target)

		require.NoError(t, againErr, "nothing left to revoke is not an error")
		assert.Zero(t, again.APIKeys)
		assert.Zero(t, again.OAuthGrants)
		assert.WithinDuration(t, time.Now(), again.TokensValidAfter, time.Minute,
			"each call means everything up to now")
	})

	t.Run("a user with nothing to revoke", func(t *testing.T) {
		empty := insertTestUser(t)
		got, emptyErr := repo.RevokeAll(ctx, empty)
		require.NoError(t, emptyErr)
		assert.Zero(t, got.APIKeys)
		assert.Zero(t, got.OAuthGrants)
		assert.False(t, got.TokensValidAfter.IsZero())
	})

	t.Run("an unknown user is ErrUserNotFound and deletes nothing", func(t *testing.T) {
		got, missingErr := repo.RevokeAll(ctx, uuid.New().String())
		require.ErrorIs(t, missingErr, repositories.ErrUserNotFound)
		assert.Nil(t, got)
		keys, grants := apiAccessRows(t, bystander)
		assert.Equal(t, 1, keys)
		assert.Equal(t, 5, grants)
	})
}

// The delete-by-subject must not scan every grant on the instance. The planner
// picks a sequential scan on a tiny table whatever indexes exist, so sequential
// scans are disabled for the check: what is asserted is that the index CAN serve
// the predicate, which is what holds on a table of real size.
func TestIntegrationAPIAccess_SubjectDeletesUseTheIndexes(t *testing.T) {
	resetAPIAccessTables(t)
	ctx := context.Background()
	seedAPIAccess(t, insertTestUser(t), "client-a")

	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off")
	require.NoError(t, err)

	for _, table := range oauthSubjectTables {
		rows, queryErr := tx.QueryContext(ctx, // #nosec G202 -- test-only, fixed identifiers
			"EXPLAIN DELETE FROM "+table+" WHERE subject = $1", uuid.New().String())
		require.NoError(t, queryErr, table)
		var plan strings.Builder
		for rows.Next() {
			var line string
			require.NoError(t, rows.Scan(&line))
			plan.WriteString(line + "\n")
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		assert.Contains(t, plan.String(), "idx_"+table+"_subject", "plan for %s:\n%s", table, plan.String())
	}
}
