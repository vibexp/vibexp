package services_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

const (
	authUserID = "user-123"
	authTeamID = "550e8400-e29b-41d4-a716-446655440000"
	authOwner  = "770e8400-e29b-41d4-a716-446655440000"
)

func TestAttachmentAuthorizerRegistry_UnknownOwnerType(t *testing.T) {
	reg := services.NewAttachmentAuthorizerRegistry()
	err := reg.Authorize(context.Background(), "memory", authUserID, authTeamID, authOwner)
	assert.ErrorIs(t, err, services.ErrAttachmentOwnerTypeUnknown)
}

func TestAttachmentAuthorizerRegistry_RegisteredAuthorizerRuns(t *testing.T) {
	reg := services.NewAttachmentAuthorizerRegistry()
	called := false
	reg.Register("artifact", func(_ context.Context, userID, teamID, ownerID string) error {
		called = true
		assert.Equal(t, authUserID, userID)
		assert.Equal(t, authTeamID, teamID)
		assert.Equal(t, authOwner, ownerID)
		return nil
	})

	err := reg.Authorize(context.Background(), "artifact", authUserID, authTeamID, authOwner)
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestAttachmentAuthorizerRegistry_AuthorizerErrorPropagates(t *testing.T) {
	reg := services.NewAttachmentAuthorizerRegistry()
	reg.Register("artifact", func(_ context.Context, _, _, _ string) error {
		return services.ErrAttachmentOwnerAccessDenied
	})

	err := reg.Authorize(context.Background(), "artifact", authUserID, authTeamID, authOwner)
	assert.ErrorIs(t, err, services.ErrAttachmentOwnerAccessDenied)
}

func TestArtifactAttachmentAuthorizer_AllowsAccessibleArtifact(t *testing.T) {
	artifacts := servicesmocks.NewMockArtifactServiceInterface(t)
	artifacts.On("GetArtifactByIDInTeam", authUserID, authTeamID, authOwner).
		Return(&models.Artifact{ID: authOwner, TeamID: authTeamID}, nil)

	authz := services.NewArtifactAttachmentAuthorizer(artifacts)
	assert.NoError(t, authz(context.Background(), authUserID, authTeamID, authOwner))
}

func TestArtifactAttachmentAuthorizer_DeniesInaccessibleArtifact(t *testing.T) {
	artifacts := servicesmocks.NewMockArtifactServiceInterface(t)
	artifacts.On("GetArtifactByIDInTeam", authUserID, authTeamID, authOwner).
		Return(nil, repositories.ErrArtifactNotFound)

	authz := services.NewArtifactAttachmentAuthorizer(artifacts)
	err := authz(context.Background(), authUserID, authTeamID, authOwner)
	// Missing and forbidden both collapse to ErrAttachmentOwnerAccessDenied so the
	// HTTP layer never leaks owner existence.
	assert.True(t, errors.Is(err, services.ErrAttachmentOwnerAccessDenied))
}

func TestArtifactAttachmentAuthorizer_PropagatesUnexpectedError(t *testing.T) {
	dbErr := errors.New("connection refused")
	artifacts := servicesmocks.NewMockArtifactServiceInterface(t)
	artifacts.On("GetArtifactByIDInTeam", authUserID, authTeamID, authOwner).
		Return(nil, dbErr)

	authz := services.NewArtifactAttachmentAuthorizer(artifacts)
	err := authz(context.Background(), authUserID, authTeamID, authOwner)
	// A real lookup failure must surface (→ 500), not be masked as access-denied (→ 404).
	assert.ErrorIs(t, err, dbErr)
	assert.False(t, errors.Is(err, services.ErrAttachmentOwnerAccessDenied))
}

func TestPromptAttachmentAuthorizer_AllowsAccessiblePrompt(t *testing.T) {
	prompts := servicesmocks.NewMockPromptServiceInterface(t)
	prompts.On("GetPrompt", authUserID, authTeamID, authOwner).
		Return(&models.Prompt{ID: authOwner, TeamID: authTeamID}, nil)

	authz := services.NewPromptAttachmentAuthorizer(prompts)
	assert.NoError(t, authz(context.Background(), authUserID, authTeamID, authOwner))
}

func TestPromptAttachmentAuthorizer_DeniesInaccessiblePrompt(t *testing.T) {
	prompts := servicesmocks.NewMockPromptServiceInterface(t)
	prompts.On("GetPrompt", authUserID, authTeamID, authOwner).
		Return(nil, repositories.ErrPromptNotFound)

	authz := services.NewPromptAttachmentAuthorizer(prompts)
	err := authz(context.Background(), authUserID, authTeamID, authOwner)
	// Missing and forbidden both collapse to ErrAttachmentOwnerAccessDenied so the
	// HTTP layer never leaks owner existence.
	assert.True(t, errors.Is(err, services.ErrAttachmentOwnerAccessDenied))
}

func TestPromptAttachmentAuthorizer_PropagatesUnexpectedError(t *testing.T) {
	dbErr := errors.New("connection refused")
	prompts := servicesmocks.NewMockPromptServiceInterface(t)
	prompts.On("GetPrompt", authUserID, authTeamID, authOwner).
		Return(nil, dbErr)

	authz := services.NewPromptAttachmentAuthorizer(prompts)
	err := authz(context.Background(), authUserID, authTeamID, authOwner)
	// A real lookup failure must surface (→ 500), not be masked as access-denied (→ 404).
	assert.ErrorIs(t, err, dbErr)
	assert.False(t, errors.Is(err, services.ErrAttachmentOwnerAccessDenied))
}

func TestBlueprintAttachmentAuthorizer_AllowsAccessibleBlueprint(t *testing.T) {
	blueprints := servicesmocks.NewMockBlueprintServiceInterface(t)
	blueprints.On("GetBlueprintByIDInTeam", authUserID, authTeamID, authOwner).
		Return(&models.Blueprint{ID: authOwner, TeamID: authTeamID}, nil)

	authz := services.NewBlueprintAttachmentAuthorizer(blueprints)
	assert.NoError(t, authz(context.Background(), authUserID, authTeamID, authOwner))
}

func TestBlueprintAttachmentAuthorizer_DeniesInaccessibleBlueprint(t *testing.T) {
	blueprints := servicesmocks.NewMockBlueprintServiceInterface(t)
	blueprints.On("GetBlueprintByIDInTeam", authUserID, authTeamID, authOwner).
		Return(nil, repositories.ErrBlueprintNotFound)

	authz := services.NewBlueprintAttachmentAuthorizer(blueprints)
	err := authz(context.Background(), authUserID, authTeamID, authOwner)
	// Missing and forbidden both collapse to ErrAttachmentOwnerAccessDenied so the
	// HTTP layer never leaks owner existence.
	assert.True(t, errors.Is(err, services.ErrAttachmentOwnerAccessDenied))
}

func TestBlueprintAttachmentAuthorizer_PropagatesUnexpectedError(t *testing.T) {
	dbErr := errors.New("connection refused")
	blueprints := servicesmocks.NewMockBlueprintServiceInterface(t)
	blueprints.On("GetBlueprintByIDInTeam", authUserID, authTeamID, authOwner).
		Return(nil, dbErr)

	authz := services.NewBlueprintAttachmentAuthorizer(blueprints)
	err := authz(context.Background(), authUserID, authTeamID, authOwner)
	// A real lookup failure must surface (→ 500), not be masked as access-denied (→ 404).
	assert.ErrorIs(t, err, dbErr)
	assert.False(t, errors.Is(err, services.ErrAttachmentOwnerAccessDenied))
}
