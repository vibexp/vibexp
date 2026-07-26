package external

import (
	"testing"

	"github.com/darkrockmountain/gomail"
	"github.com/stretchr/testify/assert"
)

func messageFrom(address string) *gomail.EmailMessage {
	return gomail.NewFullEmailMessage(
		address,
		[]string{"recipient@example.test"},
		"Subject",
		nil, nil, "",
		"text body",
		"<p>html body</p>",
		nil,
	)
}

func TestOutgoingMessage_FromHeader(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		fromName string
		want     string
	}{
		{
			name:     "no display name yields the bare address",
			address:  "hello@acme.test",
			fromName: "",
			want:     "hello@acme.test",
		},
		{
			// Not "<hello@acme.test>", which is what mail.Address{Name: ""}
			// produces — that would change the bytes every provider has
			// received since before display names existed.
			name:     "whitespace-only display name yields the bare address",
			address:  "hello@acme.test",
			fromName: "   \t  ",
			want:     "hello@acme.test",
		},
		{
			name:     "plain name is quoted",
			address:  "hello@acme.test",
			fromName: "Acme Team",
			want:     `"Acme Team" <hello@acme.test>`,
		},
		{
			name:     "name is trimmed before encoding",
			address:  "hello@acme.test",
			fromName: "  Acme Team  ",
			want:     `"Acme Team" <hello@acme.test>`,
		},
		{
			name:     "comma is quoted rather than splitting the address list",
			address:  "hello@acme.test",
			fromName: "Acme, Inc.",
			want:     `"Acme, Inc." <hello@acme.test>`,
		},
		{
			name:     "double quote is escaped",
			address:  "hello@acme.test",
			fromName: `He said "hi"`,
			want:     `"He said \"hi\"" <hello@acme.test>`,
		},
		{
			name:     "non-ASCII is RFC 2047 encoded",
			address:  "hello@acme.test",
			fromName: "Ünïcodé Tëam",
			want:     "=?utf-8?q?=C3=9Cn=C3=AFcod=C3=A9_T=C3=ABam?= <hello@acme.test>",
		},
		{
			name:     "a name that looks like an address cannot displace the real one",
			address:  "hello@acme.test",
			fromName: "Team <fake@evil.test>",
			want:     `"Team <fake@evil.test>" <hello@acme.test>`,
		},
		{
			// gomail returns "" for an address its regex rejects. Producing
			// `"Name" <>` from that would be worse than producing nothing, so
			// the empty address short-circuits.
			name:     "an address gomail rejects yields empty, never a nameless bracket pair",
			address:  "not-an-address",
			fromName: "Acme Team",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &OutgoingMessage{Message: messageFrom(tt.address), FromName: tt.fromName}

			assert.Equal(t, tt.want, out.FromHeader())
		})
	}
}

// TestOutgoingMessage_FromHeaderCannotInjectAHeader is the security case: a team
// controls from_name, so it must not be able to append a header. net/mail
// base64-encodes any CR/LF into a single RFC 2047 encoded-word.
func TestOutgoingMessage_FromHeaderCannotInjectAHeader(t *testing.T) {
	for _, injected := range []string{
		"Evil\r\nBcc: attacker@evil.test",
		"Evil\nX-Injected: 1",
		"Evil\rX-Injected: 1",
		"Evil\r\n\r\nbody smuggled here",
	} {
		t.Run(injected, func(t *testing.T) {
			out := &OutgoingMessage{Message: messageFrom("hello@acme.test"), FromName: injected}

			header := out.FromHeader()

			assert.NotContains(t, header, "\r")
			assert.NotContains(t, header, "\n")
			assert.NotContains(t, header, "Bcc:")
			assert.NotContains(t, header, "X-Injected")
			assert.Contains(t, header, "<hello@acme.test>")
		})
	}
}

func TestOutgoingMessage_SanitizedFromName(t *testing.T) {
	tests := []struct {
		name     string
		fromName string
		want     string
	}{
		{name: "empty stays empty", fromName: "", want: ""},
		{name: "whitespace-only collapses to empty", fromName: " \t\n ", want: ""},
		{name: "surrounding whitespace is trimmed", fromName: "  Acme Team  ", want: "Acme Team"},
		{name: "inner whitespace is preserved", fromName: "Acme  Team", want: "Acme  Team"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &OutgoingMessage{Message: messageFrom("hello@acme.test"), FromName: tt.fromName}

			assert.Equal(t, tt.want, out.SanitizedFromName())
		})
	}
}
