package services_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
)

func newMetadataCatalogService(
	t *testing.T,
) (*services.MetadataCatalogService, *repomocks.MockMetadataCatalogRepository) {
	t.Helper()
	repo := repomocks.NewMockMetadataCatalogRepository(t)
	return services.NewMetadataCatalogService(repo, slog.New(slog.DiscardHandler)), repo
}

func validCatalogQuery() repositories.MetadataCatalogQuery {
	return repositories.MetadataCatalogQuery{
		UserID:       "user-1",
		TeamID:       "team-1",
		ResourceType: repositories.MetadataResourceArtifacts,
		Limit:        50,
	}
}

func TestMetadataCatalogService_KeysDelegatesToRepository(t *testing.T) {
	svc, repo := newMetadataCatalogService(t)
	query := validCatalogQuery()
	want := repositories.MetadataCatalogResult{Entries: []string{"env"}, Truncated: true}
	repo.EXPECT().Keys(mock.Anything, query).Return(want, nil)

	got, err := svc.Keys(context.Background(), query)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestMetadataCatalogService_ValuesDelegatesToRepository(t *testing.T) {
	svc, repo := newMetadataCatalogService(t)
	query := validCatalogQuery()
	query.Key = "env"
	want := repositories.MetadataCatalogResult{Entries: []string{"prod"}}
	repo.EXPECT().Values(mock.Anything, query).Return(want, nil)

	got, err := svc.Values(context.Background(), query)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestMetadataCatalogService_RejectsBeforeTouchingTheRepository is the guard
// that keeps an unknown resource type away from SQL construction: the mock has
// no EXPECT calls, so any repository call fails the test.
func TestMetadataCatalogService_RejectsBeforeTouchingTheRepository(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*repositories.MetadataCatalogQuery)
		values  bool
		wantMsg string
	}{
		{
			name:    "unknown resource type",
			mutate:  func(q *repositories.MetadataCatalogQuery) { q.ResourceType = "prompts" },
			wantMsg: "unknown resource_type",
		},
		{
			name:    "resource type carrying SQL",
			mutate:  func(q *repositories.MetadataCatalogQuery) { q.ResourceType = "artifacts; DROP TABLE artifacts" },
			wantMsg: "unknown resource_type",
		},
		{
			name:    "empty resource type",
			mutate:  func(q *repositories.MetadataCatalogQuery) { q.ResourceType = "" },
			wantMsg: "unknown resource_type",
		},
		{
			name:    "missing team",
			mutate:  func(q *repositories.MetadataCatalogQuery) { q.TeamID = "" },
			wantMsg: "team_id is required",
		},
		{
			name:    "values without a key",
			mutate:  func(q *repositories.MetadataCatalogQuery) { q.Key = "" },
			values:  true,
			wantMsg: "key is required",
		},
		{
			name:    "values with an over-long key",
			mutate:  func(q *repositories.MetadataCatalogQuery) { q.Key = strings.Repeat("k", 256) },
			values:  true,
			wantMsg: "key length must be at most 255",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newMetadataCatalogService(t)
			query := validCatalogQuery()
			query.Key = "env"
			tt.mutate(&query)

			var err error
			if tt.values {
				_, err = svc.Values(context.Background(), query)
			} else {
				_, err = svc.Keys(context.Background(), query)
			}

			require.Error(t, err)
			assert.ErrorIs(t, err, services.ErrInvalidMetadataCatalogQuery)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestMetadataCatalogService_PropagatesRepositoryErrors(t *testing.T) {
	repoErr := errors.New("connection refused")

	t.Run("keys", func(t *testing.T) {
		svc, repo := newMetadataCatalogService(t)
		repo.EXPECT().Keys(mock.Anything, mock.Anything).
			Return(repositories.MetadataCatalogResult{}, repoErr)

		_, err := svc.Keys(context.Background(), validCatalogQuery())

		require.ErrorIs(t, err, repoErr)
		// A backend failure must NOT masquerade as a caller mistake.
		assert.NotErrorIs(t, err, services.ErrInvalidMetadataCatalogQuery)
	})

	t.Run("values", func(t *testing.T) {
		svc, repo := newMetadataCatalogService(t)
		repo.EXPECT().Values(mock.Anything, mock.Anything).
			Return(repositories.MetadataCatalogResult{}, repoErr)

		query := validCatalogQuery()
		query.Key = "env"
		_, err := svc.Values(context.Background(), query)

		require.ErrorIs(t, err, repoErr)
	})
}
