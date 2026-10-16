package audit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// recordingRecorder keeps every event it is handed.
type recordingRecorder struct{ events []Event }

func (r *recordingRecorder) Record(_ context.Context, e Event) { r.events = append(r.events, e) }

func TestAttributedRecorder(t *testing.T) {
	cli := Attribution{
		Source:   models.AuditSourceCLI,
		Actor:    Actor{Type: models.AuditActorSystem, Name: "recovery-cli"},
		Metadata: map[string]any{"os_user": "root", "hostname": "box", "setting": "ignored"},
	}

	t.Run("stamps source, actor and metadata, and the event's own keys win", func(t *testing.T) {
		next := &recordingRecorder{}
		own := map[string]any{"setting": "auth_providers"}
		userID := testUserID

		NewAttributedRecorder(next, cli).Record(context.Background(), Event{
			Operation: models.AuditOpInstanceSettingsUpdated,
			Source:    models.AuditSourceWeb,
			Actor:     &Actor{UserID: &userID},
			Metadata:  own,
		})

		require.Len(t, next.events, 1)
		got := next.events[0]
		assert.Equal(t, models.AuditSourceCLI, got.Source)
		assert.Equal(t, &Actor{Type: models.AuditActorSystem, Name: "recovery-cli"}, got.Actor)
		assert.Equal(t, map[string]any{"os_user": "root", "hostname": "box", "setting": "auth_providers"},
			got.Metadata)
		assert.Equal(t, map[string]any{"setting": "auth_providers"}, own, "the caller's map is not modified")
	})

	t.Run("an empty source and no metadata keep the event's own", func(t *testing.T) {
		next := &recordingRecorder{}

		NewAttributedRecorder(next, Attribution{Actor: Actor{Type: models.AuditActorSystem}}).
			Record(context.Background(), Event{Operation: models.AuditOpAuditExport, Source: models.AuditSourceAPI})

		require.Len(t, next.events, 1)
		assert.Equal(t, models.AuditSourceAPI, next.events[0].Source)
		assert.Nil(t, next.events[0].Metadata)
	})
}
