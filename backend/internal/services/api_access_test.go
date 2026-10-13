package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repo_mocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

func TestAPIAccessService_RevokeAPIAccess(t *testing.T) {
	ctx := context.Background()

	t.Run("returns what was revoked and logs it once", func(t *testing.T) {
		repo := repo_mocks.NewMockAPIAccessRepository(t)
		cutoff := time.Now()
		repo.On("RevokeAll", ctx, "user-1").
			Return(&models.APIAccessRevocation{TokensValidAfter: cutoff, APIKeys: 2, OAuthGrants: 5}, nil).Once()
		logger, recorder := logtest.New()

		got, err := NewAPIAccessService(repo, logger).RevokeAPIAccess(ctx, "user-1")

		require.NoError(t, err)
		assert.EqualValues(t, 2, got.APIKeys)
		assert.EqualValues(t, 5, got.OAuthGrants)
		assert.True(t, cutoff.Equal(got.TokensValidAfter))
		entries := recorder.AllEntries()
		require.Len(t, entries, 1)
		assert.Equal(t, slog.LevelInfo, entries[0].Level)
		assert.Equal(t, "user-1", entries[0].Data["user_id"])
	})

	t.Run("is safe to call twice", func(t *testing.T) {
		repo := repo_mocks.NewMockAPIAccessRepository(t)
		repo.On("RevokeAll", ctx, "user-1").
			Return(&models.APIAccessRevocation{TokensValidAfter: time.Now(), APIKeys: 1, OAuthGrants: 3}, nil).Once()
		repo.On("RevokeAll", ctx, "user-1").
			Return(&models.APIAccessRevocation{TokensValidAfter: time.Now()}, nil).Once()
		logger, _ := logtest.New()
		svc := NewAPIAccessService(repo, logger)

		_, err := svc.RevokeAPIAccess(ctx, "user-1")
		require.NoError(t, err)
		again, err := svc.RevokeAPIAccess(ctx, "user-1")

		require.NoError(t, err, "nothing left to revoke is not an error")
		assert.Zero(t, again.APIKeys)
		assert.Zero(t, again.OAuthGrants)
	})

	t.Run("an unknown user stays distinguishable", func(t *testing.T) {
		repo := repo_mocks.NewMockAPIAccessRepository(t)
		repo.On("RevokeAll", ctx, "ghost").Return(nil, repositories.ErrUserNotFound)
		logger, recorder := logtest.New()

		got, err := NewAPIAccessService(repo, logger).RevokeAPIAccess(ctx, "ghost")

		require.ErrorIs(t, err, repositories.ErrUserNotFound)
		assert.Nil(t, got)
		assert.Empty(t, recorder.AllEntries(), "a revocation that did not happen is not logged as one")
	})

	t.Run("a repository failure is returned", func(t *testing.T) {
		repo := repo_mocks.NewMockAPIAccessRepository(t)
		boom := errors.New("connection reset")
		repo.On("RevokeAll", ctx, "user-1").Return(nil, boom)
		logger, _ := logtest.New()

		_, err := NewAPIAccessService(repo, logger).RevokeAPIAccess(ctx, "user-1")

		require.ErrorIs(t, err, boom)
	})
}
