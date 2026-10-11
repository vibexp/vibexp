package services

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Build is Record without the append (#1333): a writer whose repository stores
// the entry in the transaction of the change builds it first. These tests pin
// that it renders the same entry and refuses the same records as Record, and
// that it never reaches storage -- newTeamSettingsAuditService's repository
// mock fails the test on any call it was not told to expect.

func TestTeamSettingsAuditService_Build_RendersTheEntryWithoutStoringIt(t *testing.T) {
	svc, _ := newTeamSettingsAuditService(t)

	entry, err := svc.Build(TeamSettingsAuditRecord{
		TeamID:            "team-1",
		ActorUserID:       "user-1",
		Surface:           models.SettingsAuditSurfaceCustomStatuses,
		CreatedResourceID: "status-1",
		Detail:            map[string]interface{}{"action": "created", "slug": "in-review"},
	})

	require.NoError(t, err)
	assert.Equal(t, "team-1", entry.TeamID)
	assert.Equal(t, models.SettingsAuditSurfaceCustomStatuses, entry.Surface)
	require.NotNil(t, entry.ActorUserID)
	assert.Equal(t, "user-1", *entry.ActorUserID)
	require.NotNil(t, entry.CreatedResourceID)
	assert.Equal(t, "status-1", *entry.CreatedResourceID)
	assert.Nil(t, entry.SourceTeamID)
	assert.Nil(t, entry.SourceResourceID)
	assert.JSONEq(t, `{"action":"created","slug":"in-review"}`, string(entry.Detail))
	// The database assigns these on insert; Build leaves them unset.
	assert.Empty(t, entry.ID)
	assert.True(t, entry.CreatedAt.IsZero())
}

func TestTeamSettingsAuditService_Build_OmittedFieldsBecomeNil(t *testing.T) {
	svc, _ := newTeamSettingsAuditService(t)

	entry, err := svc.Build(TeamSettingsAuditRecord{
		TeamID: "team-1", Surface: models.SettingsAuditSurfaceCustomStatuses,
	})

	require.NoError(t, err)
	assert.Nil(t, entry.ActorUserID, "an absent actor is NULL, not an empty uuid")
	assert.Nil(t, entry.CreatedResourceID)
	assert.Nil(t, entry.Detail, "an empty detail is left for the repository to default")
}

func TestTeamSettingsAuditService_Build_AcceptsEverySurface(t *testing.T) {
	for _, surface := range models.SettingsAuditSurfaces {
		t.Run(surface, func(t *testing.T) {
			svc, _ := newTeamSettingsAuditService(t)

			record := TeamSettingsAuditRecord{TeamID: "team-1", Surface: surface}
			if models.IsSettingsAuditCopySurface(surface) {
				record.SourceTeamID = "team-2"
			}
			entry, err := svc.Build(record)

			require.NoError(t, err)
			assert.Equal(t, surface, entry.Surface)
		})
	}
}

// Build and Record must refuse exactly the same records for the same reason:
// each case is run through both, and Record never reaches the repository.
func TestTeamSettingsAuditService_Build_RejectsWhatRecordRejects(t *testing.T) {
	cases := map[string]struct {
		record TeamSettingsAuditRecord
		reason string
	}{
		"no team": {
			record: TeamSettingsAuditRecord{Surface: models.SettingsAuditSurfaceCustomStatuses},
			reason: "team is required",
		},
		"unknown surface": {
			record: TeamSettingsAuditRecord{TeamID: "team-1", Surface: "email_provider"},
			reason: "not an audited settings surface",
		},
		"empty surface": {
			record: TeamSettingsAuditRecord{TeamID: "team-1"},
			reason: "not an audited settings surface",
		},
		"a copy with no source team": {
			record: TeamSettingsAuditRecord{TeamID: "team-1", Surface: models.SettingsAuditSurfaceModelProvider},
			reason: "source team is required for a model_provider copy",
		},
		"a source team on a surface that is not a copy": {
			record: TeamSettingsAuditRecord{
				TeamID: "team-1", SourceTeamID: "team-2", Surface: models.SettingsAuditSurfaceCustomStatuses,
			},
			reason: "is not a copy and takes no source team",
		},
		"a detail that cannot be serialized": {
			record: TeamSettingsAuditRecord{
				TeamID: "team-1", Surface: models.SettingsAuditSurfaceCustomStatuses,
				Detail: map[string]interface{}{"ratio": math.Inf(1)},
			},
			reason: "not serializable",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc, _ := newTeamSettingsAuditService(t)

			built, buildErr := svc.Build(tc.record)
			require.ErrorIs(t, buildErr, ErrInvalidSettingsAudit)
			assert.Contains(t, buildErr.Error(), tc.reason)
			assert.Nil(t, built)

			recorded, recordErr := svc.Record(context.Background(), tc.record)
			require.Error(t, recordErr)
			assert.Equal(t, buildErr.Error(), recordErr.Error())
			assert.Nil(t, recorded)
		})
	}
}

// Record is Build followed by one append of the very entry Build returns.
func TestTeamSettingsAuditService_Record_AppendsWhatBuildRenders(t *testing.T) {
	svc, repo := newTeamSettingsAuditService(t)
	record := TeamSettingsAuditRecord{
		TeamID: "team-1", ActorUserID: "user-1", SourceTeamID: "team-2", SourceResourceID: "src-1",
		CreatedResourceID: "new-1", Surface: models.SettingsAuditSurfaceEmbeddingProvider,
		Detail: map[string]interface{}{"name": "OpenAI (copy)"},
	}
	built, err := svc.Build(record)
	require.NoError(t, err)

	var appended *models.TeamSettingsAudit
	repo.EXPECT().Append(mock.Anything, mock.Anything).
		Run(func(_ context.Context, entry *models.TeamSettingsAudit) { appended = entry }).
		Return(nil).Once()

	recorded, err := svc.Record(context.Background(), record)
	require.NoError(t, err)
	assert.Same(t, appended, recorded)
	assert.Equal(t, built, appended)
}
