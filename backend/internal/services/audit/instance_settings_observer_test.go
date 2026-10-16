package audit

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

func TestInstanceSettingsObserver(t *testing.T) {
	admin := "admin-1"
	cases := []struct {
		name      string
		entry     models.InstanceSettingsAuditEntry
		op        models.AuditOperation
		actorType models.AuditActorType
		before    any
		after     any
	}{
		{
			name: "an instance setting changed by an admin",
			entry: models.InstanceSettingsAuditEntry{
				Setting: "search", Action: models.InstanceSettingsAuditActionUpsert, ActorUserID: &admin,
				Before: json.RawMessage(`{"min_score":0.2}`), After: json.RawMessage(`{"min_score":0.3}`),
			},
			op:     models.AuditOpInstanceSettingsUpdated,
			before: json.RawMessage(`{"min_score":0.2}`), after: json.RawMessage(`{"min_score":0.3}`),
		},
		{
			name: "the audit-log settings are their own operation",
			entry: models.InstanceSettingsAuditEntry{
				Setting: models.InstanceSettingAuditLog, Action: models.InstanceSettingsAuditActionDelete,
				ActorUserID: &admin, Before: json.RawMessage(`{"retention_weeks":4}`),
			},
			op: models.AuditOpAuditSettingsUpdated, before: json.RawMessage(`{"retention_weeks":4}`), after: nil,
		},
		{
			name: "no actor is the system (an import, the recovery CLI)",
			entry: models.InstanceSettingsAuditEntry{
				Setting: "auth_providers", Action: models.InstanceSettingsAuditActionImport,
				After: json.RawMessage(`{"slug":"google"}`),
			},
			op: models.AuditOpInstanceSettingsUpdated, actorType: models.AuditActorSystem,
			before: nil, after: json.RawMessage(`{"slug":"google"}`),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []Event
			observer := NewInstanceSettingsObserver(recorderFunc(func(_ context.Context, e Event) { got = append(got, e) }))

			observer.OnInstanceSettingsAudited(context.Background(), tc.entry)

			require.Len(t, got, 1)
			e := got[0]
			assert.Equal(t, tc.op, e.Operation)
			require.NotNil(t, e.Actor)
			assert.Equal(t, tc.actorType, e.Actor.Type)
			if tc.entry.ActorUserID != nil {
				assert.Equal(t, admin, *e.Actor.UserID)
			}
			assert.Equal(t, tc.entry.Setting, e.Metadata["setting"])
			assert.Equal(t, tc.entry.Action, e.Metadata["action"])
			assert.Equal(t, tc.before, e.Metadata["before"])
			assert.Equal(t, tc.after, e.Metadata["after"])
		})
	}
}

func TestInstanceSettingsObserver_PanickingRecorderIsContained(t *testing.T) {
	observer := NewInstanceSettingsObserver(recorderFunc(func(context.Context, Event) { panic("audit down") }))
	assert.NotPanics(t, func() {
		observer.OnInstanceSettingsAudited(context.Background(), models.InstanceSettingsAuditEntry{Setting: "search"})
	})
}
