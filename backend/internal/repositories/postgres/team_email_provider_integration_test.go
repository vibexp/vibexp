//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level suite for TeamEmailProviderRepository against real Postgres
// (#501). The sqlmock suite pins the statements; this one pins what the DATABASE
// does with them — that the ON CONFLICT upsert really cannot produce a second
// row for a team, that jsonb settings round-trip, that ON DELETE CASCADE from
// teams applies, and that RecordSendResult touches only the health columns.
// Never asserts SQL text.

// resetTeamEmailProviderTables clears this suite's tables. The shared
// resetIntegrationTables does not name teams, and team_email_providers hangs off
// teams rather than users, so this suite truncates its own chain.
func resetTeamEmailProviderTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, teams, team_email_providers CASCADE")
	require.NoError(t, err)
}

func integrationTeamEmailProvider(teamID string) *models.TeamEmailProvider {
	fromName := "Acme Team"
	replyTo := "reply@acme.test"
	return &models.TeamEmailProvider{
		TeamID:          teamID,
		ProviderType:    "mailgun",
		Settings:        json.RawMessage(`{"domain": "mg.acme.test", "base_url": "https://api.eu.mailgun.net/v3"}`),
		SecretEncrypted: "base64-ciphertext",
		FromAddress:     "hello@acme.test",
		FromName:        &fromName,
		ReplyTo:         &replyTo,
	}
}

func TestIntegrationTeamEmailProvider_GetByTeamID_NoRow(t *testing.T) {
	resetTeamEmailProviderTables(t)
	repo := NewTeamEmailProviderRepository(integrationDB)

	got, err := repo.GetByTeamID(context.Background(), uuid.New().String())

	assert.Nil(t, got)
	assert.ErrorIs(t, err, repositories.ErrTeamEmailProviderNotFound)
}

func TestIntegrationTeamEmailProvider_Upsert_InsertRoundTrip(t *testing.T) {
	resetTeamEmailProviderTables(t)
	repo := NewTeamEmailProviderRepository(integrationDB)
	teamID := insertTestTeam(t, insertTestUser(t))

	provider := integrationTeamEmailProvider(teamID)
	require.NoError(t, repo.Upsert(context.Background(), provider))
	assert.NotEmpty(t, provider.ID, "the generated id must come back on the struct")
	assert.Equal(t, int64(1), provider.Version)
	assert.False(t, provider.CreatedAt.IsZero())

	got, err := repo.GetByTeamID(context.Background(), teamID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, teamID, got.TeamID)
	assert.Equal(t, "mailgun", got.ProviderType)
	assert.Equal(t, "base64-ciphertext", got.SecretEncrypted)
	assert.Equal(t, "hello@acme.test", got.FromAddress)
	require.NotNil(t, got.FromName)
	assert.Equal(t, "Acme Team", *got.FromName)
	require.NotNil(t, got.ReplyTo)
	assert.Equal(t, "reply@acme.test", *got.ReplyTo)
	// jsonb is normalised by Postgres, so compare semantically.
	assert.JSONEq(t,
		`{"domain": "mg.acme.test", "base_url": "https://api.eu.mailgun.net/v3"}`,
		string(got.Settings))
	// A fresh provider has no delivery history yet.
	assert.Nil(t, got.LastSuccessAt)
	assert.Nil(t, got.LastError)
	assert.Nil(t, got.LastErrorAt)
	assert.True(t, got.IsHealthy(), "a provider that has not sent yet is not failing")
}

// The epic's core invariant: one provider per team, enforced by the schema. A
// second Upsert must update in place rather than add a row — this is what the
// sqlmock suite cannot prove.
func TestIntegrationTeamEmailProvider_Upsert_SecondWriteUpdatesInPlace(t *testing.T) {
	resetTeamEmailProviderTables(t)
	repo := NewTeamEmailProviderRepository(integrationDB)
	teamID := insertTestTeam(t, insertTestUser(t))
	ctx := context.Background()

	first := integrationTeamEmailProvider(teamID)
	require.NoError(t, repo.Upsert(ctx, first))

	second := integrationTeamEmailProvider(teamID)
	second.ProviderType = "postmark"
	second.Settings = json.RawMessage(`{"message_stream": "broadcast"}`)
	second.SecretEncrypted = "rotated-ciphertext"
	second.FromAddress = "noreply@acme.test"
	require.NoError(t, repo.Upsert(ctx, second))

	assert.Equal(t, first.ID, second.ID, "the upsert must reuse the team's existing row")
	assert.Equal(t, int64(2), second.Version, "an in-place update bumps version")

	var rows int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM team_email_providers WHERE team_id = $1", teamID).Scan(&rows))
	assert.Equal(t, 1, rows, "a team can never have two email providers")

	got, err := repo.GetByTeamID(ctx, teamID)
	require.NoError(t, err)
	assert.Equal(t, "postmark", got.ProviderType)
	assert.Equal(t, "rotated-ciphertext", got.SecretEncrypted)
	assert.Equal(t, "noreply@acme.test", got.FromAddress)
	assert.JSONEq(t, `{"message_stream": "broadcast"}`, string(got.Settings))
	assert.Equal(t, first.CreatedAt.UTC(), got.CreatedAt.UTC(), "created_at survives an update")
}

// Reconfiguring a provider must not erase the delivery history that explains
// why it was reconfigured.
func TestIntegrationTeamEmailProvider_Upsert_PreservesDeliveryHealth(t *testing.T) {
	resetTeamEmailProviderTables(t)
	repo := NewTeamEmailProviderRepository(integrationDB)
	teamID := insertTestTeam(t, insertTestUser(t))
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, integrationTeamEmailProvider(teamID)))
	failedAt := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.RecordSendResult(ctx, teamID, errors.New("550 relay denied"), failedAt))

	rotated := integrationTeamEmailProvider(teamID)
	rotated.SecretEncrypted = "rotated-ciphertext"
	require.NoError(t, repo.Upsert(ctx, rotated))

	got, err := repo.GetByTeamID(ctx, teamID)
	require.NoError(t, err)
	require.NotNil(t, got.LastError)
	assert.Equal(t, "550 relay denied", *got.LastError)
	require.NotNil(t, got.LastErrorAt)
}

func TestIntegrationTeamEmailProvider_RecordSendResult(t *testing.T) {
	resetTeamEmailProviderTables(t)
	repo := NewTeamEmailProviderRepository(integrationDB)
	teamID := insertTestTeam(t, insertTestUser(t))
	ctx := context.Background()

	provider := integrationTeamEmailProvider(teamID)
	require.NoError(t, repo.Upsert(ctx, provider))

	failedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	require.NoError(t, repo.RecordSendResult(ctx, teamID, errors.New("smtp: connection refused"), failedAt))

	failing, err := repo.GetByTeamID(ctx, teamID)
	require.NoError(t, err)
	require.NotNil(t, failing.LastError)
	assert.Equal(t, "smtp: connection refused", *failing.LastError)
	require.NotNil(t, failing.LastErrorAt)
	assert.Nil(t, failing.LastSuccessAt)
	assert.False(t, failing.IsHealthy())
	assert.Equal(t, provider.Version, failing.Version,
		"a delivery result is not a configuration edit, so version must not move")

	succeededAt := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.RecordSendResult(ctx, teamID, nil, succeededAt))

	recovered, err := repo.GetByTeamID(ctx, teamID)
	require.NoError(t, err)
	require.NotNil(t, recovered.LastSuccessAt)
	// The failure is deliberately still readable after recovery; health is
	// derived by comparing the timestamps.
	require.NotNil(t, recovered.LastError)
	assert.Equal(t, "smtp: connection refused", *recovered.LastError)
	assert.True(t, recovered.IsHealthy(), "a success after the failure means healthy")
}

func TestIntegrationTeamEmailProvider_RecordSendResult_UnknownTeamIsNoOp(t *testing.T) {
	resetTeamEmailProviderTables(t)
	repo := NewTeamEmailProviderRepository(integrationDB)

	err := repo.RecordSendResult(
		context.Background(), uuid.New().String(), nil, time.Now().UTC())

	assert.NoError(t, err, "a provider deleted mid-send must not fail the stamp")
}

func TestIntegrationTeamEmailProvider_Delete(t *testing.T) {
	resetTeamEmailProviderTables(t)
	repo := NewTeamEmailProviderRepository(integrationDB)
	teamID := insertTestTeam(t, insertTestUser(t))
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, integrationTeamEmailProvider(teamID)))
	require.NoError(t, repo.Delete(ctx, teamID))

	_, err := repo.GetByTeamID(ctx, teamID)
	assert.ErrorIs(t, err, repositories.ErrTeamEmailProviderNotFound)

	assert.ErrorIs(t, repo.Delete(ctx, teamID), repositories.ErrTeamEmailProviderNotFound,
		"deleting again reports nothing was there")
}

// Deleting a team must take its mail configuration — including the encrypted
// credential — with it.
func TestIntegrationTeamEmailProvider_TeamDeleteCascades(t *testing.T) {
	resetTeamEmailProviderTables(t)
	repo := NewTeamEmailProviderRepository(integrationDB)
	teamID := insertTestTeam(t, insertTestUser(t))
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, integrationTeamEmailProvider(teamID)))

	_, err := integrationDB.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", teamID)
	require.NoError(t, err)

	var rows int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM team_email_providers WHERE team_id = $1", teamID).Scan(&rows))
	assert.Equal(t, 0, rows, "the FK is ON DELETE CASCADE")
}

// Two teams must be able to configure providers independently — the unique
// constraint is per team, not global.
func TestIntegrationTeamEmailProvider_TeamsAreIndependent(t *testing.T) {
	resetTeamEmailProviderTables(t)
	repo := NewTeamEmailProviderRepository(integrationDB)
	ctx := context.Background()

	teamA := insertTestTeam(t, insertTestUser(t))
	teamB := insertTestTeam(t, insertTestUser(t))

	providerA := integrationTeamEmailProvider(teamA)
	providerB := integrationTeamEmailProvider(teamB)
	providerB.ProviderType = "sendgrid"
	providerB.FromAddress = "hello@beta.test"

	require.NoError(t, repo.Upsert(ctx, providerA))
	require.NoError(t, repo.Upsert(ctx, providerB))

	gotA, err := repo.GetByTeamID(ctx, teamA)
	require.NoError(t, err)
	gotB, err := repo.GetByTeamID(ctx, teamB)
	require.NoError(t, err)

	assert.Equal(t, "mailgun", gotA.ProviderType)
	assert.Equal(t, "sendgrid", gotB.ProviderType)
	assert.NotEqual(t, gotA.ID, gotB.ID)
}
