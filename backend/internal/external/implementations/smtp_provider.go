package implementations

import (
	"bytes"
	"context"
	"fmt"
	"net/smtp"
	"strconv"

	"github.com/darkrockmountain/gomail"

	"github.com/vibexp/vibexp/internal/external"
)

// fromHeaderPrefix is the start of the From header line gomail.BuildMimeMessage
// writes as the very first line of every message it builds.
const fromHeaderPrefix = "From: "

// crlf terminates each header line in a MIME message.
var crlf = []byte("\r\n")

// SMTPEmailProvider implements external.EmailProvider by assembling the MIME
// message with gomail and handing it to net/smtp.
//
// It does NOT use gomail's own SMTP sender, and cannot: that sender calls
// gomail.BuildMimeMessage, which writes `From: <addr>` from the message's
// validated bare address, and gomail.EmailMessage exposes no custom-header
// field. There is therefore no supported way to give it a display name — the
// only way to put one in the From header is to own the message bytes here
// (#549). The transport is otherwise the same call gomail's implicit-connection
// path makes: net/smtp.SendMail with PLAIN auth.
type SMTPEmailProvider struct {
	addr      string
	auth      smtp.Auth
	buildMIME func(*gomail.EmailMessage) ([]byte, error)
	// sendMail is net/smtp.SendMail in production, replaced in tests so the
	// assembled bytes can be asserted without a live SMTP server.
	sendMail func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

// NewSMTPEmailProvider creates a new SMTP email provider.
func NewSMTPEmailProvider(spec SMTPSpec) (external.EmailProvider, error) {
	port, err := strconv.Atoi(spec.Port)
	if err != nil {
		return nil, fmt.Errorf("invalid SMTP port: %w", err)
	}

	return &SMTPEmailProvider{
		addr: fmt.Sprintf("%s:%d", spec.Host, port),
		// Built unconditionally, matching what gomail did — a server that does
		// not advertise AUTH fails the same way it did before.
		auth:      smtp.PlainAuth("", spec.Username, spec.Password, spec.Host),
		buildMIME: gomail.BuildMimeMessage,
		sendMail:  smtp.SendMail,
	}, nil
}

// SendEmail assembles the message and delivers it over SMTP.
//
// The envelope sender stays the bare address — only the From header carries the
// display name — so nothing that validates the envelope sees a different value
// than before.
func (p *SMTPEmailProvider) SendEmail(_ context.Context, outgoing *external.OutgoingMessage) error {
	message := outgoing.Message

	mime, err := p.buildMIME(message)
	if err != nil {
		return fmt.Errorf("smtp: failed to build the message: %w", err)
	}

	if from := outgoing.FromHeader(); from != message.GetFrom() {
		mime = replaceFromHeader(mime, from)
	}

	// gomail puts CC and BCC recipients in the envelope but not the header,
	// which is what makes BCC blind; keep that behaviour.
	recipients := append([]string{}, message.GetTo()...)
	recipients = append(recipients, message.GetCC()...)
	recipients = append(recipients, message.GetBCC()...)

	if err := p.sendMail(p.addr, p.auth, message.GetFrom(), recipients, mime); err != nil {
		return fmt.Errorf("smtp: failed to send email: %w", err)
	}

	return nil
}

// replaceFromHeader swaps the value of the From header in a message built by
// gomail.BuildMimeMessage, which always writes it as the first line.
//
// If the first line is not a From header — gomail changed its layout — the
// message is returned untouched, so the display name is dropped rather than a
// header being corrupted or duplicated. TestBuildMimeMessageStillLeadsWithFrom
// fails when that happens, so the fallback cannot go unnoticed.
func replaceFromHeader(mime []byte, from string) []byte {
	firstLine, rest, found := bytes.Cut(mime, crlf)
	if !found || !bytes.HasPrefix(firstLine, []byte(fromHeaderPrefix)) {
		return mime
	}

	out := make([]byte, 0, len(fromHeaderPrefix)+len(from)+len(crlf)+len(rest))
	out = append(out, fromHeaderPrefix...)
	out = append(out, from...)
	out = append(out, crlf...)

	return append(out, rest...)
}
