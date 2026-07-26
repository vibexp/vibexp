package implementations

import (
	"context"
	"errors"
	"net/smtp"
	"strings"
	"testing"

	"github.com/darkrockmountain/gomail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSMTPEmailProvider(t *testing.T) {
	tests := []struct {
		name        string
		spec        SMTPSpec
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid SMTP configuration",
			spec: SMTPSpec{
				Host:     "smtp.example.com",
				Port:     "587",
				Username: "test@example.com",
				Password: "password123",
			},
			expectError: false,
		},
		{
			name: "Invalid SMTP port - non-numeric",
			spec: SMTPSpec{
				Host:     "smtp.example.com",
				Port:     "invalid",
				Username: "test@example.com",
				Password: "password123",
			},
			expectError: true,
			errorMsg:    "invalid SMTP port",
		},
		{
			name: "Empty SMTP port",
			spec: SMTPSpec{
				Host:     "smtp.example.com",
				Port:     "",
				Username: "test@example.com",
				Password: "password123",
			},
			expectError: true,
			errorMsg:    "invalid SMTP port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewSMTPEmailProvider(tt.spec)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, provider)
			} else {
				require.NoError(t, err)
				require.NotNil(t, provider)

				// Verify it's the correct type
				smtpProvider, ok := provider.(*SMTPEmailProvider)
				require.True(t, ok, "Provider should be of type *SMTPEmailProvider")
				assert.Equal(t, "smtp.example.com:587", smtpProvider.addr)
			}
		})
	}
}

// capturedSend records what the provider handed to net/smtp.
type capturedSend struct {
	addr       string
	from       string
	recipients []string
	message    []byte
	err        error
}

// newCapturingProvider builds a provider whose transport is captured rather than
// dialled, so the assembled message bytes can be asserted directly.
func newCapturingProvider(t *testing.T) (*SMTPEmailProvider, *capturedSend) {
	t.Helper()

	built, err := NewSMTPEmailProvider(SMTPSpec{
		Host:     "smtp.example.test",
		Port:     "587",
		Username: "user@example.test",
		Password: "password",
	})
	require.NoError(t, err)

	provider, ok := built.(*SMTPEmailProvider)
	require.True(t, ok)

	captured := &capturedSend{}
	provider.sendMail = func(addr string, _ smtp.Auth, from string, to []string, msg []byte) error {
		captured.addr = addr
		captured.from = from
		captured.recipients = to
		captured.message = msg
		return captured.err
	}

	return provider, captured
}

// fromHeaderOf returns the value of the message's From header.
func fromHeaderOf(t *testing.T, message []byte) string {
	t.Helper()

	firstLine, _, found := strings.Cut(string(message), "\r\n")
	require.True(t, found, "message should contain CRLF-terminated headers")
	require.True(t, strings.HasPrefix(firstLine, "From: "), "first header should be From, got %q", firstLine)

	return strings.TrimPrefix(firstLine, "From: ")
}

// TestBuildMimeMessageStillLeadsWithFrom pins the gomail behaviour the display
// name depends on: BuildMimeMessage writes From as the very first line. If
// gomail changes that, replaceFromHeader silently stops applying the name — this
// test is what makes that visible instead.
func TestBuildMimeMessageStillLeadsWithFrom(t *testing.T) {
	mime, err := gomail.BuildMimeMessage(testMessage())
	require.NoError(t, err)

	assert.True(
		t,
		strings.HasPrefix(string(mime), "From: hello@acme.test\r\n"),
		"gomail.BuildMimeMessage must still lead with the From header; got %q",
		string(mime[:min(len(mime), 80)]),
	)
}

func TestSMTPEmailProvider_SendEmail_AppliesFromName(t *testing.T) {
	tests := []struct {
		name       string
		fromName   string
		wantHeader string
	}{
		{
			name:       "plain name is quoted",
			fromName:   "Acme Team",
			wantHeader: `"Acme Team" <hello@acme.test>`,
		},
		{
			name:       "comma is quoted rather than splitting the address list",
			fromName:   "Acme, Inc.",
			wantHeader: `"Acme, Inc." <hello@acme.test>`,
		},
		{
			name:       "double quote is escaped",
			fromName:   `He said "hi"`,
			wantHeader: `"He said \"hi\"" <hello@acme.test>`,
		},
		{
			name:       "non-ASCII is RFC 2047 encoded",
			fromName:   "Ünïcodé Tëam",
			wantHeader: "=?utf-8?q?=C3=9Cn=C3=AFcod=C3=A9_T=C3=ABam?= <hello@acme.test>",
		},
		{
			name:       "surrounding whitespace is trimmed",
			fromName:   "   Acme Team   ",
			wantHeader: `"Acme Team" <hello@acme.test>`,
		},
		{
			name:       "an address-looking name cannot displace the real address",
			fromName:   "Team <fake@evil.test>",
			wantHeader: `"Team <fake@evil.test>" <hello@acme.test>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, captured := newCapturingProvider(t)

			require.NoError(t, provider.SendEmail(
				context.Background(),
				outgoing(testMessage(), tt.fromName),
			))

			assert.Equal(t, tt.wantHeader, fromHeaderOf(t, captured.message))

			// The envelope sender stays bare no matter what the display name is,
			// so nothing that validates it sees a new value.
			assert.Equal(t, "hello@acme.test", captured.from)
			assert.Equal(t, "smtp.example.test:587", captured.addr)
		})
	}
}

// TestSMTPEmailProvider_SendEmail_NameCannotInjectAHeader is the security case:
// a team controls from_name, so it must not be able to append headers. net/mail
// base64-encodes any CR/LF into a single RFC 2047 word, which is what makes that
// impossible — assert it rather than trusting it.
func TestSMTPEmailProvider_SendEmail_NameCannotInjectAHeader(t *testing.T) {
	for _, injected := range []string{
		"Evil\r\nBcc: attacker@evil.test",
		"Evil\nX-Injected: 1",
	} {
		t.Run(injected, func(t *testing.T) {
			provider, captured := newCapturingProvider(t)

			require.NoError(t, provider.SendEmail(
				context.Background(),
				outgoing(testMessage(), injected),
			))

			header := fromHeaderOf(t, captured.message)
			assert.NotContains(t, header, "\r")
			assert.NotContains(t, header, "\n")
			assert.NotContains(t, string(captured.message), "Bcc: attacker@evil.test")
			assert.NotContains(t, string(captured.message), "X-Injected")
			assert.Equal(t, []string{"recipient@example.test"}, captured.recipients)
		})
	}
}

// TestSMTPEmailProvider_SendEmail_NoFromNameIsUnchanged pins the no-regression
// half: with no display name the bytes must be exactly what gomail builds, not
// mail.Address{Name: ""}.String()'s "<addr>" form.
func TestSMTPEmailProvider_SendEmail_NoFromNameIsUnchanged(t *testing.T) {
	for _, fromName := range []string{"", "   "} {
		t.Run("name="+fromName, func(t *testing.T) {
			provider, captured := newCapturingProvider(t)

			message := testMessage()
			require.NoError(t, provider.SendEmail(context.Background(), outgoing(message, fromName)))

			assert.Equal(t, "hello@acme.test", fromHeaderOf(t, captured.message))

			expected, err := gomail.BuildMimeMessage(message)
			require.NoError(t, err)
			assert.Equal(t, fromHeaderOf(t, expected), fromHeaderOf(t, captured.message))
		})
	}
}

func TestSMTPEmailProvider_SendEmail_EnvelopeIncludesCCAndBCC(t *testing.T) {
	provider, captured := newCapturingProvider(t)

	message := gomail.NewFullEmailMessage(
		"hello@acme.test",
		[]string{"to@example.test"},
		"Subject",
		[]string{"cc@example.test"},
		[]string{"bcc@example.test"},
		"",
		"text",
		"<p>html</p>",
		nil,
	)

	require.NoError(t, provider.SendEmail(context.Background(), outgoing(message, "Acme Team")))

	assert.Equal(
		t,
		[]string{"to@example.test", "cc@example.test", "bcc@example.test"},
		captured.recipients,
	)
	// BCC stays out of the headers — it is blind.
	assert.NotContains(t, string(captured.message), "bcc@example.test\r\n")
}

func TestSMTPEmailProvider_SendEmail_WrapsTransportError(t *testing.T) {
	provider, captured := newCapturingProvider(t)
	captured.err = errors.New("connection refused")

	err := provider.SendEmail(context.Background(), outgoing(testMessage(), "Acme Team"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "smtp: failed to send email")
	assert.ErrorIs(t, err, captured.err)
}

func TestSMTPEmailProvider_SendEmail_WrapsBuildError(t *testing.T) {
	provider, _ := newCapturingProvider(t)
	buildErr := errors.New("boom")
	provider.buildMIME = func(*gomail.EmailMessage) ([]byte, error) { return nil, buildErr }

	err := provider.SendEmail(context.Background(), outgoing(testMessage(), "Acme Team"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "smtp: failed to build the message")
	assert.ErrorIs(t, err, buildErr)
}

// TestReplaceFromHeader_LeavesUnrecognisedLayoutAlone covers the defensive
// branch: if gomail ever stops leading with From, the message is passed through
// untouched rather than gaining a corrupt or duplicate header.
func TestReplaceFromHeader_LeavesUnrecognisedLayoutAlone(t *testing.T) {
	original := []byte("Subject: hi\r\nFrom: hello@acme.test\r\n\r\nbody")

	assert.Equal(t, original, replaceFromHeader(original, `"Acme" <hello@acme.test>`))
	assert.Equal(t, []byte("no headers at all"), replaceFromHeader([]byte("no headers at all"), "x"))
}
