//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level suite for the auth settings repositories (#1231) against real
// Postgres: provider CRUD and InsertIfAbsent, the allowlist singleton, admin
// grants, the shared version bump on every provider/allowlist write (and not on
// a no-op or a rolled-back write), and the audit entry each write appends. The
// tables are global to the shared test database, so no test runs in parallel.

// resetInstanceAuthSettings empties the auth settings tables and the audit log
// (the users TRUNCATE cascades to every table with a user FK). The shared
// version row is never reset: tests assert deltas.
func resetInstanceAuthSettings(t *testing.T) {
	t.Helper()
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, "instance_auth_providers")
	resetInstanceSettingsTable(t, "instance_auth_allowlist")
	resetInstanceSettingsTable(t, "instance_admins")
}

func authSettingsVersion(t *testing.T) int64 {
	t.Helper()
	v, err := NewInstanceAuthSettingsVersionRepository(integrationDB).Get(context.Background())
	require.NoError(t, err)
	return v
}

func authAuditEntries(t *testing.T, setting string) []*models.InstanceSettingsAuditEntry {
	t.Helper()
	entries, _, err := NewInstanceSettingsAuditRepository(integrationDB).List(context.Background(), setting, 100, nil)
	require.NoError(t, err)
	return entries
}

func decodeAuditDoc(t *testing.T, doc json.RawMessage) map[string]any {
	t.Helper()
	if doc == nil {
		return nil
	}
	var m map[string]any
	require.NoError(t, json.Unmarshal(doc, &m))
	return m
}

func oidcProviderFixture(slug string, sortOrder int) *models.InstanceAuthProvider {
	issuer := "https://sso.example.com/" + slug
	return &models.InstanceAuthProvider{
		Type: models.InstanceAuthProviderOIDC, Slug: slug, DisplayName: "SSO " + slug,
		SortOrder: sortOrder, ClientID: "client-" + slug, IssuerURL: &issuer,
	}
}

func googleProviderFixture(ciphertext *string) *models.InstanceAuthProvider {
	return &models.InstanceAuthProvider{
		Type: models.InstanceAuthProviderGoogle, Slug: "google", DisplayName: "Google",
		Enabled: true, ClientID: "google-client", ClientSecretEncrypted: ciphertext,
	}
}

// providerCiphertext is a fake ciphertext; the repository never decrypts.
const providerCiphertext = "enc:v1:provider-ciphertext-sentinel"

func TestIntegrationInstanceAuthProviders_CRUD(t *testing.T) {
	resetInstanceAuthSettings(t)
	repo := NewInstanceAuthProviderRepository(integrationDB)
	ctx := context.Background()
	actor := insertTestUser(t)
	start := authSettingsVersion(t)

	cipher := providerCiphertext
	google := googleProviderFixture(&cipher)
	require.NoError(t, repo.Create(ctx, google, &actor, nil))
	require.NotEmpty(t, google.ID)
	assert.False(t, google.CreatedAt.IsZero())
	assert.Equal(t, start+1, authSettingsVersion(t), "a create bumps the shared version")

	require.NoError(t, repo.Create(ctx, oidcProviderFixture("zeta", 1), &actor, nil))
	require.NoError(t, repo.Create(ctx, oidcProviderFixture("alpha", 1), &actor, nil))
	assert.Equal(t, start+3, authSettingsVersion(t))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, []string{"google", "alpha", "zeta"},
		[]string{list[0].Slug, list[1].Slug, list[2].Slug}, "ordered by sort_order, then slug")

	got, err := repo.Get(ctx, google.ID)
	require.NoError(t, err)
	assert.Equal(t, models.InstanceAuthProviderGoogle, got.Type)
	assert.True(t, got.Enabled)
	require.NotNil(t, got.ClientSecretEncrypted)
	assert.Equal(t, providerCiphertext, *got.ClientSecretEncrypted, "the ciphertext is stored as handed over")
	assert.Nil(t, got.IssuerURL)
	require.NotNil(t, got.UpdatedBy)
	assert.Equal(t, actor, *got.UpdatedBy)

	bySlug, err := repo.GetBySlug(ctx, "alpha")
	require.NoError(t, err)
	assert.Equal(t, "client-alpha", bySlug.ClientID)
	require.NotNil(t, bySlug.IssuerURL)
	assert.Equal(t, "https://sso.example.com/alpha", *bySlug.IssuerURL)

	// Update: slug and type are immutable, whatever the struct says.
	update := *got
	update.Slug, update.Type = "renamed", models.InstanceAuthProviderGitHub
	update.DisplayName, update.Enabled, update.SortOrder, update.ClientID = "Google Workspace", false, 7, "new-client"
	update.ClientSecretEncrypted = nil
	require.NoError(t, repo.Update(ctx, &update, nil, nil))
	assert.Equal(t, "google", update.Slug, "the struct reflects the stored slug")
	assert.Equal(t, models.InstanceAuthProviderGoogle, update.Type)
	assert.Equal(t, start+4, authSettingsVersion(t), "an update bumps the shared version")

	got, err = repo.Get(ctx, google.ID)
	require.NoError(t, err)
	assert.Equal(t, "google", got.Slug)
	assert.Equal(t, "Google Workspace", got.DisplayName)
	assert.False(t, got.Enabled)
	assert.Equal(t, 7, got.SortOrder)
	assert.Equal(t, "new-client", got.ClientID)
	assert.Nil(t, got.ClientSecretEncrypted, "the update wrote the (cleared) secret")
	assert.Nil(t, got.UpdatedBy)
	assert.True(t, got.UpdatedAt.After(got.CreatedAt) || got.UpdatedAt.Equal(got.CreatedAt))

	require.NoError(t, repo.Delete(ctx, bySlug.ID, &actor, nil))
	assert.Equal(t, start+5, authSettingsVersion(t), "a delete bumps the shared version")
	_, err = repo.Get(ctx, bySlug.ID)
	assert.ErrorIs(t, err, repositories.ErrInstanceAuthProviderNotFound)
	_, err = repo.GetBySlug(ctx, "alpha")
	assert.ErrorIs(t, err, repositories.ErrInstanceAuthProviderNotFound)

	missing := oidcProviderFixture("ghost", 0)
	missing.ID = uuid.New().String()
	assert.ErrorIs(t, repo.Update(ctx, missing, &actor, nil), repositories.ErrInstanceAuthProviderNotFound)
	assert.ErrorIs(t, repo.Delete(ctx, missing.ID, &actor, nil), repositories.ErrInstanceAuthProviderNotFound)
	assert.Equal(t, start+5, authSettingsVersion(t), "a missing target writes nothing")
	assert.Len(t, authAuditEntries(t, models.InstanceSettingAuthProviders), 5, "one audit entry per write")
}

func TestIntegrationInstanceAuthProviders_List_Empty(t *testing.T) {
	resetInstanceAuthSettings(t)

	list, err := NewInstanceAuthProviderRepository(integrationDB).List(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, list)
	assert.Empty(t, list)
}

// Rejected writes roll back: no row, no audit entry, no version bump.
func TestIntegrationInstanceAuthProviders_RejectedWritesRollBack(t *testing.T) {
	resetInstanceAuthSettings(t)
	repo := NewInstanceAuthProviderRepository(integrationDB)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, googleProviderFixture(nil), nil, nil))
	stored := oidcProviderFixture("corp", 0)
	require.NoError(t, repo.Create(ctx, stored, nil, nil))
	start := authSettingsVersion(t)

	secondGoogle := googleProviderFixture(nil)
	secondGoogle.Slug = "google-2"
	assert.ErrorIs(t, repo.Create(ctx, secondGoogle, nil, nil), repositories.ErrInstanceAuthProviderConflict)
	assert.ErrorIs(t, repo.Create(ctx, oidcProviderFixture("corp", 0), nil, nil),
		repositories.ErrInstanceAuthProviderConflict, "a duplicate slug")

	noIssuer := oidcProviderFixture("no-issuer", 0)
	noIssuer.IssuerURL = nil
	assert.ErrorIs(t, repo.Create(ctx, noIssuer, nil, nil), repositories.ErrInstanceAuthProviderInvalid)

	badUpdate := *stored
	badUpdate.IssuerURL = nil
	assert.ErrorIs(t, repo.Update(ctx, &badUpdate, nil, nil), repositories.ErrInstanceAuthProviderInvalid)

	assert.Equal(t, start, authSettingsVersion(t), "a rolled-back write does not bump the version")
	assert.Len(t, authAuditEntries(t, models.InstanceSettingAuthProviders), 2, "nor append an audit entry")
	got, err := repo.Get(ctx, stored.ID)
	require.NoError(t, err)
	require.NotNil(t, got.IssuerURL, "the rejected update left the row untouched")
}

func TestIntegrationInstanceAuthProviders_ExpectedVersion(t *testing.T) {
	resetInstanceAuthSettings(t)
	repo := NewInstanceAuthProviderRepository(integrationDB)
	ctx := context.Background()

	current := authSettingsVersion(t)
	stale := current - 1
	p := oidcProviderFixture("corp", 0)
	assert.ErrorIs(t, repo.Create(ctx, p, nil, &stale), repositories.ErrInstanceSettingsVersionConflict)
	assert.Equal(t, current, authSettingsVersion(t))
	_, err := repo.GetBySlug(ctx, "corp")
	require.ErrorIs(t, err, repositories.ErrInstanceAuthProviderNotFound, "a conflict writes nothing")

	require.NoError(t, repo.Create(ctx, p, nil, &current))
	next := current + 1
	assert.Equal(t, next, authSettingsVersion(t))

	// An allowlist write moves the shared version too, so a provider write
	// holding the pre-allowlist version conflicts.
	require.NoError(t, NewInstanceAuthAllowlistRepository(integrationDB).UpsertAudited(ctx,
		&models.InstanceAuthAllowlist{Domains: []string{"example.com"}}, nil, nil))
	p.DisplayName = "Renamed"
	assert.ErrorIs(t, repo.Update(ctx, p, nil, &next), repositories.ErrInstanceSettingsVersionConflict)
	assert.ErrorIs(t, repo.Delete(ctx, p.ID, nil, &next), repositories.ErrInstanceSettingsVersionConflict)

	latest := authSettingsVersion(t)
	require.NoError(t, repo.Update(ctx, p, nil, &latest))
	latest++
	require.NoError(t, repo.Delete(ctx, p.ID, nil, &latest))
}

func TestIntegrationInstanceAuthProviders_InsertIfAbsent(t *testing.T) {
	resetInstanceAuthSettings(t)
	repo := NewInstanceAuthProviderRepository(integrationDB)
	ctx := context.Background()
	start := authSettingsVersion(t)

	cipher := providerCiphertext
	inserted, err := repo.InsertIfAbsent(ctx, googleProviderFixture(&cipher))
	require.NoError(t, err)
	assert.True(t, inserted)
	assert.Equal(t, start+1, authSettingsVersion(t), "an insert bumps the shared version")

	sameSlug := oidcProviderFixture("google", 0)
	inserted, err = repo.InsertIfAbsent(ctx, sameSlug)
	require.NoError(t, err)
	assert.False(t, inserted, "the slug is taken")

	otherGoogle := googleProviderFixture(nil)
	otherGoogle.Slug = "google-2"
	inserted, err = repo.InsertIfAbsent(ctx, otherGoogle)
	require.NoError(t, err)
	assert.False(t, inserted, "a google provider is already stored")
	assert.Equal(t, start+1, authSettingsVersion(t), "a no-op does not bump the version")

	entries := authAuditEntries(t, models.InstanceSettingAuthProviders)
	require.Len(t, entries, 1, "only the insert is audited")
	assert.Equal(t, models.InstanceSettingsAuditActionImport, entries[0].Action)
	assert.Nil(t, entries[0].ActorUserID)
	assert.Nil(t, entries[0].Before)

	stored, err := repo.GetBySlug(ctx, "google")
	require.NoError(t, err)
	assert.Nil(t, stored.UpdatedBy, "an import has no editor")

	bad := oidcProviderFixture("bad", 0)
	bad.IssuerURL = nil
	_, err = repo.InsertIfAbsent(ctx, bad)
	assert.ErrorIs(t, err, repositories.ErrInstanceAuthProviderInvalid)
}

// Replicas importing the same provider at once: the database decides, and
// exactly one of them inserts.
func TestIntegrationInstanceAuthProviders_ConcurrentInsertIfAbsent(t *testing.T) {
	resetInstanceAuthSettings(t)
	repo := NewInstanceAuthProviderRepository(integrationDB)
	ctx := context.Background()
	start := authSettingsVersion(t)

	const replicas = 4
	var wg sync.WaitGroup
	results := make(chan bool, replicas)
	errs := make(chan error, replicas)
	begin := make(chan struct{})
	for i := 0; i < replicas; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-begin
			ok, err := repo.InsertIfAbsent(ctx, oidcProviderFixture("corp", 0))
			results <- ok
			errs <- err
		}()
	}
	close(begin)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	var insertedCount int
	for ok := range results {
		if ok {
			insertedCount++
		}
	}
	assert.Equal(t, 1, insertedCount)
	assert.Equal(t, start+1, authSettingsVersion(t))
	assert.Len(t, authAuditEntries(t, models.InstanceSettingAuthProviders), 1)
}

// Concurrent creates with the same expected version: the version lock
// serializes them, so exactly one passes the compare-and-set.
func TestIntegrationInstanceAuthProviders_ConcurrentCreatesCompareAndSet(t *testing.T) {
	resetInstanceAuthSettings(t)
	repo := NewInstanceAuthProviderRepository(integrationDB)
	ctx := context.Background()
	expected := authSettingsVersion(t)

	const writers = 4
	errs := make(chan error, writers)
	begin := make(chan struct{})
	for i := 0; i < writers; i++ {
		p := oidcProviderFixture("corp-"+string(rune('a'+i)), i)
		go func() {
			<-begin
			errs <- repo.Create(ctx, p, nil, &expected)
		}()
	}
	close(begin)
	var ok, conflicts int
	for i := 0; i < writers; i++ {
		err := <-errs
		switch {
		case err == nil:
			ok++
		case assert.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict):
			conflicts++
		}
	}
	assert.Equal(t, 1, ok)
	assert.Equal(t, writers-1, conflicts)
	assert.Equal(t, expected+1, authSettingsVersion(t))
}

func TestIntegrationInstanceAuthProviders_AuditIsRedacted(t *testing.T) {
	resetInstanceAuthSettings(t)
	repo := NewInstanceAuthProviderRepository(integrationDB)
	ctx := context.Background()
	actor := insertTestUser(t)

	cipher := providerCiphertext
	p := googleProviderFixture(&cipher)
	require.NoError(t, repo.Create(ctx, p, &actor, nil))

	p.DisplayName = "Google (renamed)"
	require.NoError(t, repo.Update(ctx, p, &actor, nil)) // same ciphertext
	rotated := "enc:v1:rotated-ciphertext-sentinel"
	p.ClientSecretEncrypted = &rotated
	require.NoError(t, repo.Update(ctx, p, &actor, nil))
	require.NoError(t, repo.Delete(ctx, p.ID, &actor, nil))

	entries := authAuditEntries(t, models.InstanceSettingAuthProviders) // newest first
	require.Len(t, entries, 4)
	for _, e := range entries {
		assert.NotContains(t, string(e.Before), "sentinel", "no ciphertext in a before snapshot")
		assert.NotContains(t, string(e.After), "sentinel", "no ciphertext in an after snapshot")
		require.NotNil(t, e.ActorUserID)
		assert.Equal(t, actor, *e.ActorUserID)
	}

	deleted, rotate, rename, create := entries[0], entries[1], entries[2], entries[3]

	assert.Equal(t, models.InstanceSettingsAuditActionUpsert, create.Action)
	assert.Nil(t, create.Before)
	createAfter := decodeAuditDoc(t, create.After)
	assert.Equal(t, models.InstanceSettingsAuditSecretChanged, createAfter["client_secret"])
	assert.Equal(t, true, createAfter["has_client_secret"])
	assert.Equal(t, "google", createAfter["slug"])
	assert.NotContains(t, createAfter, "client_secret_encrypted")

	assert.Equal(t, models.InstanceSettingsAuditSecretUnchanged, decodeAuditDoc(t, rename.After)["client_secret"])
	assert.Equal(t, "Google", decodeAuditDoc(t, rename.Before)["display_name"])
	assert.Equal(t, "Google (renamed)", decodeAuditDoc(t, rename.After)["display_name"])
	assert.NotContains(t, decodeAuditDoc(t, rename.Before), "client_secret", "only the after snapshot carries the marker")

	assert.Equal(t, models.InstanceSettingsAuditSecretChanged, decodeAuditDoc(t, rotate.After)["client_secret"])

	assert.Equal(t, models.InstanceSettingsAuditActionDelete, deleted.Action)
	assert.Nil(t, deleted.After)
	assert.Equal(t, true, decodeAuditDoc(t, deleted.Before)["has_client_secret"])
}

func TestIntegrationInstanceAuthAllowlist_Lifecycle(t *testing.T) {
	resetInstanceAuthSettings(t)
	repo := NewInstanceAuthAllowlistRepository(integrationDB)
	ctx := context.Background()
	actor := insertTestUser(t)
	start := authSettingsVersion(t)

	_, err := repo.Get(ctx)
	assert.ErrorIs(t, err, repositories.ErrInstanceAuthAllowlistNotFound, "no row = open access")

	first := &models.InstanceAuthAllowlist{Domains: []string{"example.com"}} // Emails nil
	require.NoError(t, repo.UpsertAudited(ctx, first, &actor, nil))
	assert.Equal(t, int64(1), first.Version)
	assert.Equal(t, start+1, authSettingsVersion(t))

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"example.com"}, got.Domains)
	assert.NotNil(t, got.Emails, "a nil list is stored as an empty array")
	assert.Empty(t, got.Emails)
	require.NotNil(t, got.UpdatedBy)
	assert.Equal(t, actor, *got.UpdatedBy)

	stale := int64(0)
	second := &models.InstanceAuthAllowlist{Domains: []string{"corp.example.org"}, Emails: []string{"a@example.com"}}
	assert.ErrorIs(t, repo.UpsertAudited(ctx, second, &actor, &stale), repositories.ErrInstanceSettingsVersionConflict)
	assert.Equal(t, start+1, authSettingsVersion(t), "a conflict does not bump the version")

	expected := int64(1)
	require.NoError(t, repo.UpsertAudited(ctx, second, &actor, &expected))
	assert.Equal(t, int64(2), second.Version)
	assert.Equal(t, start+2, authSettingsVersion(t))
	got, err = repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"corp.example.org"}, got.Domains)
	assert.Equal(t, []string{"a@example.com"}, got.Emails)

	deleted, err := repo.DeleteAudited(ctx, &actor)
	require.NoError(t, err)
	assert.True(t, deleted)
	assert.Equal(t, start+3, authSettingsVersion(t))
	_, err = repo.Get(ctx)
	assert.ErrorIs(t, err, repositories.ErrInstanceAuthAllowlistNotFound)

	deleted, err = repo.DeleteAudited(ctx, &actor)
	require.NoError(t, err)
	assert.False(t, deleted)
	assert.Equal(t, start+3, authSettingsVersion(t), "deleting nothing does not bump the version")

	assert.ErrorIs(t, repo.UpsertAudited(ctx, second, &actor, &expected), repositories.ErrInstanceSettingsVersionConflict,
		"an expected version with no row stored conflicts")

	entries := authAuditEntries(t, models.InstanceSettingAuthAllowlist) // newest first
	require.Len(t, entries, 3)
	assert.Equal(t, models.InstanceSettingsAuditActionDelete, entries[0].Action)
	assert.Nil(t, entries[0].After)
	assert.Equal(t, []any{"corp.example.org"}, decodeAuditDoc(t, entries[0].Before)["domains"])
	assert.Equal(t, models.InstanceSettingsAuditActionUpsert, entries[1].Action)
	assert.Equal(t, []any{"example.com"}, decodeAuditDoc(t, entries[1].Before)["domains"])
	assert.Nil(t, entries[2].Before)
	require.NotNil(t, entries[2].ActorUserID)
	assert.Equal(t, actor, *entries[2].ActorUserID)
}

func TestIntegrationInstanceAuthAllowlist_InsertIfAbsent(t *testing.T) {
	resetInstanceAuthSettings(t)
	repo := NewInstanceAuthAllowlistRepository(integrationDB)
	ctx := context.Background()
	start := authSettingsVersion(t)

	inserted, err := repo.InsertIfAbsent(ctx, &models.InstanceAuthAllowlist{Emails: []string{"a@example.com"}})
	require.NoError(t, err)
	assert.True(t, inserted)
	assert.Equal(t, start+1, authSettingsVersion(t))

	inserted, err = repo.InsertIfAbsent(ctx, &models.InstanceAuthAllowlist{Domains: []string{"other.example.com"}})
	require.NoError(t, err)
	assert.False(t, inserted)
	assert.Equal(t, start+1, authSettingsVersion(t), "a no-op does not bump the version")

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"a@example.com"}, got.Emails, "the stored allowlist is left untouched")
	assert.Nil(t, got.UpdatedBy)

	entries := authAuditEntries(t, models.InstanceSettingAuthAllowlist)
	require.Len(t, entries, 1)
	assert.Equal(t, models.InstanceSettingsAuditActionImport, entries[0].Action)
	assert.Nil(t, entries[0].ActorUserID)
}

func TestIntegrationInstanceAdmins_GrantRevoke(t *testing.T) {
	resetInstanceAuthSettings(t)
	repo := NewInstanceAdminRepository(integrationDB)
	ctx := context.Background()
	grantor, alice, bob := insertTestUser(t), insertTestUser(t), insertTestUser(t)
	start := authSettingsVersion(t)

	granted, err := repo.IsGranted(ctx, alice)
	require.NoError(t, err)
	assert.False(t, granted)

	ok, err := repo.Grant(ctx, alice, &grantor)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = repo.Grant(ctx, alice, &grantor)
	require.NoError(t, err)
	assert.False(t, ok, "granting an existing admin is a no-op")
	ok, err = repo.Grant(ctx, bob, nil)
	require.NoError(t, err)
	assert.True(t, ok)

	granted, err = repo.IsGranted(ctx, alice)
	require.NoError(t, err)
	assert.True(t, granted)

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	byUser := map[string]*models.InstanceAdminGrant{list[0].UserID: list[0], list[1].UserID: list[1]}
	require.NotNil(t, byUser[alice].GrantedBy)
	assert.Equal(t, grantor, *byUser[alice].GrantedBy)
	assert.Nil(t, byUser[bob].GrantedBy)
	assert.False(t, byUser[alice].CreatedAt.IsZero())

	require.NoError(t, repo.Revoke(ctx, alice, &grantor))
	granted, err = repo.IsGranted(ctx, alice)
	require.NoError(t, err)
	assert.False(t, granted)
	assert.ErrorIs(t, repo.Revoke(ctx, alice, &grantor), repositories.ErrInstanceAdminNotFound)

	_, err = repo.Grant(ctx, uuid.New().String(), &grantor)
	assert.ErrorIs(t, err, repositories.ErrUserNotFound)

	assert.Equal(t, start, authSettingsVersion(t), "admin grants do not bump the auth settings version")

	entries := authAuditEntries(t, models.InstanceSettingInstanceAdmins) // newest first
	require.Len(t, entries, 3, "two grants and one revoke; the no-op and failures are not audited")
	assert.Equal(t, models.InstanceSettingsAuditActionDelete, entries[0].Action)
	assert.Equal(t, alice, decodeAuditDoc(t, entries[0].Before)["user_id"])
	assert.Nil(t, entries[0].After)
	assert.Equal(t, models.InstanceSettingsAuditActionUpsert, entries[2].Action)
	assert.Nil(t, entries[2].Before)
	assert.Equal(t, alice, decodeAuditDoc(t, entries[2].After)["user_id"])
	require.NotNil(t, entries[2].ActorUserID)
	assert.Equal(t, grantor, *entries[2].ActorUserID)
	assert.Nil(t, entries[1].ActorUserID, "a grant with no grantor has no actor")
}

func TestIntegrationInstanceAdmins_List_Empty(t *testing.T) {
	resetInstanceAuthSettings(t)

	list, err := NewInstanceAdminRepository(integrationDB).List(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, list)
	assert.Empty(t, list)
}
