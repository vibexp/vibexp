//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level suite for InstanceEmailProviderRepository against real
// Postgres (#1186). The table is a singleton that is global to the shared test
// database, so every test starts from and leaves behind an empty table, and none
// runs in parallel.

func resetInstanceEmailProvider(t *testing.T) {
	t.Helper()
	wipe := func() {
		_, err := integrationDB.ExecContext(context.Background(), "DELETE FROM instance_email_provider")
		require.NoError(t, err)
	}
	wipe()
	t.Cleanup(wipe)
}

func integrationInstanceEmailProvider(t *testing.T) *models.InstanceEmailProvider {
	t.Helper()
	secret := "base64-ciphertext"
	fromName := "Acme"
	replyTo := "reply@acme.test"
	contact := "contact@acme.test"
	privacy := "https://acme.test/privacy"
	updatedBy := insertTestUser(t)
	return &models.InstanceEmailProvider{
		ProviderType:            "smtp",
		Settings:                json.RawMessage(`{"host": "smtp.acme.test", "port": "587"}`),
		SecretEncrypted:         &secret,
		FromAddress:             "noreply@acme.test",
		FromName:                &fromName,
		ReplyTo:                 &replyTo,
		ContactRecipientAddress: &contact,
		PrivacyPolicyURL:        &privacy,
		UpdatedBy:               &updatedBy,
	}
}

func countInstanceEmailProviderRows(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT count(*) FROM instance_email_provider").Scan(&n))
	return n
}

func TestIntegrationInstanceEmailProvider_Get_NoRow(t *testing.T) {
	resetInstanceEmailProvider(t)
	repo := NewInstanceEmailProviderRepository(integrationDB)

	got, err := repo.Get(context.Background())

	assert.Nil(t, got)
	assert.ErrorIs(t, err, repositories.ErrInstanceEmailProviderNotFound)
}

func TestIntegrationInstanceEmailProvider_Upsert_InsertRoundTrip(t *testing.T) {
	resetInstanceEmailProvider(t)
	repo := NewInstanceEmailProviderRepository(integrationDB)
	ctx := context.Background()

	provider := integrationInstanceEmailProvider(t)
	require.NoError(t, repo.Upsert(ctx, provider))
	assert.Equal(t, int64(1), provider.Version)
	assert.False(t, provider.CreatedAt.IsZero(), "created_at must come back on the struct")

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, "smtp", got.ProviderType)
	assert.JSONEq(t, `{"host": "smtp.acme.test", "port": "587"}`, string(got.Settings))
	require.NotNil(t, got.SecretEncrypted)
	assert.Equal(t, "base64-ciphertext", *got.SecretEncrypted)
	assert.Equal(t, "noreply@acme.test", got.FromAddress)
	assert.Equal(t, provider.FromName, got.FromName)
	assert.Equal(t, provider.ReplyTo, got.ReplyTo)
	assert.Equal(t, provider.ContactRecipientAddress, got.ContactRecipientAddress)
	assert.Equal(t, provider.PrivacyPolicyURL, got.PrivacyPolicyURL)
	assert.Equal(t, provider.UpdatedBy, got.UpdatedBy)
	assert.Nil(t, got.LastSuccessAt)
	assert.Nil(t, got.LastErrorAt)
}

func TestIntegrationInstanceEmailProvider_Upsert_NilSecretAndSettings(t *testing.T) {
	resetInstanceEmailProvider(t)
	repo := NewInstanceEmailProviderRepository(integrationDB)
	ctx := context.Background()

	// An unauthenticated relay: no credential, and nothing but the From.
	require.NoError(t, repo.Upsert(ctx, &models.InstanceEmailProvider{
		ProviderType: "smtp", FromAddress: "noreply@acme.test",
	}))

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Nil(t, got.SecretEncrypted, "a nil secret is stored as NULL")
	assert.False(t, got.HasCredential())
	assert.JSONEq(t, `{}`, string(got.Settings))
	assert.Nil(t, got.UpdatedBy)
}

func TestIntegrationInstanceEmailProvider_Upsert_UpdatesInPlaceAndKeepsHealth(t *testing.T) {
	resetInstanceEmailProvider(t)
	repo := NewInstanceEmailProviderRepository(integrationDB)
	ctx := context.Background()

	first := integrationInstanceEmailProvider(t)
	require.NoError(t, repo.Upsert(ctx, first))
	failedAt := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	require.NoError(t, repo.RecordError(ctx, errors.New("smtp: 535 auth failed"), failedAt))

	second := &models.InstanceEmailProvider{
		ProviderType: "postmark",
		Settings:     json.RawMessage(`{"message_stream": "outbound"}`),
		FromAddress:  "mail@acme.test",
	}
	require.NoError(t, repo.Upsert(ctx, second))

	assert.Equal(t, 1, countInstanceEmailProviderRows(t), "an upsert can never produce a second row")
	assert.Equal(t, int64(2), second.Version)
	assert.True(t, second.CreatedAt.Equal(first.CreatedAt), "the update keeps the original created_at")

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, "postmark", got.ProviderType)
	assert.Equal(t, "mail@acme.test", got.FromAddress)
	assert.Nil(t, got.SecretEncrypted, "the replacement's nil secret replaces the stored one")
	assert.Nil(t, got.ContactRecipientAddress)
	require.NotNil(t, got.LastError, "reconfiguring must keep the delivery history")
	assert.Equal(t, "smtp: 535 auth failed", *got.LastError)
	require.NotNil(t, got.LastErrorAt)
	assert.True(t, got.LastErrorAt.Equal(failedAt))
}

func TestIntegrationInstanceEmailProvider_InsertIfAbsent(t *testing.T) {
	resetInstanceEmailProvider(t)
	repo := NewInstanceEmailProviderRepository(integrationDB)
	ctx := context.Background()

	provider := integrationInstanceEmailProvider(t)
	inserted, err := repo.InsertIfAbsent(ctx, provider)
	require.NoError(t, err)
	assert.True(t, inserted, "an empty table takes the row")
	assert.Equal(t, int64(1), provider.Version)

	other := &models.InstanceEmailProvider{ProviderType: "sendgrid", FromAddress: "other@acme.test"}
	inserted, err = repo.InsertIfAbsent(ctx, other)
	require.NoError(t, err)
	assert.False(t, inserted, "an existing row is not replaced")

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, "smtp", got.ProviderType, "the existing row is left untouched")
	assert.Equal(t, "noreply@acme.test", got.FromAddress)
	assert.Equal(t, int64(1), got.Version)
}

func TestIntegrationInstanceEmailProvider_InsertIfAbsent_ConcurrentExactlyOneWins(t *testing.T) {
	resetInstanceEmailProvider(t)
	repo := NewInstanceEmailProviderRepository(integrationDB)
	ctx := context.Background()

	const writers = 2
	var wg sync.WaitGroup
	results := make([]bool, writers)
	errs := make([]error, writers)
	start := make(chan struct{})
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = repo.InsertIfAbsent(ctx, &models.InstanceEmailProvider{
				ProviderType: "smtp", FromAddress: "replica@acme.test",
			})
		}()
	}
	close(start)
	wg.Wait()

	wins := 0
	for i := range writers {
		require.NoError(t, errs[i], "a losing replica gets inserted=false, not an error")
		if results[i] {
			wins++
		}
	}
	assert.Equal(t, 1, wins, "exactly one concurrent writer inserts")
	assert.Equal(t, 1, countInstanceEmailProviderRows(t))
}

func TestIntegrationInstanceEmailProvider_Delete(t *testing.T) {
	resetInstanceEmailProvider(t)
	repo := NewInstanceEmailProviderRepository(integrationDB)
	ctx := context.Background()

	assert.ErrorIs(t, repo.Delete(ctx), repositories.ErrInstanceEmailProviderNotFound,
		"nothing to delete")

	require.NoError(t, repo.Upsert(ctx, integrationInstanceEmailProvider(t)))
	require.NoError(t, repo.Delete(ctx))

	_, err := repo.Get(ctx)
	assert.ErrorIs(t, err, repositories.ErrInstanceEmailProviderNotFound, "the row is gone")
}

func TestIntegrationInstanceEmailProvider_RecordHealth(t *testing.T) {
	resetInstanceEmailProvider(t)
	repo := NewInstanceEmailProviderRepository(integrationDB)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, integrationInstanceEmailProvider(t)))
	before, err := repo.Get(ctx)
	require.NoError(t, err)

	failedAt := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	require.NoError(t, repo.RecordError(ctx, errors.New("dial tcp: refused"), failedAt))
	got, err := repo.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, got.LastError)
	assert.Equal(t, "dial tcp: refused", *got.LastError)
	assert.True(t, got.LastErrorAt.Equal(failedAt))
	assert.Nil(t, got.LastSuccessAt)
	assert.False(t, got.IsHealthy())

	recoveredAt := failedAt.Add(time.Hour)
	require.NoError(t, repo.RecordSuccess(ctx, recoveredAt))
	got, err = repo.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, got.LastSuccessAt)
	assert.True(t, got.LastSuccessAt.Equal(recoveredAt))
	require.NotNil(t, got.LastError, "a success keeps the last error for diagnosis")
	assert.True(t, got.IsHealthy())

	// Health stamping is not a configuration edit.
	assert.Equal(t, before.Version, got.Version, "version is not bumped")
	assert.True(t, got.UpdatedAt.Equal(before.UpdatedAt), "updated_at is not touched")
	assert.Equal(t, before.FromAddress, got.FromAddress)
	assert.Equal(t, before.SecretEncrypted, got.SecretEncrypted)
}

func TestIntegrationInstanceEmailProvider_RecordHealth_NoRowIsNoOp(t *testing.T) {
	resetInstanceEmailProvider(t)
	repo := NewInstanceEmailProviderRepository(integrationDB)
	ctx := context.Background()

	require.NoError(t, repo.RecordSuccess(ctx, time.Now()))
	require.NoError(t, repo.RecordError(ctx, errors.New("boom"), time.Now()))
	assert.Equal(t, 0, countInstanceEmailProviderRows(t), "stamping health never creates a row")
}

func TestIntegrationInstanceEmailProvider_UpdatedBySetNullOnUserDelete(t *testing.T) {
	resetInstanceEmailProvider(t)
	repo := NewInstanceEmailProviderRepository(integrationDB)
	ctx := context.Background()

	provider := integrationInstanceEmailProvider(t)
	require.NoError(t, repo.Upsert(ctx, provider))
	_, err := integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", *provider.UpdatedBy)
	require.NoError(t, err)

	got, err := repo.Get(ctx)
	require.NoError(t, err, "deleting the last editor must not delete the instance config")
	assert.Nil(t, got.UpdatedBy)
}
