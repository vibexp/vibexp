package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/darkrockmountain/gomail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// What a provider receives (#1470). Every email is sent exactly as rendered:
// the mail library's own sanitizer used to strip the styles from the HTML
// part and HTML-escape the text part and the subject. These tests read the
// message the way a provider does (GetHTML, GetText, GetSubject), so they see
// what is delivered and not what was rendered.

const (
	markupName  = `Ada <script>alert(1)</script> "the & one" O'Brien`
	markupTeam  = `R&D <img src=x onerror=alert(2)> "Core"`
	markupBrand = `Acme <b>&</b> Co`
)

// everyEmail sends one of each email this service renders from a template,
// with hostile values wherever a user or an administrator supplies text.
func everyEmail() map[string]func(es *EmailService) error {
	ctx := context.Background()
	link := AccountLinkMessage{
		To: "ada@example.com", Name: markupName, ValidFor: time.Hour,
		LinkURL: "https://app.example.com/account/reset#token=EXAMPLE-LINK-TOKEN",
	}
	return map[string]func(es *EmailService) error{
		"team invitation": func(es *EmailService) error {
			return es.SendTeamInvitation(ctx, "team-1", &models.TeamInvitation{
				InviteeEmail: "new@example.com", Token: "tok", Role: models.TeamMemberRoleMember,
				ExpiresAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			}, markupTeam, markupName)
		},
		"support notification": func(es *EmailService) error {
			return es.sendSupportNotificationToAdmin(ctx, markupName, "ada@example.com", &models.SupportRequest{
				Text:           markupTeam,
				AdditionalInfo: map[string]string{`key <i>"k"</i>`: markupName},
			})
		},
		"support acknowledgement": func(es *EmailService) error {
			return es.sendSupportAcknowledgement(ctx, markupName, "ada@example.com",
				&models.SupportRequest{Text: markupTeam, Acknowledgement: true})
		},
		"account setup":  func(es *EmailService) error { return es.SendAccountSetup(ctx, link) },
		"password reset": func(es *EmailService) error { return es.SendPasswordReset(ctx, link) },
		"password changed": func(es *EmailService) error {
			return es.SendPasswordChanged(ctx,
				PasswordChangedNotice{To: "ada@example.com", Name: markupName, ChangedAt: time.Now()})
		},
	}
}

// The layout's styles and classes reach the provider. Without them the email
// is unstyled text and the button is a plain link.
func TestEmailsAreDeliveredWithTheirStyling(t *testing.T) {
	for name, send := range everyEmail() {
		t.Run(name, func(t *testing.T) {
			got := sendWith(t, acmeSiteInfo(), send)

			assert.Contains(t, got.html, "<style", "the layout's style block is delivered")
			assert.Contains(t, got.html, `class="`, "and so are the classes it styles")
			assert.Contains(t, got.html, "<!DOCTYPE html>", "the document is delivered whole")
		})
	}
	// The button of an email that has one keeps its class.
	for _, name := range []string{"team invitation", "account setup", "password reset"} {
		got := sendWith(t, acmeSiteInfo(), everyEmail()[name])
		assert.Contains(t, got.html, `class="button"`, name)
	}
}

// Markup in a name, a team name, a support message or the branding arrives
// inert in the HTML part, and exactly as typed in the text part.
func TestEmailsDeliverHostileValuesInertInHTMLAndVerbatimInText(t *testing.T) {
	brand := fixedSiteInfo{CompanyName: markupBrand}
	for name, send := range everyEmail() {
		t.Run(name, func(t *testing.T) {
			got := sendWith(t, brand, send)

			// HTML: no value brings markup of its own.
			for _, live := range []string{"<script>", "<img src=x", "<b>&</b>", "onerror=alert(2)>", `<i>"k"</i>`} {
				assert.NotContains(t, got.html, live)
			}
			// Each value that this email prints is there, escaped once.
			assert.NotContains(t, got.html, "&amp;lt;", "nothing is escaped twice")
			assert.NotContains(t, got.html, "&amp;amp;")
			assert.NotContains(t, got.html, "&amp;#34;")
			if strings.Contains(got.text, "Ada ") {
				assert.Contains(t, got.html, "Ada &lt;script&gt;alert(1)&lt;/script&gt;")
			}
			assert.Contains(t, got.html, "Acme &lt;b&gt;&amp;&lt;/b&gt; Co")

			// Text: plain text, as typed. No entities.
			assert.NotContains(t, got.text, "&lt;")
			assert.NotContains(t, got.text, "&#34;")
			assert.NotContains(t, got.text, "&#39;")
			assert.NotContains(t, got.text, "&amp;")
			assert.Contains(t, got.text, markupBrand)
		})
	}

	t.Run("the values appear verbatim in the text part", func(t *testing.T) {
		invitation := sendWith(t, brand, everyEmail()["team invitation"])
		assert.Contains(t, invitation.text, markupName)
		assert.Contains(t, invitation.text, markupTeam)

		reset := sendWith(t, brand, everyEmail()["password reset"])
		assert.Contains(t, reset.text, "Hello "+markupName+",")

		support := sendWith(t, brand, everyEmail()["support notification"])
		assert.Contains(t, support.text, markupTeam)
		assert.Contains(t, support.text, `key <i>"k"</i>: `+markupName)
		assert.Contains(t, support.html, "key &lt;i&gt;&#34;k&#34;&lt;/i&gt;:")
	})
}

// A subject is text. It carries no entities, and a value in it cannot end the
// header line. A team name reaches the invitation's subject raw (branding is
// already made one line when it is stored), so the injection is tried there.
// The delivered subject is checked exactly; the header block is then checked
// on a message built from it the way the SMTP provider builds one
// (gomail.BuildMimeMessage), which pins that one cleaned line yields one header.
func TestEmailSubjectsCannotInjectAHeader(t *testing.T) {
	got := sendWith(t, acmeSiteInfo(), func(es *EmailService) error {
		return es.SendTeamInvitation(context.Background(), "team-1", &models.TeamInvitation{
			InviteeEmail: "new@example.com", Token: "tok", Role: models.TeamMemberRoleMember,
			ExpiresAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		}, "R&D\r\nBcc: x@evil.test\nX-Evil: 1", "Ada")
	})

	assert.Equal(t, "You have been invited to join R&D Bcc: x@evil.test X-Evil: 1 on Acme Inc.", got.subject)
	msg := newEmailMessage("from@example.com", "to@example.com", got.subject, "", got.text, got.html)
	built, err := gomail.BuildMimeMessage(msg)
	require.NoError(t, err)
	headers, _, _ := strings.Cut(string(built), "\r\n\r\n")
	subjects := 0
	for _, line := range strings.Split(headers, "\r\n") {
		assert.False(t, strings.HasPrefix(line, "Bcc:"), "no injected Bcc header: %q", line)
		assert.False(t, strings.HasPrefix(line, "X-Evil:"), "no injected header: %q", line)
		if strings.HasPrefix(line, "Subject:") {
			subjects++
		}
	}
	assert.Equal(t, 1, subjects)
	assert.Contains(t, string(built), "<style", "and the delivered message still carries its styles")
}

func TestEmailSubjectsAreTextAndOneLine(t *testing.T) {
	brand := fixedSiteInfo{CompanyName: "R&D \"Labs\"\r\nBcc: attacker@evil.test"}

	got := sendWith(t, brand, everyEmail()["password reset"])

	assert.Equal(t, `Reset your R&D "Labs" Bcc: attacker@evil.test password`, got.subject)
	assert.NotContains(t, got.subject, "\r")
	assert.NotContains(t, got.subject, "\n")
	assert.NotContains(t, got.subject, "&amp;")

	invitation := sendWith(t, acmeSiteInfo(), everyEmail()["team invitation"])
	assert.Contains(t, invitation.subject, markupTeam, "a team name is printed as typed")
}

func TestCleanEmailSubject(t *testing.T) {
	for in, want := range map[string]string{
		"Plain subject":                        "Plain subject",
		"  padded  ":                           "padded",
		"line one\r\nBcc: x@evil.test":         "line one Bcc: x@evil.test",
		"tab\tand\x00nul\x1b[31m escape":       "tab and nul [31m escape",
		"R&D <team> \"quoted\" O'Brien":        "R&D <team> \"quoted\" O'Brien",
		"Ünïcödé — ok":                         "Ünïcödé — ok",
		"new\u2028line\u0085separators\u2029x": "new line separators x",
	} {
		assert.Equal(t, want, cleanEmailSubject(in), "%q", in)
	}
}

// A URL from the branding lands in an href. One that is not a web address
// does not survive as a live link.
func TestEmailBrandingURLsCannotCarryScript(t *testing.T) {
	brand := fixedSiteInfo{
		CompanyName: "Acme", SupportURL: "javascript:alert(1)", TermsURL: `https://acme.test/terms?a=1&b="2"`,
		LogoURL: "javascript:alert(3)",
	}

	got := sendWith(t, brand, everyEmail()["password reset"])

	assert.NotContains(t, got.html, "javascript:")
	assert.Contains(t, got.html, `href="https://acme.test/terms?a=1&amp;b=%222%22"`,
		"a web address is kept, escaped for the attribute it is in")
}

// The message constructor is the one place the library's sanitizers are
// replaced; the provider settings test send goes through it too.
func TestNewEmailMessageSendsBodiesAsGiven(t *testing.T) {
	html := `<html><head><style>.b{color:red}</style></head><body><a class="b" href="https://x.test/?a=1&amp;b=2">x</a></body></html>`
	text := `He said "hi" & left <now>` + "\n\n  indented"

	msg := newEmailMessage("from@example.com", "to@example.com", "Sub\r\nject", "reply@example.com", text, html)

	assert.Equal(t, html, msg.GetHTML())
	assert.Equal(t, text, msg.GetText())
	assert.Equal(t, "Sub ject", msg.GetSubject())
	assert.Equal(t, "from@example.com", msg.GetFrom())
	assert.Equal(t, []string{"to@example.com"}, msg.GetTo())
	assert.Equal(t, "reply@example.com", msg.GetReplyTo())

	test := testMessageFor(testSender{FromAddress: "from@example.com"}, "to@example.com")
	require.NotNil(t, test.Message)
	assert.Equal(t, testEmailBodyHTML, test.Message.GetHTML(), "the test send is delivered as written too")
	assert.Equal(t, testEmailBodyText, test.Message.GetText())
}

// A caller that brings its own rendered HTML (the notification digest) gets it
// delivered as it is: its styles survive too, and nothing in it is rewritten.
func TestSendNotificationEmailDeliversAReadyMadeBodyUnchanged(t *testing.T) {
	body := `<html><head><style>.digest{color:#111}</style></head>` +
		`<body><div class="digest">3 new posts in R&amp;D &lt;Core&gt;</div></body></html>`

	got := sendWith(t, acmeSiteInfo(), func(es *EmailService) error {
		return es.SendNotificationEmail(context.Background(), "", "ada@example.com", `Your digest - R&D "Core"`, body)
	})

	assert.Equal(t, body, got.html)
	assert.Equal(t, `Your digest - R&D "Core"`, got.subject)
	assert.Empty(t, got.text)
}
