package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Company branding in transactional email (#1362).

// noSiteInfo is an instance with nothing configured.
type noSiteInfo struct{}

func (noSiteInfo) Resolve(context.Context) models.InstanceSiteInfoValues {
	return models.InstanceSiteInfoValues{}
}

// fixedSiteInfo is an instance with the given values configured.
type fixedSiteInfo models.InstanceSiteInfoValues

func (f fixedSiteInfo) Resolve(context.Context) models.InstanceSiteInfoValues {
	return models.InstanceSiteInfoValues(f)
}

func acmeSiteInfo() fixedSiteInfo {
	return fixedSiteInfo{
		CompanyName:  "Acme Inc.",
		CompanyEmail: "hello@acme.test",
		Website:      "https://acme.test",
		SupportURL:   "https://acme.test/help",
		TermsURL:     "https://acme.test/terms",
		PrivacyURL:   "https://acme.test/privacy",
		LogoURL:      "https://cdn.acme.test/logo.png",
		LogoDarkURL:  "https://cdn.acme.test/logo-dark.png",
	}
}

// capturedEmail records the one message a send produced.
type capturedEmail struct {
	subject, html, text string
}

// sendWith runs send against an EmailService over siteInfo and returns the
// message it handed to the provider.
func sendWith(
	t *testing.T, siteInfo InstanceSiteInfoResolver, send func(es *EmailService) error,
) capturedEmail {
	t.Helper()
	provider := new(MockEmailProvider)
	var got capturedEmail
	provider.On("SendEmail", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		msg := args.Get(1).(*external.OutgoingMessage)
		got = capturedEmail{
			subject: msg.Message.GetSubject(), html: msg.Message.GetHTML(), text: msg.Message.GetText(),
		}
	}).Return(nil)

	resolver := instanceResolver(provider)
	resolver.identity = InstanceEmailIdentity{FromAddress: "test@example.com"}
	es := NewEmailService(resolver, siteInfo,
		&config.Config{Frontend: config.FrontendConfig{BaseURL: "https://app.example.com"}})
	require.NoError(t, send(es))
	provider.AssertNumberOfCalls(t, "SendEmail", 1)
	return got
}

func sendInvitation(es *EmailService) error {
	return es.SendTeamInvitation(context.Background(), "team-1", &models.TeamInvitation{
		InviteeEmail: "new@example.com", Token: "tok", Role: models.TeamMemberRoleMember,
		ExpiresAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	}, "Platform", "Ada")
}

func sendAcknowledgement(es *EmailService) error {
	return es.sendSupportAcknowledgement(context.Background(), "Ada Lovelace", "ada@example.com",
		&models.SupportRequest{Text: "Help", Acknowledgement: true})
}

func sendNotification(es *EmailService) error {
	return es.sendSupportNotificationToAdmin(context.Background(), "Ada", "ada@example.com",
		&models.SupportRequest{Text: "Help"})
}

// The mail library sanitizes HTML bodies, adding rel="nofollow" to every link.
const poweredByHTML = `Powered by <a href="https://vibexp.io" rel="nofollow">VibeXP</a>`

func TestEmailBranding_ConfiguredLayout(t *testing.T) {
	got := sendWith(t, acmeSiteInfo(), sendInvitation)

	assert.Contains(t, got.html, `<img src="https://cdn.acme.test/logo.png"`)
	assert.Contains(t, got.html, `alt="Acme Inc."`)
	assert.NotContains(t, got.html, "logo-dark", "a mail client cannot switch on theme")
	assert.Contains(t, got.html, "Acme Inc.. All rights reserved.")
	assert.Contains(t, got.html, `<a href="https://acme.test/help" rel="nofollow">Support</a>`)
	assert.Contains(t, got.html, `<a href="https://acme.test/terms" rel="nofollow">Terms of Service</a>`)
	assert.Contains(t, got.html, `<a href="https://acme.test/privacy" rel="nofollow">Privacy Policy</a>`,
		"the privacy link is the site info URL exactly as stored")
	assert.Contains(t, got.html, poweredByHTML)

	assert.Equal(t, "You have been invited to join Platform on Acme Inc.", got.subject)
	assert.Contains(t, got.html, "<strong>Platform</strong> on Acme Inc. as")
	assert.Contains(t, got.text, "Platform on Acme Inc. as a member")
	assert.Contains(t, got.text, "Acme Inc.. All rights reserved.")
	assert.Contains(t, got.text, "Powered by VibeXP - https://vibexp.io")
}

func TestEmailBranding_UnconfiguredLayoutFallsBackToVibeXP(t *testing.T) {
	got := sendWith(t, noSiteInfo{}, sendInvitation)

	assert.Contains(t, got.html, `<img src="https://vibexp.io/logo.png"`)
	assert.Contains(t, got.html, `alt="VibeXP"`)
	assert.Contains(t, got.html, "VibeXP. All rights reserved.")
	assert.NotContains(t, got.html, ">Support</a>")
	assert.NotContains(t, got.html, "Terms of Service")
	assert.NotContains(t, got.html, "Privacy Policy")
	assert.Contains(t, got.html, poweredByHTML)

	assert.Equal(t, "You have been invited to join Platform on VibeXP", got.subject)
	assert.Contains(t, got.text, "Platform on VibeXP as a member")
	assert.Contains(t, got.text, "Powered by VibeXP - https://vibexp.io")
}

// Each legal link appears only for the URL that is set, and support falls back
// to the company email.
func TestEmailBranding_FooterLinksAreIndependent(t *testing.T) {
	t.Run("support by email, terms only", func(t *testing.T) {
		got := sendWith(t, fixedSiteInfo{CompanyEmail: "hello@acme.test", TermsURL: "https://acme.test/terms"},
			sendInvitation)

		assert.Contains(t, got.html, `<a href="mailto:hello@acme.test" rel="nofollow">Support</a>`)
		assert.Contains(t, got.html, "Terms of Service")
		assert.NotContains(t, got.html, "Privacy Policy")
	})

	t.Run("privacy only, with a query string of its own", func(t *testing.T) {
		got := sendWith(t, fixedSiteInfo{PrivacyURL: "https://acme.test/legal?doc=privacy"}, sendInvitation)

		assert.Contains(t, got.html, `<a href="https://acme.test/legal?doc=privacy" rel="nofollow">Privacy Policy</a>`,
			"nothing is appended to the stored URL")
		assert.NotContains(t, got.html, "Terms of Service")
		assert.NotContains(t, got.html, ">Support</a>")
	})
}

func TestEmailBranding_SupportAcknowledgement(t *testing.T) {
	t.Run("configured", func(t *testing.T) {
		got := sendWith(t, acmeSiteInfo(), sendAcknowledgement)

		assert.Equal(t, "Thank you for contacting Acme Inc. Support", got.subject)
		assert.Contains(t, got.html, "Thank you for contacting Acme Inc. Support!")
		assert.Contains(t, got.html, poweredByHTML)
		assert.Contains(t, got.text, "Thank you for contacting Acme Inc. Support!")
		assert.Contains(t, got.text, "sent via the Acme Inc. support system")
		assert.Contains(t, got.text, "Powered by VibeXP - https://vibexp.io")
	})

	t.Run("unconfigured", func(t *testing.T) {
		got := sendWith(t, noSiteInfo{}, sendAcknowledgement)

		assert.Equal(t, "Thank you for contacting VibeXP Support", got.subject)
		assert.Contains(t, got.html, "Thank you for contacting VibeXP Support!")
		assert.Contains(t, got.text, "sent via the VibeXP support system")
	})
}

// The operator-facing support notification carries the same layout.
func TestEmailBranding_SupportNotification(t *testing.T) {
	got := sendWith(t, acmeSiteInfo(), sendNotification)

	assert.Contains(t, got.html, `alt="Acme Inc."`)
	assert.Contains(t, got.html, poweredByHTML)
	assert.Contains(t, got.text, "Powered by VibeXP - https://vibexp.io")
}

// Every email's footer carries the year it was sent in (#1375): the two
// support emails once passed a literal that went stale.
func TestEmailBranding_FooterCarriesCurrentYear(t *testing.T) {
	senders := map[string]func(es *EmailService) error{
		"team invitation":         sendInvitation,
		"support acknowledgement": sendAcknowledgement,
		"support notification":    sendNotification,
	}
	for name, send := range senders {
		t.Run(name, func(t *testing.T) {
			// Read before the send; the mail library's sanitizer decodes the
			// layout's &copy; entity, so both bodies carry the literal sign.
			footer := fmt.Sprintf("© %d Acme Inc.. All rights reserved.", time.Now().Year())
			got := sendWith(t, acmeSiteInfo(), send)

			assert.Contains(t, got.html, footer)
			assert.Contains(t, got.text, footer)
		})
	}
}

// The base layout renders through text/template, which escapes nothing, so
// admin-entered values are escaped explicitly wherever they land in HTML.
func TestEmailBranding_CompanyValuesAreHTMLEscaped(t *testing.T) {
	hostile := fixedSiteInfo{
		CompanyName: `Acme <script>alert(1)</script> "Inc"`,
		SupportURL:  `https://acme.test/help?a=1&b="x"`,
		TermsURL:    `https://acme.test/terms?x=<y>`,
		PrivacyURL:  `https://acme.test/privacy?p="q"`,
		LogoURL:     `https://cdn.acme.test/logo.png?v="1"`,
	}

	for name, send := range map[string]func(*EmailService) error{
		"invitation": sendInvitation, "acknowledgement": sendAcknowledgement,
	} {
		t.Run(name, func(t *testing.T) {
			got := sendWith(t, hostile, send)

			assert.NotContains(t, got.html, "<script>alert(1)</script>")
			assert.Contains(t, got.html, "Acme &lt;script&gt;alert(1)&lt;/script&gt; &#34;Inc&#34;")
			assert.NotContains(t, got.html, `b="x"`)
			assert.NotContains(t, got.html, "x=<y>")
			assert.NotContains(t, got.html, `p="q"`)
			assert.NotContains(t, got.html, `v="1"`)
		})
	}
}

// A company name with line breaks must not be able to add a header: the
// subject is always one line.
func TestEmailBranding_SubjectIsASingleLine(t *testing.T) {
	injected := fixedSiteInfo{CompanyName: "Acme\r\nBcc: evil@example.com"}

	for name, send := range map[string]func(*EmailService) error{
		"invitation": sendInvitation, "acknowledgement": sendAcknowledgement,
	} {
		t.Run(name, func(t *testing.T) {
			got := sendWith(t, injected, send)

			assert.False(t, strings.ContainsAny(got.subject, "\r\n"), "subject: %q", got.subject)
			assert.Contains(t, got.subject, "Acme Bcc: evil@example.com")
		})
	}
}

// failingSiteInfoRepo fails every read.
type failingSiteInfoRepo struct {
	repositories.InstanceSiteInfoRepository
}

func (failingSiteInfoRepo) Get(context.Context) (*models.InstanceSiteInfo, error) {
	return nil, errors.New("pq: connection reset by peer")
}

// A branding read failure never fails a send: the real service resolves to
// nothing configured, and the email goes out with VibeXP defaults.
func TestEmailBranding_SiteInfoReadFailureStillSends(t *testing.T) {
	siteInfo := NewInstanceSiteInfoService(failingSiteInfoRepo{}, slog.New(slog.DiscardHandler))

	got := sendWith(t, siteInfo, sendInvitation)

	assert.Equal(t, "You have been invited to join Platform on VibeXP", got.subject)
	assert.Contains(t, got.html, `alt="VibeXP"`)
	assert.NotContains(t, got.html, "connection reset")
}
