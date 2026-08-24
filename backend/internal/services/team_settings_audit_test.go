package services

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

func newTeamSettingsAuditService(t *testing.T) (*TeamSettingsAuditService, *mocks.MockTeamSettingsAuditRepository) {
	t.Helper()
	repo := mocks.NewMockTeamSettingsAuditRepository(t)
	return NewTeamSettingsAuditService(repo, slog.Default()), repo
}

func TestTeamSettingsAuditService_Record_ProviderCopy(t *testing.T) {
	svc, repo := newTeamSettingsAuditService(t)

	repo.EXPECT().Append(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, entry *models.TeamSettingsAudit) error {
			entry.ID = "audit-1"
			return nil
		})

	entry, err := svc.Record(context.Background(), TeamSettingsAuditRecord{
		TeamID:            "team-1",
		ActorUserID:       "user-1",
		Surface:           models.SettingsAuditSurfaceEmbeddingProvider,
		SourceTeamID:      "team-2",
		SourceResourceID:  "src-1",
		CreatedResourceID: "new-1",
		Detail:            map[string]interface{}{"name": "OpenAI (copy)"},
	})

	require.NoError(t, err)
	assert.Equal(t, "audit-1", entry.ID)
	assert.Equal(t, "team-1", entry.TeamID)
	require.NotNil(t, entry.SourceTeamID)
	assert.Equal(t, "team-2", *entry.SourceTeamID)
	require.NotNil(t, entry.CreatedResourceID)
	assert.Equal(t, "new-1", *entry.CreatedResourceID)
	assert.JSONEq(t, `{"name":"OpenAI (copy)"}`, string(entry.Detail))
}

// A custom-types copy produces many rows from one action, so it carries neither
// resource id. Both must reach the repository as nil rather than as an empty
// string, which the uuid columns would reject.
func TestTeamSettingsAuditService_Record_OmittedIDsBecomeNil(t *testing.T) {
	svc, repo := newTeamSettingsAuditService(t)

	repo.EXPECT().Append(mock.Anything, mock.Anything).Return(nil)

	entry, err := svc.Record(context.Background(), TeamSettingsAuditRecord{
		TeamID:       "team-1",
		Surface:      models.SettingsAuditSurfaceCustomTypes,
		SourceTeamID: "team-2",
	})

	require.NoError(t, err)
	assert.Nil(t, entry.SourceResourceID)
	assert.Nil(t, entry.CreatedResourceID)
	assert.Nil(t, entry.ActorUserID, "an absent actor is NULL, not an empty uuid")
	assert.Nil(t, entry.Detail, "an empty detail is left for the repository to default")
}

func TestTeamSettingsAuditService_Record_RejectsIncompleteEntries(t *testing.T) {
	cases := map[string]struct {
		record TeamSettingsAuditRecord
		reason string
	}{
		"no destination team": {
			record: TeamSettingsAuditRecord{
				Surface: models.SettingsAuditSurfaceModelProvider, SourceTeamID: "team-2",
			},
			reason: "destination team is required",
		},
		"no source team": {
			record: TeamSettingsAuditRecord{
				TeamID: "team-1", Surface: models.SettingsAuditSurfaceModelProvider,
			},
			reason: "source team is required",
		},
		"unknown surface": {
			record: TeamSettingsAuditRecord{
				TeamID: "team-1", SourceTeamID: "team-2", Surface: "email_provider",
			},
			reason: "not a copyable settings surface",
		},
		"empty surface": {
			record: TeamSettingsAuditRecord{TeamID: "team-1", SourceTeamID: "team-2"},
			reason: "not a copyable settings surface",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// No EXPECT: a rejected entry must never reach storage, and
			// mockery fails the test if Append is called anyway.
			svc, _ := newTeamSettingsAuditService(t)

			entry, err := svc.Record(context.Background(), tc.record)

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidSettingsAudit)
			assert.Contains(t, err.Error(), tc.reason)
			assert.Nil(t, entry)
		})
	}
}

// Every surface constant must be accepted — the allowlist and the constants are
// two lists that can drift apart.
func TestTeamSettingsAuditService_Record_AcceptsEverySurface(t *testing.T) {
	for _, surface := range models.SettingsAuditSurfaces {
		t.Run(surface, func(t *testing.T) {
			svc, repo := newTeamSettingsAuditService(t)
			repo.EXPECT().Append(mock.Anything, mock.Anything).Return(nil)

			entry, err := svc.Record(context.Background(), TeamSettingsAuditRecord{
				TeamID: "team-1", SourceTeamID: "team-2", Surface: surface,
			})

			require.NoError(t, err)
			assert.Equal(t, surface, entry.Surface)
		})
	}
}

func TestTeamSettingsAuditService_Record_UnserializableDetail(t *testing.T) {
	svc, _ := newTeamSettingsAuditService(t)

	entry, err := svc.Record(context.Background(), TeamSettingsAuditRecord{
		TeamID:       "team-1",
		SourceTeamID: "team-2",
		Surface:      models.SettingsAuditSurfaceModelProvider,
		Detail:       map[string]interface{}{"ratio": math.Inf(1)},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidSettingsAudit)
	assert.Contains(t, err.Error(), "not serializable")
	assert.Nil(t, entry)
}

// A storage failure must surface. The entry is the compensating control for a
// copy that moved a credential's use into a different set of members, so a
// caller that swallowed this would report a copy nothing recorded.
func TestTeamSettingsAuditService_Record_StorageError(t *testing.T) {
	svc, repo := newTeamSettingsAuditService(t)

	repo.EXPECT().Append(mock.Anything, mock.Anything).Return(errors.New("append boom"))

	entry, err := svc.Record(context.Background(), TeamSettingsAuditRecord{
		TeamID: "team-1", SourceTeamID: "team-2",
		Surface: models.SettingsAuditSurfaceModelProvider,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to record team settings audit entry")
	assert.Contains(t, err.Error(), "append boom")
	assert.Nil(t, entry)
}
