package services

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// Team settings changes in the instance audit log (#1520).

const settingsAuditActor = "actor-1"

// requireTeamSettingsEvent asserts events holds exactly one team.settings.updated
// event for surface/action in team, and returns its metadata.
func requireTeamSettingsEvent(
	t *testing.T, events *auditEvents, teamID, surface, action string,
) map[string]any {
	t.Helper()
	e := events.only(t)
	assert.Equal(t, models.AuditOpTeamSettingsUpdated, e.Operation)
	assert.Equal(t, models.AuditResourceTypeTeamSettings, e.ResourceType)
	assert.Equal(t, teamSettingsSurfaceLabels[surface], e.ResourceName)
	assert.Equal(t, teamID, e.TeamID)
	assert.Equal(t, surface, e.Metadata[teamSettingsMetaSurface])
	assert.Equal(t, action, e.Metadata[teamSettingsMetaAction])
	return e.Metadata
}

// changedFields reads the changed_fields list of an event's metadata.
func changedFields(t *testing.T, metadata map[string]any) []string {
	t.Helper()
	fields, ok := metadata[teamSettingsMetaChangedFields].([]string)
	require.True(t, ok, "changed_fields is a string list")
	return fields
}

// assertNoSecretLeak fails if the serialized event carries any substring of a
// secret longer than 4 characters: not the value, not a fragment of it.
func assertNoSecretLeak(t *testing.T, e audit.Event, secrets ...string) {
	t.Helper()
	encoded, err := json.Marshal(e)
	require.NoError(t, err)
	text := string(encoded)
	for _, secret := range secrets {
		for i := 0; i+5 <= len(secret); i++ {
			require.NotContains(t, text, secret[i:i+5], "the audit event must not carry the secret %q", secret)
		}
	}
}

func TestTeamSettingsChangeMetadata_UpdateKeepsOnlyChangedFields(t *testing.T) {
	metadata := teamSettingsChangeMetadata(teamSettingsChange{
		Surface: teamSettingsSurfaceSearch, Action: teamSettingsActionUpdate,
		Before: map[string]any{"enabled": true, "weight": 0.5, "name": "a"},
		After:  map[string]any{"enabled": true, "weight": 0.7, "name": "b"},
	})

	assert.Equal(t, []string{"name", "weight"}, metadata[teamSettingsMetaChangedFields])
	assert.Equal(t, map[string]any{"old": 0.5, "new": 0.7}, metadata["weight"])
	assert.Equal(t, map[string]any{"old": "a", "new": "b"}, metadata["name"])
	assert.NotContains(t, metadata, "enabled", "an unchanged field is not recorded")
}

func TestTeamSettingsChangeMetadata_OneSidedChanges(t *testing.T) {
	created := teamSettingsChangeMetadata(teamSettingsChange{
		Action: teamSettingsActionCreate, After: map[string]any{"name": "x"},
	})
	assert.Equal(t, []string{"name"}, created[teamSettingsMetaChangedFields])
	assert.Equal(t, map[string]any{"new": "x"}, created["name"])

	deleted := teamSettingsChangeMetadata(teamSettingsChange{
		Action: teamSettingsActionDelete, Before: map[string]any{"name": "x"},
	})
	assert.Equal(t, []string{"name"}, deleted[teamSettingsMetaChangedFields])
	assert.Equal(t, map[string]any{"old": "x"}, deleted["name"])

	// A key present on one side only of an update is a change too.
	moved := teamSettingsChangeMetadata(teamSettingsChange{
		Before: map[string]any{"host": "a"}, After: map[string]any{"domain": "b"},
	})
	assert.Equal(t, []string{"domain", "host"}, moved[teamSettingsMetaChangedFields])

	nothing := teamSettingsChangeMetadata(teamSettingsChange{Action: teamSettingsActionReset})
	assert.Equal(t, []string{}, nothing[teamSettingsMetaChangedFields], "never null")
}

func TestTeamSettingsChangeMetadata_WithheldFieldsAreNamedWithoutValues(t *testing.T) {
	metadata := teamSettingsChangeMetadata(teamSettingsChange{
		Before:   map[string]any{"name": "a"},
		After:    map[string]any{"name": "a"},
		Withheld: []string{"api_key", "configuration"},
	})

	assert.Equal(t, []string{"api_key", "configuration"}, metadata[teamSettingsMetaChangedFields])
	assert.NotContains(t, metadata, "api_key")
	assert.NotContains(t, metadata, "configuration")
}

func TestTeamSettingsChangeMetadata_NonScalarsAreNamedOnly(t *testing.T) {
	value := "v"
	metadata := teamSettingsChangeMetadata(teamSettingsChange{
		Before: map[string]any{"list": []string{"a"}, "ptr": (*string)(nil)},
		After:  map[string]any{"list": []string{"b"}, "ptr": &value},
	})

	assert.Equal(t, []string{"list", "ptr"}, metadata[teamSettingsMetaChangedFields])
	assert.NotContains(t, metadata, "list", "a list is never written out")
	assert.Equal(t, map[string]any{"old": nil, "new": "v"}, metadata["ptr"], "pointers are dereferenced")
}

func TestTeamSettingsChangeMetadata_DetailsWinOverAFieldOfTheSameName(t *testing.T) {
	metadata := teamSettingsChangeMetadata(teamSettingsChange{
		Surface: teamSettingsSurfaceSearch, Action: teamSettingsActionUpdate,
		After:   map[string]any{"surface": "spoofed", "rule_id": "field"},
		Details: map[string]any{"rule_id": "rule-1"},
	})

	assert.Equal(t, teamSettingsSurfaceSearch, metadata[teamSettingsMetaSurface])
	assert.Equal(t, "rule-1", metadata["rule_id"])
}

func TestTeamSettingsAuditor_RecordsOneEventAndIsANoOpWithoutARecorder(t *testing.T) {
	change := teamSettingsChange{
		TeamID: "team-1", ActorID: settingsAuditActor,
		Surface: teamSettingsSurfaceFreshness, Action: teamSettingsActionReset,
	}
	teamSettingsAuditor{}.record(context.Background(), change) // must not panic
	assert.False(t, teamSettingsAuditor{}.enabled())

	events := &auditEvents{}
	auditor := teamSettingsAuditor{recorder: events}
	assert.True(t, auditor.enabled())
	auditor.record(context.Background(), change)

	requireTeamSettingsEvent(t, events, "team-1", teamSettingsSurfaceFreshness, teamSettingsActionReset)
	assert.Equal(t, settingsAuditActor, *events.only(t).Actor.UserID)
}

func TestTeamSettingsAuditor_EveryRecorderOptionSetsTheRecorder(t *testing.T) {
	events := &auditEvents{}
	auditors := map[string]teamSettingsAuditor{
		"search": NewTeamSearchSettingsService(nil, nil, nil, nil,
			WithTeamSearchSettingsAuditRecorder(events)).teamSettingsAuditor,
		"ai_summary": NewTeamAISummarySettingsService(nil, nil, nil, nil, nil,
			WithTeamAISummarySettingsAuditRecorder(events)).teamSettingsAuditor,
		"email_provider": NewTeamEmailProviderService(TeamEmailProviderServiceDeps{
			Repo:          nil,
			InstanceRepo:  nil,
			UserRepo:      nil,
			Enc:           nil,
			Config:        localDevProviderConfig(),
			Authz:         nil,
			Logger:        nil,
			AuditRecorder: events,
		}).teamSettingsAuditor,
		"model_provider": NewModelProviderService(nil, nil, localDevProviderConfig(), nil, nil,
			WithModelProviderAuditRecorder(events)).teamSettingsAuditor,
		"embedding_provider": NewEmbeddingProviderService(nil, nil, localDevProviderConfig(), nil, nil, nil,
			WithEmbeddingProviderAuditRecorder(events)).teamSettingsAuditor,
		"freshness": NewFreshnessService(FreshnessServiceDeps{AuditRecorder: events}).teamSettingsAuditor,
		"github_app_config": NewGitHubAppConfigService(nil, nil, nil, nil, "",
			WithGitHubAppConfigAuditRecorder(events)).teamSettingsAuditor,
		"github_app": NewGitHubAppService(GitHubAppServiceDeps{AuditRecorder: events}).(*GitHubAppService).
			teamSettingsAuditor,
		"custom_types": NewTypeService(nil, nil, nil, nil, WithTypeAuditRecorder(events)).teamSettingsAuditor,
		"custom_statuses": NewStatusManagementService(nil, nil, nil, nil, nil,
			WithStatusManagementAuditRecorder(events)).teamSettingsAuditor,
	}
	for name, auditor := range auditors {
		assert.Same(t, events, auditor.recorder, name)
	}
}
