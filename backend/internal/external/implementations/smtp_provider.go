package implementations

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

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
// path makes — net/smtp.SendMail's steps, with PLAIN auth when a password is
// set — except that it honours the context (sendMailContext).
type SMTPEmailProvider struct {
	addr string
	auth smtp.Auth
	// sendMail is sendMailContext in production, replaced in tests so the
	// assembled bytes can be asserted without a live SMTP server.
	sendMail func(ctx context.Context, addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

// NewSMTPEmailProvider creates a new SMTP email provider.
func NewSMTPEmailProvider(spec SMTPSpec) (external.EmailProvider, error) {
	port, err := strconv.Atoi(spec.Port)
	if err != nil {
		return nil, fmt.Errorf("invalid SMTP port: %w", err)
	}

	// No password means an unauthenticated relay (Mailpit, an internal relay;
	// #1208): a nil auth makes sendMailContext skip AUTH, which such a server
	// may not advertise at all. With a password it authenticates with PLAIN.
	var auth smtp.Auth
	if spec.Password != "" {
		auth = smtp.PlainAuth("", spec.Username, spec.Password, spec.Host)
	}

	return &SMTPEmailProvider{
		addr:     fmt.Sprintf("%s:%d", spec.Host, port),
		auth:     auth,
		sendMail: sendMailContext,
	}, nil
}

// SendEmail assembles the message and delivers it over SMTP.
//
// The envelope sender stays the bare address — only the From header carries the
// display name — so nothing that validates the envelope sees a different value
// than before.
//
// ctx bounds the whole SMTP conversation (#1472): a server that accepts the
// connection and then stalls fails the send when ctx ends, or after
// smtpDefaultTimeout when ctx has no deadline.
func (p *SMTPEmailProvider) SendEmail(ctx context.Context, outgoing *external.OutgoingMessage) error {
	message := outgoing.Message

	// The error is part of gomail's signature rather than a reachable outcome —
	// BuildMimeMessage has no failure path today — but it is handled rather
	// than discarded in case that changes.
	mime, err := gomail.BuildMimeMessage(message)
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

	if err := p.sendMail(ctx, p.addr, p.auth, message.GetFrom(), recipients, mime); err != nil {
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

// smtpDefaultTimeout is the longest a send may take, whatever the context
// allows: it bounds a send whose context has no deadline, and it ends a
// stalled send well inside the 60 s request deadline and write timeout, so a
// caller such as the admin "send test email" still gets its answer. It matches
// the per-task bound of the account email runner (#1433). A var so tests can
// shorten it.
var smtpDefaultTimeout = 30 * time.Second

// errSMTPNoAuth is net/smtp.SendMail's refusal to send credentials to a server
// that does not offer AUTH, kept word for word.
var errSMTPNoAuth = errors.New("smtp: server doesn't support AUTH")

// sendMailContext is net/smtp.SendMail with a context (#1472). It takes the
// same steps — EHLO, STARTTLS when offered, AUTH when a is set, MAIL, RCPT,
// DATA, QUIT — over a connection whose deadline is ctx's or smtpDefaultTimeout
// from now, whichever comes first, and that is cut when ctx is cancelled, so
// a stalled server cannot hold the caller.
//
// One difference: SendMail skips AUTH against a server that answers only HELO
// (no extensions at all); here a set password against such a server is refused
// like one that does not offer AUTH, rather than sent unauthenticated.
func sendMailContext(
	ctx context.Context, addr string, a smtp.Auth, from string, to []string, msg []byte,
) (err error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("smtp: bad address %q: %w", addr, err)
	}
	// Refuse a line break in an address before anything is sent, as SendMail
	// does: checked command by command, a bad second recipient would leave
	// the server a half-built envelope.
	for _, line := range append([]string{from}, to...) {
		if strings.ContainsAny(line, "\r\n") {
			return errors.New("smtp: A line must not contain CR or LF")
		}
	}
	// The bound covers the dial too: a host that drops SYNs would otherwise
	// hold the caller for the OS connect timeout (minutes). WithTimeout keeps
	// an earlier deadline of ctx.
	ctx, cancel := context.WithTimeout(ctx, smtpDefaultTimeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	if err = conn.SetDeadline(deadline); err != nil {
		_ = conn.Close() //nolint:errcheck // the deadline error is the one reported
		return err
	}
	// A cancelled context fails the read or write in progress at once. Best
	// effort: if it cannot be set, the deadline above still ends the send.
	stop := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now()) //nolint:errcheck // see above
	})
	defer stop()
	defer func() { err = withContextCause(ctx, err) }()

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close() //nolint:errcheck // the greeting error is the one reported
		return err
	}
	// After QUIT the connection is already closed; on failure the error that
	// ended the conversation is the one reported.
	defer func() { _ = c.Close() }() //nolint:errcheck // see above
	if err = handshake(c, host, a); err != nil {
		return err
	}
	return deliver(c, from, to, msg)
}

// withContextCause says when the context ended a send: its error is added to
// err. A connection timeout counts as the deadline too, since the only
// deadline on the connection is ctx's, and the read can fail a moment before
// ctx's own timer marks it done.
func withContextCause(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%w (%w)", err, ctxErr)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%w (%w)", err, context.DeadlineExceeded)
	}
	return err
}

// handshake is the first half of net/smtp.SendMail's conversation: EHLO,
// STARTTLS when the server offers it, and AUTH when a is set.
func handshake(c *smtp.Client, host string, a smtp.Auth) error {
	if err := c.Hello("localhost"); err != nil {
		return err
	}
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if a != nil {
		if ok, _ := c.Extension("AUTH"); !ok {
			return errSMTPNoAuth
		}
		if err := c.Auth(a); err != nil {
			return err
		}
	}
	return nil
}

// deliver is the second half: the envelope, the message and QUIT.
func deliver(c *smtp.Client, from string, to []string, msg []byte) error {
	// Mail and Rcpt refuse an address containing CR or LF, as SendMail does.
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err := c.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(msg); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
