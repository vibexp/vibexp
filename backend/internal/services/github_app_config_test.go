package services

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

const (
	testAppConfigTeamID = "team-1"
	testAppConfigUserID = "user-1"
)

// testRSAPEM generates a real RSA key in PEM form. Validation and JWT minting
// both actually parse the key, so a fixture string would only prove that the
// error path works.
func testRSAPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
}

func newAppConfigService(
	t *testing.T, repo repositories.GitHubAppConfigRepository,
) *GitHubAppConfigService {
	t.Helper()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	return NewGitHubAppConfigService(repo, enc, permissiveProviderAuthz{}, nil, "https://vibexp.example")
}

func validCreateRequest(t *testing.T) models.CreateGitHubAppConfigRequest {
	t.Helper()
	return models.CreateGitHubAppConfigRequest{
		AppID:        "123456",
		AppSlug:      "my-app",
		ClientID:     "Iv1.abc",
		PrivateKey:   testRSAPEM(t),
		ClientSecret: "cs-secret",
	}
}

// storedConfig is what a repository read returns: ciphertext only.
func storedConfig(t *testing.T, s *GitHubAppConfigService) *models.GitHubAppConfig {
	t.Helper()
	pk, err := s.encrypt(testRSAPEM(t))
	require.NoError(t, err)
	cs, err := s.encrypt("cs-secret")
	require.NoError(t, err)
	ws, err := s.encrypt("ws-secret")
	require.NoError(t, err)
	return &models.GitHubAppConfig{
		ID: "cfg-1", TeamID: testAppConfigTeamID, AppID: "123456", AppSlug: "my-app",
		ClientID: "Iv1.abc", PrivateKeyEncrypted: pk, ClientSecretEncrypted: cs,
		WebhookSecretEncrypted: ws, WebhookToken: "tok-abc", Version: 1,
	}
}

func TestGitHubAppConfigService_Create(t *testing.T) {
	ctx := context.Background()

	t.Run("encrypts every secret and discloses the webhook secret once", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		req := validCreateRequest(t)

		var persisted *models.GitHubAppConfig
		repo.EXPECT().Create(ctx, mock.Anything).RunAndReturn(
			func(_ context.Context, c *models.GitHubAppConfig) error {
				persisted = c
				c.ID = "cfg-1"
				c.Version = 1
				return nil
			})

		created, err := svc.CreateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID, req)
		require.NoError(t, err)

		// What reaches the database must be ciphertext, not the plaintext we sent.
		require.NotNil(t, persisted)
		assert.NotEqual(t, req.PrivateKey, persisted.PrivateKeyEncrypted)
		assert.NotEqual(t, req.ClientSecret, persisted.ClientSecretEncrypted)
		assert.NotEmpty(t, persisted.WebhookSecretEncrypted)

		// ...and must decrypt back byte-identically.
		gotKey, err := svc.decrypt(persisted.PrivateKeyEncrypted)
		require.NoError(t, err)
		assert.Equal(t, req.PrivateKey, gotKey)
		gotSecret, err := svc.decrypt(persisted.ClientSecretEncrypted)
		require.NoError(t, err)
		assert.Equal(t, req.ClientSecret, gotSecret)

		// The one deliberate disclosure: the plaintext webhook secret, which the
		// admin must paste into GitHub and can never read back.
		assert.NotEmpty(t, created.WebhookSecret)
		roundTripped, err := svc.decrypt(persisted.WebhookSecretEncrypted)
		require.NoError(t, err)
		assert.Equal(t, created.WebhookSecret, roundTripped)

		assert.True(t, created.HasPrivateKey)
		assert.True(t, created.HasClientSecret)
		assert.True(t, created.HasWebhookSecret)
		assert.Equal(t,
			"https://vibexp.example/api/v1/webhooks/github/"+persisted.WebhookToken,
			created.WebhookURL)
	})

	// The token ends up in a URL path segment. Minting it URL-safe by
	// construction is what lets #481's route skip percent-decoding entirely.
	t.Run("mints a URL-safe unpadded webhook token", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)

		var token string
		repo.EXPECT().Create(ctx, mock.Anything).RunAndReturn(
			func(_ context.Context, c *models.GitHubAppConfig) error {
				token = c.WebhookToken
				return nil
			})

		_, err := svc.CreateAppConfig(
			ctx, testAppConfigTeamID, testAppConfigUserID, validCreateRequest(t))
		require.NoError(t, err)

		assert.Regexp(t, regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`), token)
		assert.NotContains(t, token, "=", "RawURLEncoding must not pad")
	})

	t.Run("accepts a base64-encoded PEM", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		repo.EXPECT().Create(ctx, mock.Anything).Return(nil)

		req := validCreateRequest(t)
		req.PrivateKey = base64.StdEncoding.EncodeToString([]byte(req.PrivateKey))

		_, err := svc.CreateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID, req)
		assert.NoError(t, err, "operators moving off config.yaml keep their base64 key")
	})

	// An unusable key must be a 400 at paste time, not a 500 at install time.
	t.Run("rejects an unparseable private key as validation", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)

		req := validCreateRequest(t)
		req.PrivateKey = "not a pem"

		_, err := svc.CreateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "private key")
	})

	// The retry exists so an (astronomically unlikely) token collision is a
	// transparent non-event rather than a spurious 500. Untested, it would be
	// indistinguishable from a broken one.
	t.Run("re-mints the token once on a collision", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)

		var tokens []string
		repo.EXPECT().Create(ctx, mock.Anything).RunAndReturn(
			func(_ context.Context, c *models.GitHubAppConfig) error {
				tokens = append(tokens, c.WebhookToken)
				if len(tokens) == 1 {
					return repositories.ErrGitHubAppWebhookTokenTaken
				}
				return nil
			}).Twice()

		_, err := svc.CreateAppConfig(
			ctx, testAppConfigTeamID, testAppConfigUserID, validCreateRequest(t))
		require.NoError(t, err)
		require.Len(t, tokens, 2)
		assert.NotEqual(t, tokens[0], tokens[1], "the retry must mint a NEW token, not resend the collided one")
	})

	t.Run("gives up after a persistent token collision", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		repo.EXPECT().Create(ctx, mock.Anything).
			Return(repositories.ErrGitHubAppWebhookTokenTaken).Twice()

		_, err := svc.CreateAppConfig(
			ctx, testAppConfigTeamID, testAppConfigUserID, validCreateRequest(t))
		require.Error(t, err, "a persistent collision must surface, not loop")
	})

	t.Run("maps a duplicate app id to the sentinel", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		repo.EXPECT().Create(ctx, mock.Anything).
			Return(repositories.ErrGitHubAppAlreadyRegistered)

		_, err := svc.CreateAppConfig(
			ctx, testAppConfigTeamID, testAppConfigUserID, validCreateRequest(t))
		assert.ErrorIs(t, err, ErrGitHubAppAlreadyRegistered)
	})

	t.Run("maps an existing team config to the sentinel", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		repo.EXPECT().Create(ctx, mock.Anything).
			Return(repositories.ErrGitHubAppConfigTeamTaken)

		_, err := svc.CreateAppConfig(
			ctx, testAppConfigTeamID, testAppConfigUserID, validCreateRequest(t))
		assert.ErrorIs(t, err, ErrGitHubAppConfigExists)
	})
}

func TestGitHubAppConfigService_Get(t *testing.T) {
	ctx := context.Background()

	t.Run("never exposes a secret", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		stored := storedConfig(t, svc)
		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)

		got, err := svc.GetAppConfig(ctx, testAppConfigTeamID)
		require.NoError(t, err)

		// The masks say "configured" without handing anything back.
		assert.True(t, got.HasPrivateKey)
		assert.True(t, got.HasClientSecret)
		assert.True(t, got.HasWebhookSecret)
		// A read is NOT a disclosure path: GitHubAppConfigResponse has no field
		// that could carry the plaintext webhook secret at all.
		assert.Equal(t, "https://vibexp.example/api/v1/webhooks/github/tok-abc", got.WebhookURL)
	})

	t.Run("absent config maps to the sentinel", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).
			Return(nil, repositories.ErrGitHubAppConfigNotFound)

		_, err := svc.GetAppConfig(ctx, testAppConfigTeamID)
		assert.ErrorIs(t, err, ErrGitHubAppNotConfigured)
	})
}

// The three-case update rule is the whole point of this test: absent keeps,
// non-empty replaces, and empty is an error rather than a silent clear.
func TestGitHubAppConfigService_UpdateSecretSemantics(t *testing.T) {
	ctx := context.Background()
	ptr := func(s string) *string { return &s }

	t.Run("absent field keeps the stored secret", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		stored := storedConfig(t, svc)
		before := stored.PrivateKeyEncrypted

		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)
		repo.EXPECT().Update(ctx, stored).Return(nil)

		_, err := svc.UpdateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID,
			models.UpdateGitHubAppConfigRequest{AppSlug: ptr("renamed")})
		require.NoError(t, err)
		assert.Equal(t, before, stored.PrivateKeyEncrypted, "an omitted secret must be untouched")
		assert.Equal(t, "renamed", stored.AppSlug)
	})

	t.Run("non-empty field re-encrypts and replaces", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		stored := storedConfig(t, svc)
		before := stored.PrivateKeyEncrypted
		newKey := testRSAPEM(t)

		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)
		repo.EXPECT().Update(ctx, stored).Return(nil)

		_, err := svc.UpdateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID,
			models.UpdateGitHubAppConfigRequest{PrivateKey: &newKey})
		require.NoError(t, err)
		assert.NotEqual(t, before, stored.PrivateKeyEncrypted)
		decrypted, err := svc.decrypt(stored.PrivateKeyEncrypted)
		require.NoError(t, err)
		assert.Equal(t, newKey, decrypted)
	})

	t.Run("explicitly empty field is a validation error, never a clear", func(t *testing.T) {
		for _, field := range []string{"private_key", "client_secret", "app_id", "app_slug", "client_id"} {
			t.Run(field, func(t *testing.T) {
				repo := mocks.NewMockGitHubAppConfigRepository(t)
				svc := newAppConfigService(t, repo)
				stored := storedConfig(t, svc)
				repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)

				req := models.UpdateGitHubAppConfigRequest{}
				switch field {
				case "private_key":
					req.PrivateKey = ptr("")
				case "client_secret":
					req.ClientSecret = ptr("")
				case "app_id":
					req.AppID = ptr("")
				case "app_slug":
					req.AppSlug = ptr("")
				case "client_id":
					req.ClientID = ptr("")
				}

				_, err := svc.UpdateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID, req)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cannot be empty")
				// repo.Update is never EXPECTed, so mockery fails the test if the
				// empty value reached the database.
			})
		}
	})

	t.Run("version conflict maps to the sentinel", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		stored := storedConfig(t, svc)
		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)
		repo.EXPECT().Update(ctx, stored).
			Return(repositories.ErrGitHubAppConfigVersionConflict)

		_, err := svc.UpdateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID,
			models.UpdateGitHubAppConfigRequest{AppSlug: ptr("x")})
		assert.ErrorIs(t, err, ErrGitHubAppConfigConflict)
	})
}

func TestGitHubAppConfigService_Delete(t *testing.T) {
	ctx := context.Background()

	repo := mocks.NewMockGitHubAppConfigRepository(t)
	svc := newAppConfigService(t, repo)
	stored := storedConfig(t, svc)
	repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)
	repo.EXPECT().Delete(ctx, testAppConfigTeamID, "cfg-1").Return(nil)

	require.NoError(t, svc.DeleteAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID))
}

// A credential EDIT invalidates the client cache by itself (the cache keys on
// version), but a DELETE has no new version to observe — so it must be pushed,
// or the deleted App's credentials sit in memory until LRU pressure.
func TestGitHubAppConfigService_DeleteEvictsCachedClient(t *testing.T) {
	ctx := context.Background()

	repo := mocks.NewMockGitHubAppConfigRepository(t)
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	evictor := &staticGitHubAppResolver{}
	svc := NewGitHubAppConfigService(repo, enc, permissiveProviderAuthz{}, evictor, "https://vibexp.example")

	stored := storedConfig(t, svc)
	repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)
	repo.EXPECT().Delete(ctx, testAppConfigTeamID, "cfg-1").Return(nil)

	require.NoError(t, svc.DeleteAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID))
	assert.Equal(t, []string{"cfg-1"}, evictor.evicted,
		"deleting a config must evict its cached client")
}

// A failed delete must NOT evict: the config still exists, and dropping its
// client would force a needless rebuild on the next call.
func TestGitHubAppConfigService_FailedDeleteDoesNotEvict(t *testing.T) {
	ctx := context.Background()

	repo := mocks.NewMockGitHubAppConfigRepository(t)
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	evictor := &staticGitHubAppResolver{}
	svc := NewGitHubAppConfigService(repo, enc, permissiveProviderAuthz{}, evictor, "https://vibexp.example")

	stored := storedConfig(t, svc)
	repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)
	repo.EXPECT().Delete(ctx, testAppConfigTeamID, "cfg-1").Return(errors.New("boom"))

	require.Error(t, svc.DeleteAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID))
	assert.Empty(t, evictor.evicted)
}

func TestGitHubAppConfigService_Rotation(t *testing.T) {
	ctx := context.Background()

	// Rotation is the ONLY recovery path for a lost webhook secret, so it must
	// both replace the stored ciphertext and disclose the new plaintext.
	t.Run("webhook secret rotation replaces and discloses", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		stored := storedConfig(t, svc)
		before := stored.WebhookSecretEncrypted

		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)
		repo.EXPECT().Update(ctx, stored).Return(nil)

		out, err := svc.RotateWebhookSecret(ctx, testAppConfigTeamID, testAppConfigUserID)
		require.NoError(t, err)
		assert.NotEmpty(t, out.WebhookSecret)
		assert.NotEqual(t, before, stored.WebhookSecretEncrypted)

		roundTripped, err := svc.decrypt(stored.WebhookSecretEncrypted)
		require.NoError(t, err)
		assert.Equal(t, out.WebhookSecret, roundTripped)
	})

	t.Run("token rotation changes the webhook URL", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		stored := storedConfig(t, svc)

		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)
		repo.EXPECT().Update(ctx, stored).Return(nil)

		out, err := svc.RotateWebhookToken(ctx, testAppConfigTeamID, testAppConfigUserID)
		require.NoError(t, err)
		assert.NotEqual(t, "tok-abc", stored.WebhookToken)
		assert.Regexp(t, regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`), stored.WebhookToken)
		assert.Contains(t, out.WebhookURL, stored.WebhookToken)
	})
}

// ResolveWebhookToken backs a PUBLIC, unauthenticated route, so its two
// outcomes both matter: a real token yields the App plus its decrypted secret,
// and an unknown one yields a typed sentinel the handler turns into a bare 404.
func TestGitHubAppConfigService_ResolveWebhookToken(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves the App and decrypts its secret", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		stored := storedConfig(t, svc)
		repo.EXPECT().GetByWebhookToken(ctx, "tok-abc").Return(stored, nil)

		target, err := svc.ResolveWebhookToken(ctx, "tok-abc")
		require.NoError(t, err)
		assert.Equal(t, "cfg-1", target.AppConfigID)
		assert.Equal(t, testAppConfigTeamID, target.TeamID)
		// The handler HMACs against this, so it must be plaintext, not ciphertext.
		assert.Equal(t, "ws-secret", target.WebhookSecret)
		assert.NotEqual(t, stored.WebhookSecretEncrypted, target.WebhookSecret)
	})

	t.Run("unknown token maps to the sentinel", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		repo.EXPECT().GetByWebhookToken(ctx, "nope").
			Return(nil, repositories.ErrGitHubAppConfigNotFound)

		_, err := svc.ResolveWebhookToken(ctx, "nope")
		assert.ErrorIs(t, err, ErrGitHubAppNotConfigured)
	})

	t.Run("transport failure is not laundered into not-configured", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		boom := errors.New("connection reset")
		repo.EXPECT().GetByWebhookToken(ctx, "tok-abc").Return(nil, boom)

		_, err := svc.ResolveWebhookToken(ctx, "tok-abc")
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrGitHubAppNotConfigured)
		assert.ErrorIs(t, err, boom)
	})
}
