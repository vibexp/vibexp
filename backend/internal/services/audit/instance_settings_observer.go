package audit

import (
	"context"
	"encoding/json"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// InstanceSettingsObserver records every durable instance settings change
// (#1515): it implements repositories.InstanceSettingsAuditObserver, which the
// repositories call after the change and its instance_settings_audit entry
// commit, so a rolled-back change never reaches the log. One hook covers every
// instance settings surface: email, search, AI summary, site info, the auth
// providers, allowlist and policy, instance-admin grants, setup tokens and the
// audit-log settings themselves.
type InstanceSettingsObserver struct {
	recorder Recorder
}

var _ repositories.InstanceSettingsAuditObserver = (*InstanceSettingsObserver)(nil)

// NewInstanceSettingsObserver creates the observer over recorder.
func NewInstanceSettingsObserver(recorder Recorder) *InstanceSettingsObserver {
	return &InstanceSettingsObserver{recorder: recorder}
}

// OnInstanceSettingsAudited records the change as instance.settings.updated,
// or audit.settings.updated for the audit-log settings. The snapshots are
// already redacted by the services that built them. An entry with no actor (a
// startup import, a setup token) is recorded as the system. The recovery CLI
// (vibexp admin auth) registers its own observer on its database handle, over a
// synchronous recorder wrapped in its Attribution (#1535).
func (o *InstanceSettingsObserver) OnInstanceSettingsAudited(
	ctx context.Context, entry models.InstanceSettingsAuditEntry,
) {
	op := models.AuditOpInstanceSettingsUpdated
	if entry.Setting == models.InstanceSettingAuditLog {
		op = models.AuditOpAuditSettingsUpdated
	}
	actor := &Actor{Type: models.AuditActorSystem}
	if entry.ActorUserID != nil && *entry.ActorUserID != "" {
		actor = &Actor{UserID: entry.ActorUserID}
	}
	SafeRecord(ctx, o.recorder, Event{
		Operation: op, Actor: actor,
		Metadata: map[string]any{
			"setting": entry.Setting, "action": entry.Action,
			"before": rawOrNil(entry.Before), "after": rawOrNil(entry.After),
		},
	})
}

// rawOrNil keeps an absent snapshot as JSON null rather than an empty
// (invalid) raw message.
func rawOrNil(doc []byte) any {
	if len(doc) == 0 {
		return nil
	}
	return json.RawMessage(doc)
}
