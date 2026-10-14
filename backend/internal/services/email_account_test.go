package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// The account emails (#1433): the setup link, the reset link and the
// password-changed notice.

const accountEmailTestLink = "https://app.example.com/account/reset#token=EXAMPLE-LINK-TOKEN"

func TestAccountLinkEmails(t *testing.T) {
	for name, tc := range map[string]struct {
		send        func(es *EmailService, msg AccountLinkMessage) error
		validFor    time.Duration
		subject     string
		heading     string
		validInText string
	}{
		"setup": {
			send: func(es *EmailService, msg AccountLinkMessage) error {
				return es.SendAccountSetup(context.Background(), msg)
			},
			validFor: 7 * 24 * time.Hour, subject: "Set up your Acme Inc. account",
			heading: "Set up your Acme Inc. account", validInText: "valid for 7 days",
		},
		"reset": {
			send: func(es *EmailService, msg AccountLinkMessage) error {
				return es.SendPasswordReset(context.Background(), msg)
			},
			validFor: time.Hour, subject: "Reset your Acme Inc. password",
			heading: "Reset your password", validInText: "valid for 1 hour",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := sendWith(t, acmeSiteInfo(), func(es *EmailService) error {
				return tc.send(es, AccountLinkMessage{
					To: "ada@example.com", Name: " Ada Lovelace ", LinkURL: accountEmailTestLink, ValidFor: tc.validFor,
				})
			})

			assert.Equal(t, tc.subject, got.subject, "the subject names the instance, from site info")
			for part, body := range map[string]string{"html": got.html, "text": got.text} {
				assert.Contains(t, body, tc.heading, part)
				assert.Contains(t, body, "Ada Lovelace", part)
				assert.Contains(t, body, "ada@example.com", part)
				assert.Contains(t, body, tc.validInText, part)
				assert.Contains(t, body, "works once", part)
				assert.Contains(t, body, accountEmailTestLink, part)
			}
			// The link once as a button and once as text in HTML, once in text;
			// the token nowhere else.
			token := strings.SplitN(accountEmailTestLink, "#token=", 2)[1]
			assert.Equal(t, 2, strings.Count(got.html, token), "button and plain address")
			assert.Equal(t, 1, strings.Count(got.text, token))
			assert.NotContains(t, got.subject, token)
		})
	}
}

// text/template does not escape. A display name is user-controlled and the
// branding is admin-entered, so the HTML part must escape both.
func TestAccountEmailsEscapeHostileValues(t *testing.T) {
	hostileName := `Ada <script>alert(1)</script> "quoted" & co`
	hostileBrand := fixedSiteInfo{CompanyName: `Acme <img src=x onerror=alert(2)>`}
	// A link is built by the server, but the template must not trust it either.
	hostileLink := `https://app.example.com/account/reset#token=abc"><script>alert(3)</script>`

	sends := map[string]func(es *EmailService) error{
		"setup": func(es *EmailService) error {
			return es.SendAccountSetup(context.Background(),
				AccountLinkMessage{To: "ada@example.com", Name: hostileName, LinkURL: hostileLink, ValidFor: time.Hour})
		},
		"reset": func(es *EmailService) error {
			return es.SendPasswordReset(context.Background(),
				AccountLinkMessage{To: "ada@example.com", Name: hostileName, LinkURL: hostileLink, ValidFor: time.Hour})
		},
		"password changed": func(es *EmailService) error {
			return es.SendPasswordChanged(context.Background(),
				PasswordChangedNotice{To: "ada@example.com", Name: hostileName, ChangedAt: time.Now()})
		},
	}
	for name, send := range sends {
		t.Run(name, func(t *testing.T) {
			got := sendWith(t, hostileBrand, send)

			assert.NotContains(t, got.html, "<script>", "no markup from a value reaches the HTML part")
			assert.NotContains(t, got.html, "<img src=x")
			assert.NotContains(t, got.html, `"><`)
			if name != "password changed" {
				assert.Contains(t, got.html, `#token=abc&#34;&gt;&lt;script&gt;`, "the link is escaped as well")
			}
			assert.Contains(t, got.html, "Ada &lt;script&gt;alert(1)&lt;/script&gt;")
			assert.Contains(t, got.html, "Acme &lt;img src=x onerror=alert(2)&gt;")
			// The text part is plain text: the name is there exactly as typed.
			assert.Contains(t, got.text, hostileName)
		})
	}
}

func TestPasswordChangedEmail(t *testing.T) {
	changedAt := time.Date(2026, 10, 14, 9, 5, 0, 0, time.FixedZone("CEST", 2*60*60))

	got := sendWith(t, acmeSiteInfo(), func(es *EmailService) error {
		return es.SendPasswordChanged(context.Background(), PasswordChangedNotice{
			To: "ada@example.com", Name: "Ada", ChangedAt: changedAt, CanResetByEmail: true,
		})
	})

	assert.Equal(t, "Your Acme Inc. password was changed", got.subject)
	for part, body := range map[string]string{"html": got.html, "text": got.text} {
		assert.Contains(t, body, "14 October 2026 at 07:05 UTC", part+": the time is written in UTC")
		assert.Contains(t, body, "ada@example.com", part)
		assert.Contains(t, body, "If it was not you", part)
		assert.Contains(t, body, "contact your administrator", part)
		assert.Contains(t, body, "Forgot password", part)
		assert.NotContains(t, body, "#token=", part+": the notice carries no link to change anything")
		assert.NotContains(t, body, "/account/reset", part)
		assert.NotContains(t, body, "/account/setup", part)
	}

	// On an instance where the sign-in page has no "Forgot password", the
	// notice does not point at one.
	without := sendWith(t, acmeSiteInfo(), func(es *EmailService) error {
		return es.SendPasswordChanged(context.Background(),
			PasswordChangedNotice{To: "ada@example.com", ChangedAt: changedAt})
	})
	assert.NotContains(t, without.html, "Forgot password")
	assert.NotContains(t, without.text, "Forgot password")
	assert.Contains(t, without.text, "contact your administrator")
	assert.Contains(t, without.text, "Hello,", "no name, no dangling space")
}

func TestDurationInWords(t *testing.T) {
	for d, want := range map[time.Duration]string{
		time.Hour:                       "1 hour",
		24 * time.Hour:                  "24 hours",
		7 * 24 * time.Hour:              "7 days",
		2 * 24 * time.Hour:              "2 days",
		36 * time.Hour:                  "36 hours",
		30 * time.Minute:                "30 minutes",
		time.Minute:                     "1 minute",
		59*time.Minute + 40*time.Second: "60 minutes",
	} {
		assert.Equal(t, want, durationInWords(d), d.String())
	}
}

// A link's lifetime reaches the email as the time left when it is sent, which
// is a hair under the nominal lifetime. The words must still be the nominal
// ones.
func TestAccountLinkMessageRoundsTheLifetime(t *testing.T) {
	issued := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	msg := accountLinkMessage(AccountLinkEmail{
		User: &models.User{Email: "ada@example.com", Name: "Ada"}, URL: accountEmailTestLink,
		ExpiresAt: issued.Add(time.Hour),
	}, issued.Add(450*time.Millisecond))
	assert.Equal(t, time.Hour, msg.ValidFor)
	assert.Equal(t, "ada@example.com", msg.To)

	require.Equal(t, "1 hour", durationInWords(roundLinkLifetime(time.Hour-300*time.Millisecond)))
	require.Equal(t, "7 days", durationInWords(roundLinkLifetime(7*24*time.Hour-2*time.Second)))
}
