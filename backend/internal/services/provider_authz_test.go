package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// Regression tests for the authorization half of #464. Provider settings hold
// encrypted API keys and decide where a team's embedding/model traffic goes, but
// the endpoints sat behind team-membership validation only — and every user has
// a personal team, so that was effectively any authenticated user.

// deniedEmbeddingService builds an embedding service whose authz refuses
// everything, standing in for a plain team member.
func deniedEmbeddingService(t *testing.T) *EmbeddingProviderService {
	t.Helper()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	// The repo mock is deliberately given no expectations: a denied call must
	// not reach it, and mockery fails the test if it does.
	return NewEmbeddingProviderService(
		mocks.NewMockEmbeddingProviderRepository(t), enc,
		localDevProviderConfig(), denyingProviderAuthz{},
	)
}

func deniedModelService(t *testing.T) *ModelProviderService {
	t.Helper()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	return NewModelProviderService(
		mocks.NewMockModelProviderRepository(t), enc,
		localDevProviderConfig(), denyingProviderAuthz{},
	)
}

func TestEmbeddingProviderMutations_DeniedForMember(t *testing.T) {
	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		_, err := deniedEmbeddingService(t).CreateEmbeddingProvider(
			ctx, testProviderTeamID, testProviderUserID,
			models.CreateEmbeddingProviderRequest{Name: "n", ProviderType: ProviderTypeOpenAICompatible},
		)
		assert.True(t, errors.Is(err, ErrPermissionDenied), "got: %v", err)
	})

	t.Run("update", func(t *testing.T) {
		_, err := deniedEmbeddingService(t).UpdateEmbeddingProvider(
			ctx, testProviderTeamID, testProviderUserID, "provider-1",
			models.UpdateEmbeddingProviderRequest{},
		)
		assert.True(t, errors.Is(err, ErrPermissionDenied), "got: %v", err)
	})

	t.Run("delete", func(t *testing.T) {
		err := deniedEmbeddingService(t).DeleteEmbeddingProvider(
			ctx, testProviderTeamID, testProviderUserID, "provider-1",
		)
		assert.True(t, errors.Is(err, ErrPermissionDenied), "got: %v", err)
	})

	t.Run("validate", func(t *testing.T) {
		// /validate persists nothing but makes the server fetch a caller-supplied
		// URL, which is the SSRF primitive — so it is gated like a mutation.
		_, err := deniedEmbeddingService(t).ValidateEmbeddingProvider(
			ctx, testProviderTeamID, testProviderUserID,
			models.ValidateEmbeddingProviderRequest{
				ProviderType: ProviderTypeOpenAICompatible,
				BaseURL:      "https://api.openai.com/v1",
				Model:        "m",
			},
		)
		assert.True(t, errors.Is(err, ErrPermissionDenied), "got: %v", err)
	})
}

func TestModelProviderMutations_DeniedForMember(t *testing.T) {
	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		_, err := deniedModelService(t).CreateModelProvider(
			ctx, testProviderTeamID, testProviderUserID,
			models.CreateModelProviderRequest{Name: "n", ProviderType: ProviderTypeOpenAICompatible},
		)
		assert.True(t, errors.Is(err, ErrPermissionDenied), "got: %v", err)
	})

	t.Run("update", func(t *testing.T) {
		_, err := deniedModelService(t).UpdateModelProvider(
			ctx, testProviderTeamID, testProviderUserID, "provider-1",
			models.UpdateModelProviderRequest{},
		)
		assert.True(t, errors.Is(err, ErrPermissionDenied), "got: %v", err)
	})

	t.Run("delete", func(t *testing.T) {
		err := deniedModelService(t).DeleteModelProvider(
			ctx, testProviderTeamID, testProviderUserID, "provider-1",
		)
		assert.True(t, errors.Is(err, ErrPermissionDenied), "got: %v", err)
	})

	t.Run("validate", func(t *testing.T) {
		_, err := deniedModelService(t).ValidateModelProvider(
			ctx, testProviderTeamID, testProviderUserID,
			models.ValidateModelProviderRequest{
				ProviderType: ProviderTypeOpenAICompatible,
				BaseURL:      "https://api.openai.com/v1",
				Model:        "m",
			},
		)
		assert.True(t, errors.Is(err, ErrPermissionDenied), "got: %v", err)
	})
}

// TestProviderService_NilAuthzFailsClosed pins that a service constructed
// without an authorization service denies rather than allows. Wiring is the kind
// of thing that regresses silently, and the failure mode must be "nobody can"
// rather than "everybody can".
func TestProviderService_NilAuthzFailsClosed(t *testing.T) {
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	eps := NewEmbeddingProviderService(
		mocks.NewMockEmbeddingProviderRepository(t), enc, localDevProviderConfig(), nil,
	)
	_, embErr := eps.CreateEmbeddingProvider(
		context.Background(), testProviderTeamID, testProviderUserID,
		models.CreateEmbeddingProviderRequest{Name: "n", ProviderType: ProviderTypeOpenAICompatible},
	)
	assert.True(t, errors.Is(embErr, ErrPermissionDenied), "got: %v", embErr)

	mps := NewModelProviderService(
		mocks.NewMockModelProviderRepository(t), enc, localDevProviderConfig(), nil,
	)
	_, modErr := mps.CreateModelProvider(
		context.Background(), testProviderTeamID, testProviderUserID,
		models.CreateModelProviderRequest{Name: "n", ProviderType: ProviderTypeOpenAICompatible},
	)
	assert.True(t, errors.Is(modErr, ErrPermissionDenied), "got: %v", modErr)
}

// TestResolveActiveProvider_StoredProviderIsGuarded is the half of the finding
// that is NOT about the validate probe: a base_url already persisted on a
// provider row redirects every document the team embeds. The runtime client must
// carry the same guard, so a stored internal address cannot be used to exfiltrate
// content (or reach the internal network) after the fact.
func TestResolveActiveProvider_StoredProviderIsGuarded(t *testing.T) {
	repo := mocks.NewMockEmbeddingProviderRepository(t)
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	internal := "http://169.254.169.254/v1"
	repo.On("GetActiveProvider", mock.Anything, testProviderTeamID).Return(
		&models.EmbeddingProvider{
			ProviderType: ProviderTypeOpenAICompatible,
			BaseURL:      &internal,
			Model:        "m",
			ChunkSize:    1000,
			ChunkOverlap: 200,
			Concurrency:  1,
		}, nil,
	)

	svc := NewEmbeddingProviderService(
		repo, enc,
		&config.Config{Frontend: config.FrontendConfig{BaseURL: "https://app.example.com"}},
		permissiveProviderAuthz{},
	)

	resolved, err := svc.ResolveActiveProvider(context.Background(), testProviderTeamID)
	require.NoError(t, err)
	require.NotNil(t, resolved)

	// Resolution itself succeeds (the row is valid config); the guard bites when
	// the provider actually dials.
	_, embedErr := resolved.Provider.GenerateEmbeddings(context.Background(), []string{"secret document text"})

	require.Error(t, embedErr, "a stored internal base_url must not be dialable")
	assert.Contains(t, embedErr.Error(), "disallowed address range")
}
