package implementations

import (
	"bufio"
	"context"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noAuthRelay is a loopback SMTP server that, like Mailpit or an internal
// relay, advertises no AUTH extension. It records whether a client tried AUTH
// and whether a message was accepted.
type noAuthRelay struct {
	listener net.Listener
	mu       sync.Mutex
	authSeen bool
	accepted bool
	done     chan struct{}
}

func startNoAuthRelay(t *testing.T) *noAuthRelay {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	relay := &noAuthRelay{listener: listener, done: make(chan struct{})}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil && !strings.Contains(err.Error(), "use of closed") {
			t.Logf("closing the fake relay: %v", err)
		}
	})
	go relay.serveOne()
	return relay
}

func (r *noAuthRelay) port() string {
	return strings.TrimPrefix(r.listener.Addr().String(), "127.0.0.1:")
}

func (r *noAuthRelay) serveOne() {
	defer close(r.done)
	conn, err := r.listener.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }() //nolint:errcheck // close error is irrelevant to the fake relay

	reader := bufio.NewReader(conn)
	reply := func(line string) bool {
		_, werr := conn.Write([]byte(line + "\r\n"))
		return werr == nil
	}
	if !reply("220 relay.test ESMTP") {
		return
	}
	for {
		line, rerr := reader.ReadString('\n')
		if rerr != nil {
			return
		}
		if !r.handle(strings.ToUpper(strings.TrimSpace(line)), reader, reply) {
			return
		}
	}
}

// handle answers one command; false ends the session.
func (r *noAuthRelay) handle(cmd string, reader *bufio.Reader, reply func(string) bool) bool {
	switch {
	case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
		// No AUTH and no STARTTLS in the extension list.
		return reply("250-relay.test") && reply("250 8BITMIME")
	case strings.HasPrefix(cmd, "AUTH"):
		r.mu.Lock()
		r.authSeen = true
		r.mu.Unlock()
		return reply("502 AUTH not supported")
	case strings.HasPrefix(cmd, "DATA"):
		if !reply("354 go ahead") {
			return false
		}
		for {
			body, err := reader.ReadString('\n')
			if err != nil {
				return false
			}
			if body == ".\r\n" {
				break
			}
		}
		r.mu.Lock()
		r.accepted = true
		r.mu.Unlock()
		return reply("250 queued")
	case strings.HasPrefix(cmd, "QUIT"):
		reply("221 bye")
		return false
	default: // MAIL, RCPT, RSET, NOOP
		return reply("250 ok")
	}
}

func (r *noAuthRelay) outcome() (authSeen, accepted bool) {
	<-r.done
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.authSeen, r.accepted
}

// Without a password the provider is an unauthenticated relay client (#1208):
// it never attempts AUTH, so a relay that does not advertise it accepts mail.
func TestSMTPEmailProvider_NoPasswordSendsWithoutAuth(t *testing.T) {
	relay := startNoAuthRelay(t)

	provider, err := NewSMTPEmailProvider(SMTPSpec{Host: "127.0.0.1", Port: relay.port()})
	require.NoError(t, err)
	assert.Nil(t, provider.(*SMTPEmailProvider).auth, "no password means no AUTH")

	require.NoError(t, provider.SendEmail(context.Background(), outgoing(testMessage(), "")))

	authSeen, accepted := relay.outcome()
	assert.False(t, authSeen)
	assert.True(t, accepted)
}

// The counterpart, and the failure #1208 fixes: with a password the provider
// authenticates, which a relay without AUTH refuses.
func TestSMTPEmailProvider_PasswordAgainstANoAuthRelayFails(t *testing.T) {
	relay := startNoAuthRelay(t)

	provider, err := NewSMTPEmailProvider(SMTPSpec{
		Host: "127.0.0.1", Port: relay.port(), Username: "relay", Password: "password",
	})
	require.NoError(t, err)

	err = provider.SendEmail(context.Background(), outgoing(testMessage(), ""))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "doesn't support AUTH")
	_, accepted := relay.outcome()
	assert.False(t, accepted)
}

// With a password the provider still authenticates with PLAIN.
func TestSMTPEmailProvider_PasswordUsesPlainAuth(t *testing.T) {
	provider, err := NewSMTPEmailProvider(SMTPSpec{
		Host: "smtp.example.test", Port: "587", Username: "user@example.test", Password: "password",
	})
	require.NoError(t, err)

	auth := provider.(*SMTPEmailProvider).auth
	require.NotNil(t, auth)
	mechanism, initial, err := auth.Start(&smtp.ServerInfo{Name: "smtp.example.test", TLS: true, Auth: []string{"PLAIN"}})
	require.NoError(t, err)
	assert.Equal(t, "PLAIN", mechanism)
	assert.Equal(t, "\x00user@example.test\x00password", string(initial))
}
