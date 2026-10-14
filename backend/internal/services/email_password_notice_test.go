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

// A password removed because a verified sign-in claimed the account (#1493)
// names the provider and the reason, points to Account settings, and carries
// no link to change the password.
func TestPasswordNoticeForAClaimNamesTheProviderAndTheReason(t *testing.T) {
	changedAt := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	got := sendWith(t, acmeSiteInfo(), func(es *EmailService) error {
		return es.SendPasswordChanged(context.Background(), PasswordChangedNotice{
			To: "ada@example.com", Name: "Ada", Kind: AccountChangeRemovedByClaim, Provider: "Corp SSO",
			ChangedAt: changedAt, CanResetByEmail: true,
		})
	})

	assert.Contains(t, got.subject, "password was removed")
	for name, body := range map[string]string{"text": got.text, "html": got.html} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, body, "Corp SSO")
			assert.Contains(t, body, "set before the address was verified")
			assert.Contains(t, body, "set a new one under Account settings")
			assert.Contains(t, body, "If you did not sign in through Corp SSO, contact your administrator.")
			assert.NotContains(t, body, "If this was you", "the owner did not change the password")
			assert.NotContains(t, body, "#token=")
			assert.Contains(t, body, "On "+changedAt.Format(accountNoticeTimeFormat))
		})
	}
}
