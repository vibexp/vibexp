package services

import (
	"context"
	"errors"
	"testing"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

const (
	testTypeTeamID = "team-1"
	testTypeUserID = "user-1"
)

func newTypeService(t *testing.T) (*TypeService, *repomocks.MockTypeRepository) {
	t.Helper()
	repo := repomocks.NewMockTypeRepository(t)
	logger := slog.New(slog.DiscardHandler)
	return NewTypeService(repo, logger), repo
}

func TestTypeService_List_UnsupportedResourceType(t *testing.T) {
	svc, _ := newTypeService(t)
	_, err := svc.List(context.Background(), testTypeTeamID, "prompts")
	assert.ErrorIs(t, err, ErrTypeResourceTypeUnsupported)
}

func TestTypeService_List_Supported(t *testing.T) {
	svc, repo := newTypeService(t)
	want := []models.Type{{Slug: "general", IsSystem: true}}
	repo.EXPECT().List(mock.Anything, testTypeTeamID, "artifacts").Return(want, nil)

	got, err := svc.List(context.Background(), testTypeTeamID, "artifacts")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestTypeService_CreateCustom_Validation(t *testing.T) {
	cases := []struct {
		name    string
		params  CreateTypeParams
		wantErr error
	}{
		{
			"unsupported resource",
			CreateTypeParams{ResourceType: "prompts", Slug: "x", Name: "X"},
			ErrTypeResourceTypeUnsupported,
		},
		{"empty slug", CreateTypeParams{ResourceType: "artifacts", Slug: "", Name: "X"}, ErrTypeSlugRequired},
		{
			"bad slug chars",
			CreateTypeParams{ResourceType: "artifacts", Slug: "Bad Slug", Name: "X"},
			ErrTypeSlugInvalid,
		},
		{
			"uppercase slug",
			CreateTypeParams{ResourceType: "artifacts", Slug: "BugReport", Name: "X"},
			ErrTypeSlugInvalid,
		},
		{"empty name", CreateTypeParams{ResourceType: "artifacts", Slug: "bug-report", Name: ""}, ErrTypeNameRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Validation fails before any repository call, so the mock expects nothing.
			svc, _ := newTypeService(t)
			_, err := svc.CreateCustom(context.Background(), tc.params)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestTypeService_CreateCustom_Conflict(t *testing.T) {
	svc, repo := newTypeService(t)
	// An existing row (global default or team row) returned by GetBySlug is a collision.
	repo.EXPECT().GetBySlug(mock.Anything, testTypeTeamID, "artifacts", "general").
		Return(&models.Type{Slug: "general", IsSystem: true}, nil)

	_, err := svc.CreateCustom(context.Background(), CreateTypeParams{
		TeamID: testTypeTeamID, UserID: testTypeUserID, ResourceType: "artifacts", Slug: "general", Name: "General",
	})
	assert.ErrorIs(t, err, repositories.ErrTypeAlreadyExists)
}

func TestTypeService_CreateCustom_Success(t *testing.T) {
	svc, repo := newTypeService(t)
	repo.EXPECT().GetBySlug(mock.Anything, testTypeTeamID, "artifacts", "bug-report").
		Return(nil, repositories.ErrTypeNotFound)
	repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(tp *models.Type) bool {
		return tp.TeamID == testTypeTeamID && tp.CreatedBy == testTypeUserID &&
			tp.ResourceType == "artifacts" && tp.Slug == "bug-report" && tp.Name == "Bug report"
	})).Return(nil)

	got, err := svc.CreateCustom(context.Background(), CreateTypeParams{
		TeamID: testTypeTeamID, UserID: testTypeUserID, ResourceType: "artifacts", Slug: "bug-report", Name: "Bug report",
	})
	require.NoError(t, err)
	assert.Equal(t, "bug-report", got.Slug)
}

func TestTypeService_Delete_DelegatesWithGeneralFallback(t *testing.T) {
	svc, repo := newTypeService(t)
	repo.EXPECT().DeleteCustom(mock.Anything, testTypeTeamID, "type-1", "general").Return(nil)

	require.NoError(t, svc.Delete(context.Background(), testTypeTeamID, "type-1"))
}

func TestTypeService_ValidateType(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		svc, repo := newTypeService(t)
		repo.EXPECT().GetBySlug(mock.Anything, testTypeTeamID, "artifacts", "general").
			Return(&models.Type{Slug: "general"}, nil)
		ok, err := svc.ValidateType(context.Background(), testTypeTeamID, "artifacts", "general")
		require.NoError(t, err)
		assert.True(t, ok)
	})
	t.Run("unknown returns false not error", func(t *testing.T) {
		svc, repo := newTypeService(t)
		repo.EXPECT().GetBySlug(mock.Anything, testTypeTeamID, "artifacts", "nope").
			Return(nil, repositories.ErrTypeNotFound)
		ok, err := svc.ValidateType(context.Background(), testTypeTeamID, "artifacts", "nope")
		require.NoError(t, err)
		assert.False(t, ok)
	})
	t.Run("repo error propagates", func(t *testing.T) {
		svc, repo := newTypeService(t)
		repo.EXPECT().GetBySlug(mock.Anything, testTypeTeamID, "artifacts", "x").
			Return(nil, errors.New("db down"))
		_, err := svc.ValidateType(context.Background(), testTypeTeamID, "artifacts", "x")
		assert.Error(t, err)
	})
}
