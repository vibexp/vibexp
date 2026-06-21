package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

// buildFixtureUser returns a fully-populated *models.User including all sensitive fields.
func buildFixtureUser() *models.User {
	googleID := "google-sensitive-123"
	idpProvider := "google"
	idpSubject := "subj-sensitive-456"
	stripeCustomerID := "cus_sensitive_abc"
	avatarURL := "https://example.com/avatar.png"
	defaultTeamID := "team-uuid-fixture"
	plan := "teams_pro"
	canceledAt := time.Now().Add(-24 * time.Hour)

	return &models.User{
		ID:                     "user-uuid-fixture",
		GoogleID:               &googleID,
		IDPProvider:            &idpProvider,
		IDPSubject:             &idpSubject,
		Email:                  "jane@example.com",
		Name:                   "Jane Doe",
		AvatarURL:              &avatarURL,
		StripeCustomerID:       &stripeCustomerID,
		SubscriptionStatus:     "active",
		SubscriptionPlan:       &plan,
		SubscriptionCanceledAt: &canceledAt,
		DefaultTeamID:          &defaultTeamID,
		OnboardingCompleted:    true,
		CreatedAt:              time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		Version:                7,
	}
}

func TestGetUserWithUser_Success(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	mockAuthService := mocks.NewMockAuthServiceInterface(t)
	srv.container = &TestContainer{AuthServiceMock: mockAuthService}

	fixture := buildFixtureUser()
	mockAuthService.On("GetUserByID", context.Background(), "user-uuid-fixture").
		Return(fixture, nil)

	result, structuredResult, err := srv.getUserWithUser(
		context.Background(), nil, &GetUserParams{}, "user-uuid-fixture",
	)

	// Basic success assertions
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	// Structured content must be *models.UserBasicInfo
	info, ok := structuredResult.(*models.UserBasicInfo)
	require.True(t, ok, "expected structuredResult to be *models.UserBasicInfo")
	assert.Equal(t, "user-uuid-fixture", info.ID)
	assert.Equal(t, "jane@example.com", info.Email)
	assert.Equal(t, "Jane Doe", info.Name)
	require.NotNil(t, info.DefaultTeamID)
	assert.Equal(t, "team-uuid-fixture", *info.DefaultTeamID)

	// Verify JSON content
	textContent, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "expected TextContent in result")

	var parsed models.UserBasicInfo
	require.NoError(t, json.Unmarshal([]byte(textContent.Text), &parsed))
	assert.Equal(t, "user-uuid-fixture", parsed.ID)
	assert.Equal(t, "jane@example.com", parsed.Email)
	assert.Equal(t, "Jane Doe", parsed.Name)

	// Sensitive fields must be absent from JSON text
	sensitiveFields := []string{
		"google_id",
		"idp_provider",
		"idp_subject",
		"stripe_customer_id",
		"subscription_canceled_at",
		"version",
	}
	for _, field := range sensitiveFields {
		assert.False(
			t,
			strings.Contains(textContent.Text, `"`+field+`"`),
			"JSON must not contain sensitive field %q", field,
		)
	}

	mockAuthService.AssertExpectations(t)
}

func TestGetUserWithUser_ServiceError(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	mockAuthService := mocks.NewMockAuthServiceInterface(t)
	srv.container = &TestContainer{AuthServiceMock: mockAuthService}

	mockAuthService.On("GetUserByID", context.Background(), "user-error").
		Return(nil, errors.New("db error"))

	result, structuredResult, err := srv.getUserWithUser(
		context.Background(), nil, &GetUserParams{}, "user-error",
	)

	require.NoError(t, err, "infrastructure errors are wrapped in result, not returned")
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Nil(t, structuredResult)

	require.NotEmpty(t, result.Content)
	textContent, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "Failed to get user")

	mockAuthService.AssertExpectations(t)
}

func TestGetUserWithUser_FieldExclusion(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	mockAuthService := mocks.NewMockAuthServiceInterface(t)
	srv.container = &TestContainer{AuthServiceMock: mockAuthService}

	fixture := buildFixtureUser()
	mockAuthService.On("GetUserByID", context.Background(), fixture.ID).
		Return(fixture, nil)

	result, _, err := srv.getUserWithUser(
		context.Background(), nil, &GetUserParams{}, fixture.ID,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)

	textContent, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	rawJSON := textContent.Text

	// All sensitive fields that hold real data in the fixture must be absent
	assert.False(t, strings.Contains(rawJSON, `"google_id"`), "google_id must be excluded")
	assert.False(t, strings.Contains(rawJSON, `"idp_provider"`), "idp_provider must be excluded")
	assert.False(t, strings.Contains(rawJSON, `"idp_subject"`), "idp_subject must be excluded")
	assert.False(t, strings.Contains(rawJSON, `"stripe_customer_id"`), "stripe_customer_id must be excluded")
	assert.False(t, strings.Contains(rawJSON, `"subscription_canceled_at"`), "subscription_canceled_at must be excluded")
	assert.False(t, strings.Contains(rawJSON, `"version"`), "version must be excluded")

	// Safe fields must be present
	assert.True(t, strings.Contains(rawJSON, `"id"`))
	assert.True(t, strings.Contains(rawJSON, `"email"`))
	assert.True(t, strings.Contains(rawJSON, `"name"`))
	assert.True(t, strings.Contains(rawJSON, `"subscription_status"`))
	assert.True(t, strings.Contains(rawJSON, `"onboarding_completed"`))
	assert.True(t, strings.Contains(rawJSON, `"created_at"`))

	mockAuthService.AssertExpectations(t)
}

func TestAddAllTools_RegistersGetUser(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: "1.0.0",
	}, nil)

	manager := NewMCPToolsManager(srv)
	manager.AddAllTools(mcpServer, "test-user")

	// Verify the tool is registered by connecting a client and listing tools
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverSession, err := mcpServer.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := serverSession.Close(); closeErr != nil {
			t.Logf("serverSession.Close: %v", closeErr)
		}
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := clientSession.Close(); closeErr != nil {
			t.Logf("clientSession.Close: %v", closeErr)
		}
	})

	listResult, err := clientSession.ListTools(ctx, nil)
	require.NoError(t, err)

	toolNames := make([]string, 0, len(listResult.Tools))
	for _, tool := range listResult.Tools {
		toolNames = append(toolNames, tool.Name)
	}

	assert.Contains(t, toolNames, "vibexp_io_get_user",
		"AddAllTools should register vibexp_io_get_user; registered tools: %v", toolNames)
}
