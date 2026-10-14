package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The password notice says exactly what happened (#1436), and none of its
// forms carries a link to change the password.
func TestPasswordNoticeSaysWhatHappened(t *testing.T) {
	changedAt := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		kind    AccountChangeKind
		subject string
		says    string
		byAdmin bool
	}{
		{AccountChangeSet, "password was set", "A password was set for", false},
		{AccountChangeChanged, "password was changed", "was changed on", false},
		{AccountChangeReset, "password was reset", "was reset with a reset link on", false},
		{AccountChangeRemoved, "password was removed", "You sign in with your other sign-in methods", false},
		{AccountChangeRemovedByAdmin, "password was removed", "An administrator removed the password of", true},
		{AccountChangeDisabledByAdmin, "password was disabled", "wait for the reset link your administrator sends you", true},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			got := sendWith(t, acmeSiteInfo(), func(es *EmailService) error {
				return es.SendPasswordChanged(context.Background(), PasswordChangedNotice{
					To: "ada@example.com", Name: "Ada", Kind: tc.kind, ChangedAt: changedAt, CanResetByEmail: true,
				})
			})

			assert.Contains(t, got.subject, tc.subject)
			assert.Contains(t, got.text, tc.says)
			assert.Contains(t, got.html, tc.says)
			assert.NotContains(t, got.text, "#token=")
			assert.NotContains(t, got.html, "#token=")
			if tc.byAdmin {
				assert.Contains(t, got.text, "contact your administrator")
				assert.NotContains(t, got.text, "If this was you", "the owner did not do this")
			} else {
				assert.Contains(t, got.text, "If this was you, there is nothing to do.")
			}
		})
	}
}
