package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

func TestAdminService_GetInstanceCounts(t *testing.T) {
	want := models.InstanceCounts{Users: 5, Teams: 2, Prompts: 9, Artifacts: 4, Memories: 7}
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetInstanceCounts", mock.Anything).Return(want, nil)

	got, err := NewAdminService(repo).GetInstanceCounts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestAdminService_GetInstanceCounts_Error(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetInstanceCounts", mock.Anything).Return(models.InstanceCounts{}, errors.New("boom"))

	_, err := NewAdminService(repo).GetInstanceCounts(context.Background())
	require.Error(t, err)
}
