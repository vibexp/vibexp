package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

func createTestModelProviderService(repo repositories.ModelProviderRepository) *ModelProviderService {
	// testEncryptionKey is a valid 32-byte key, so this never errors.
	enc, err := NewEncryptionService(testEncryptionKey)
	if err != nil {
		panic(err)
	}
	return NewModelProviderService(repo, enc, localDevProviderConfig(), permissiveProviderAuthz{})
}

// TestModelProviderService_EncryptDecryptRoundTrip verifies the service round-trips
// a secret through the shared EncryptionService and that empty input is a no-op
// both ways (the empty-string passthrough preserved in the delegating wrappers).
func TestModelProviderService_EncryptDecryptRoundTrip(t *testing.T) {
	svc := createTestModelProviderService(nil)

	encrypted, err := svc.encrypt("sk-super-secret-key")
	require.NoError(t, err)
	assert.NotEmpty(t, encrypted)
	assert.NotEqual(t, "sk-super-secret-key", encrypted)

	decrypted, err := svc.decrypt(encrypted)
	require.NoError(t, err)
	assert.Equal(t, "sk-super-secret-key", decrypted)

	// Empty input is a no-op both ways.
	enc, err := svc.encrypt("")
	require.NoError(t, err)
	assert.Empty(t, enc)
	dec, err := svc.decrypt("")
	require.NoError(t, err)
	assert.Empty(t, dec)
}

// TestModelProviderService_CreateModelProvider_SetsDefault verifies a create with
// is_default true encrypts the key, persists the row, and flags it default.
func TestModelProviderService_CreateModelProvider_SetsDefault(t *testing.T) {
	mockRepo := mocks.NewMockModelProviderRepository(t)
	svc := createTestModelProviderService(mockRepo)

	req := models.CreateModelProviderRequest{
		Name:         "OpenAI",
		ProviderType: "openai_compatible",
		Model:        "gpt-4o-mini",
		IsDefault:    boolPtr(true),
		BaseURL:      stringPtr("https://api.openai.com/v1"),
		APIKey:       stringPtr("sk-test"),
		Configuration: map[string]interface{}{
			"temperature": 0.7,
		},
	}

	mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(p *models.ModelProvider) bool {
		// The API key is encrypted (never stored as plaintext) and configuration is
		// marshalled to JSON.
		return p.Name == "OpenAI" &&
			p.APIKeyEncrypted != nil && *p.APIKeyEncrypted != "sk-test" &&
			p.Configuration != "" && p.Configuration != "{}"
	})).Run(func(args mock.Arguments) {
		args.Get(1).(*models.ModelProvider).ID = "provider-new"
	}).Return(nil)
	mockRepo.On("SetDefault", mock.Anything, "team-1", "provider-new").Return(nil)

	provider, err := svc.CreateModelProvider(context.Background(), "team-1", "user-1", req)
	require.NoError(t, err)
	assert.Equal(t, "provider-new", provider.ID)
	mockRepo.AssertExpectations(t)
}

// TestModelProviderService_CreateModelProvider_Duplicate maps a unique-constraint
// error from the repository to the ErrModelProviderAlreadyExists sentinel.
func TestModelProviderService_CreateModelProvider_Duplicate(t *testing.T) {
	mockRepo := mocks.NewMockModelProviderRepository(t)
	svc := createTestModelProviderService(mockRepo)

	req := models.CreateModelProviderRequest{
		Name:         "Dup",
		ProviderType: "openai_compatible",
		Model:        "gpt-4o-mini",
	}
	mockRepo.On("Create", mock.Anything, mock.Anything).
		Return(assertUniqueViolation())

	_, err := svc.CreateModelProvider(context.Background(), "team-1", "user-1", req)
	require.ErrorIs(t, err, ErrModelProviderAlreadyExists)
	mockRepo.AssertExpectations(t)
}

// TestModelProviderService_GetModelProvider_MasksKey verifies the encrypted key is
// stripped from the response and has_api_key reflects its presence.
func TestModelProviderService_GetModelProvider_MasksKey(t *testing.T) {
	mockRepo := mocks.NewMockModelProviderRepository(t)
	svc := createTestModelProviderService(mockRepo)

	encrypted := "encrypted-blob"
	mockRepo.On("GetByID", mock.Anything, "team-1", "provider-1").
		Return(&models.ModelProvider{ID: "provider-1", APIKeyEncrypted: &encrypted}, nil)

	resp, err := svc.GetModelProvider(context.Background(), "team-1", "provider-1")
	require.NoError(t, err)
	assert.Nil(t, resp.APIKeyEncrypted)
	assert.True(t, resp.HasAPIKey)
	mockRepo.AssertExpectations(t)
}

// TestModelProviderService_GetModelProvider_NotFound wraps a repo miss in the
// ErrModelProviderNotFound sentinel.
func TestModelProviderService_GetModelProvider_NotFound(t *testing.T) {
	mockRepo := mocks.NewMockModelProviderRepository(t)
	svc := createTestModelProviderService(mockRepo)

	mockRepo.On("GetByID", mock.Anything, "team-1", "missing").
		Return((*models.ModelProvider)(nil), repositories.ErrModelProviderNotFound)

	_, err := svc.GetModelProvider(context.Background(), "team-1", "missing")
	require.ErrorIs(t, err, ErrModelProviderNotFound)
	mockRepo.AssertExpectations(t)
}

// TestModelProviderService_DeleteModelProvider_LastBlocked blocks deleting the
// team's last remaining provider.
func TestModelProviderService_DeleteModelProvider_LastBlocked(t *testing.T) {
	mockRepo := mocks.NewMockModelProviderRepository(t)
	svc := createTestModelProviderService(mockRepo)

	mockRepo.On("GetByID", mock.Anything, "team-1", "provider-1").
		Return(&models.ModelProvider{ID: "provider-1"}, nil)
	mockRepo.On("Count", mock.Anything, "team-1").Return(1, nil)

	err := svc.DeleteModelProvider(context.Background(), "team-1", testProviderUserID, "provider-1")
	require.ErrorIs(t, err, ErrLastModelProviderDelete)
	mockRepo.AssertExpectations(t)
}

// TestModelProviderService_DeleteModelProvider_Success deletes when more than one
// provider remains.
func TestModelProviderService_DeleteModelProvider_Success(t *testing.T) {
	mockRepo := mocks.NewMockModelProviderRepository(t)
	svc := createTestModelProviderService(mockRepo)

	mockRepo.On("GetByID", mock.Anything, "team-1", "provider-1").
		Return(&models.ModelProvider{ID: "provider-1"}, nil)
	mockRepo.On("Count", mock.Anything, "team-1").Return(2, nil)
	mockRepo.On("Delete", mock.Anything, "team-1", "provider-1").Return(nil)

	require.NoError(t, svc.DeleteModelProvider(context.Background(), "team-1", testProviderUserID, "provider-1"))
	mockRepo.AssertExpectations(t)
}

// TestModelProviderService_UpdateModelProvider_BlankKeyPreserved verifies an update
// that omits api_key leaves the stored encrypted key untouched.
func TestModelProviderService_UpdateModelProvider_BlankKeyPreserved(t *testing.T) {
	mockRepo := mocks.NewMockModelProviderRepository(t)
	svc := createTestModelProviderService(mockRepo)

	stored := "stored-encrypted-key"
	newName := "Renamed"
	mockRepo.On("GetByID", mock.Anything, "team-1", "provider-1").
		Return(&models.ModelProvider{ID: "provider-1", Name: "Old", APIKeyEncrypted: &stored}, nil)
	mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(p *models.ModelProvider) bool {
		// api_key omitted → the stored encrypted key is preserved.
		return p.Name == "Renamed" && p.APIKeyEncrypted != nil && *p.APIKeyEncrypted == stored
	})).Return(nil)

	_, err := svc.UpdateModelProvider(context.Background(), "team-1", testProviderUserID, "provider-1",
		models.UpdateModelProviderRequest{Name: &newName})
	require.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

// TestModelProviderService_Validate_ModelsProbeSucceeds accepts a provider whose
// GET /models listing returns 200.
func TestModelProviderService_Validate_ModelsProbeSucceeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	svc := createTestModelProviderService(nil)
	resp, err := svc.ValidateModelProvider(context.Background(), testProviderTeamID, testProviderUserID, models.ValidateModelProviderRequest{
		ProviderType: "openai_compatible",
		Model:        "gpt-4o-mini",
		BaseURL:      server.URL,
	})
	require.NoError(t, err)
	assert.True(t, resp.IsValid)
	assert.Equal(t, http.StatusOK, resp.Details.StatusCode)
}

// TestModelProviderService_Validate_FallsBackToChatCompletions accepts a provider
// that 404s on /models but answers /chat/completions.
func TestModelProviderService_Validate_FallsBackToChatCompletions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	svc := createTestModelProviderService(nil)
	resp, err := svc.ValidateModelProvider(context.Background(), testProviderTeamID, testProviderUserID, models.ValidateModelProviderRequest{
		ProviderType: "openai_compatible",
		Model:        "gpt-4o-mini",
		BaseURL:      server.URL,
	})
	require.NoError(t, err)
	assert.True(t, resp.IsValid)
	assert.Equal(t, http.StatusOK, resp.Details.StatusCode)
}

// TestModelProviderService_Validate_Unauthorized reports is_valid=false with an
// auth message when both probes return 401.
func TestModelProviderService_Validate_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	svc := createTestModelProviderService(nil)
	resp, err := svc.ValidateModelProvider(context.Background(), testProviderTeamID, testProviderUserID, models.ValidateModelProviderRequest{
		ProviderType: "openai_compatible",
		Model:        "gpt-4o-mini",
		BaseURL:      server.URL,
		APIKey:       stringPtr("bad-key"),
	})
	require.NoError(t, err)
	assert.False(t, resp.IsValid)
	assert.Equal(t, http.StatusUnauthorized, resp.Details.StatusCode)
	assert.Contains(t, resp.Message, "Authentication failed")
}

// TestModelProviderService_Validate_Unreachable reports is_valid=false with a
// reach-failure message when both probes fail at the transport layer (the host is
// closed/unreachable), exercising the both-errors branch.
func TestModelProviderService_Validate_Unreachable(t *testing.T) {
	// Spin up a server only to grab a guaranteed-free URL, then close it so every
	// connection attempt fails at the transport layer.
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := server.URL
	server.Close()

	svc := createTestModelProviderService(nil)
	resp, err := svc.ValidateModelProvider(context.Background(), testProviderTeamID, testProviderUserID, models.ValidateModelProviderRequest{
		ProviderType: "openai_compatible",
		Model:        "gpt-4o-mini",
		BaseURL:      closedURL,
	})
	require.NoError(t, err)
	assert.False(t, resp.IsValid)
	assert.Contains(t, resp.Message, "Failed to reach the model provider")
	assert.NotEmpty(t, resp.Details.ErrorDetails)
}

// TestModelProviderService_Validate_UnsupportedType returns is_valid=false for an
// unknown provider type without touching the network.
func TestModelProviderService_Validate_UnsupportedType(t *testing.T) {
	svc := createTestModelProviderService(nil)
	resp, err := svc.ValidateModelProvider(context.Background(), testProviderTeamID, testProviderUserID, models.ValidateModelProviderRequest{
		ProviderType: "unsupported",
		Model:        "m",
		BaseURL:      "https://example.com/v1",
	})
	require.NoError(t, err)
	assert.False(t, resp.IsValid)
	assert.Contains(t, resp.Message, "Unsupported provider type")
}

// assertUniqueViolation returns an error whose text triggers the service's
// duplicate-detection branch.
func assertUniqueViolation() error {
	return &pqLikeError{msg: "pq: duplicate key value violates unique constraint"}
}

type pqLikeError struct{ msg string }

func (e *pqLikeError) Error() string { return e.msg }
