package implementations

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A mail server that accepts the connection and then stalls must not hold the
// sender (#1472). stallingServer accepts every connection; with greet it says
// hello and answers EHLO, then never answers again, otherwise it never says
// anything.
func stallingServer(t *testing.T, greet bool) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	var mu sync.Mutex
	var conns []net.Conn
	closed := false
	t.Cleanup(func() {
		_ = listener.Close() //nolint:errcheck // test teardown
		mu.Lock()
		defer mu.Unlock()
		closed = true
		for _, c := range conns {
			_ = c.Close() //nolint:errcheck // test teardown
		}
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			if closed {
				// Accepted as the test ended: nothing will close it later.
				_ = conn.Close() //nolint:errcheck // test teardown
				mu.Unlock()
				return
			}
			conns = append(conns, conn)
			mu.Unlock()
			go stall(conn, greet)
		}
	}()
	return strings.TrimPrefix(listener.Addr().String(), "127.0.0.1:")
}

func stall(conn net.Conn, greet bool) {
	if !greet {
		return // read nothing, write nothing
	}
	reader := bufio.NewReader(conn)
	_, _ = conn.Write([]byte("220 stall.test ESMTP\r\n")) //nolint:errcheck // a fake server
	if _, err := reader.ReadString('\n'); err != nil {    // EHLO
		return
	}
	_, _ = conn.Write([]byte("250-stall.test\r\n250 8BITMIME\r\n")) //nolint:errcheck // a fake server
	// MAIL FROM and everything after it go unanswered.
	_, _ = reader.ReadString('\n') //nolint:errcheck // a fake server
}

func stallingProvider(t *testing.T, greet bool) *SMTPEmailProvider {
	t.Helper()
	provider, err := NewSMTPEmailProvider(SMTPSpec{Host: "127.0.0.1", Port: stallingServer(t, greet)})
	require.NoError(t, err)
	return provider.(*SMTPEmailProvider)
}

// sendLimit is how long a send may take before the test calls it stuck.
const sendLimit = 3 * time.Second

// sendWithin sends and fails the test if the send outlives sendLimit.
func sendWithin(t *testing.T, ctx context.Context, p *SMTPEmailProvider) error {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- p.SendEmail(ctx, outgoing(testMessage(), "")) }()
	select {
	case err := <-result:
		return err
	case <-time.After(sendLimit):
		t.Fatalf("the send was still running after %s", sendLimit)
		return nil
	}
}

func TestSMTPEmailProvider_StalledServerEndsWithTheContextDeadline(t *testing.T) {
	for name, greet := range map[string]bool{"silent from the start": false, "stalls after EHLO": true} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			start := time.Now()

			err := sendWithin(t, ctx, stallingProvider(t, greet))

			require.Error(t, err)
			assert.ErrorIs(t, err, context.DeadlineExceeded, "the error says the context ended it")
			assert.Less(t, time.Since(start), 2*time.Second)
		})
	}
}

func TestSMTPEmailProvider_CancellingTheContextEndsAStalledSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(150*time.Millisecond, cancel)

	err := sendWithin(t, ctx, stallingProvider(t, true))

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSMTPEmailProvider_NoDeadlineIsStillBounded(t *testing.T) {
	previous := smtpDefaultTimeout
	smtpDefaultTimeout = 200 * time.Millisecond
	t.Cleanup(func() { smtpDefaultTimeout = previous })

	err := sendWithin(t, context.Background(), stallingProvider(t, true))

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded, "the default deadline ended it")
}

// A context that allows longer than the bound (the 60 s request deadline) is
// still cut at the bound, so the caller hears back before its own deadline.
func TestSMTPEmailProvider_ALongDeadlineIsCappedByTheBound(t *testing.T) {
	previous := smtpDefaultTimeout
	smtpDefaultTimeout = 200 * time.Millisecond
	t.Cleanup(func() { smtpDefaultTimeout = previous })
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	err := sendWithin(t, ctx, stallingProvider(t, true))

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// A line break in an address is refused before anything is dialled, as
// net/smtp.SendMail does: no half-built envelope reaches the server.
func TestSMTPEmailProvider_LineBreakInAnAddressDialsNothing(t *testing.T) {
	err := sendMailContext(context.Background(), "127.0.0.1:1", nil,
		"from@example.test", []string{"ok@example.test", "bad@example.test\r\nRCPT TO:<x@y>"}, []byte("x"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "CR or LF", "refused before the dial (port 1 would refuse the connection)")
}

func TestSMTPEmailProvider_DialFailureIsPrompt(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := strings.TrimPrefix(listener.Addr().String(), "127.0.0.1:")
	require.NoError(t, listener.Close())
	provider, err := NewSMTPEmailProvider(SMTPSpec{Host: "127.0.0.1", Port: port})
	require.NoError(t, err)

	err = sendWithin(t, context.Background(), provider.(*SMTPEmailProvider))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "smtp: failed to send email")
}

// The bound is on the whole conversation, not on each step, and it is far
// from what a slow but healthy server needs: one that holds every reply back
// (a greeting delay, a busy relay) still delivers. Seven replies at 300 ms is
// over 2 s; a send fails only at smtpDefaultTimeout (30 s), as Mailgun's
// sender does at its own 30 s.
func TestSMTPEmailProvider_ASlowButHealthyServerStillDelivers(t *testing.T) {
	relay := startRelay(t, 300*time.Millisecond)
	provider, err := NewSMTPEmailProvider(SMTPSpec{Host: "127.0.0.1", Port: relay.port()})
	require.NoError(t, err)

	start := time.Now()
	require.NoError(t, provider.SendEmail(context.Background(), outgoing(testMessage(), "")))

	assert.Greater(t, time.Since(start), 2*time.Second, "the server really was slow")
	_, accepted := relay.outcome()
	assert.True(t, accepted)
}
