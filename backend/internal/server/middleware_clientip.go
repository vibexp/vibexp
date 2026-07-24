package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/vibexp/vibexp/internal/contextkeys"
)

// Client-IP resolution (#465).
//
// This replaces chi's middleware.RealIP, which overwrote r.RemoteAddr from
// X-Forwarded-For / X-Real-IP unconditionally. Those headers are client-supplied:
// on a directly-reachable instance — which "deploy anywhere" explicitly invites —
// a caller can rotate X-Forwarded-For per request and defeat the per-IP rate
// limiter guarding the unauthenticated auth surface and the OAuth AS entirely.
// chi deprecated RealIP for exactly this reason (GHSA-3fxj-6jh8-hvhx).
//
// The rule here: forwarded headers are honoured ONLY when the immediate peer is
// a configured trusted proxy. With no trusted proxies configured (the default)
// the peer address always wins, so the headers cannot influence anything.

// clientIPMiddleware resolves the client IP for each request and makes it
// available to the limiter, the request logger, access events, and activity
// records. It rewrites r.RemoteAddr so httprate.LimitByIP keeps working
// unchanged, and stashes the resolved value in the request context so
// downstream consumers stop re-parsing headers themselves.
func clientIPMiddleware(trusted []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resolved := resolveClientIP(r, trusted)
			if resolved != "" {
				// Rewrite RemoteAddr IN PLACE, before taking the context copy, so
				// middleware registered outside this one (the request logger) also
				// sees the resolved address — middleware.RealIP mutated in place
				// too, and logging the raw peer there while everything else used
				// the resolved IP would split the audit trail in two.
				//
				// httprate splits host:port, so keep it well-formed. The port is
				// meaningless once the address may have come from a header, and
				// nothing downstream reads it.
				r.RemoteAddr = net.JoinHostPort(resolved, "0")
				r = r.WithContext(context.WithValue(r.Context(), contextkeys.ClientIP, resolved))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// resolveClientIP implements the trust rule. Returns "" when nothing parses,
// leaving RemoteAddr untouched.
func resolveClientIP(r *http.Request, trusted []*net.IPNet) string {
	peer := peerIP(r.RemoteAddr)
	if peer == nil {
		return ""
	}

	// Untrusted peer: it speaks only for itself. This is the default path, and
	// the one that closes the bypass.
	if !ipInAny(peer, trusted) {
		return peer.String()
	}

	// Trusted peer: walk X-Forwarded-For RIGHT TO LEFT and take the right-most
	// entry that is not itself a trusted proxy. The left-most entry is the one
	// an attacker controls — it is whatever they sent — so reading from the left
	// (as the old getClientIP did) trusts the client, not the proxy chain.
	for _, hop := range rightToLeftHops(r.Header.Get("X-Forwarded-For")) {
		ip := net.ParseIP(hop)
		if ip == nil {
			// A malformed hop means the chain cannot be trusted past this point.
			break
		}
		if !ipInAny(ip, trusted) {
			return ip.String()
		}
	}

	// Whole chain was trusted proxies, or there was no XFF: fall back to
	// X-Real-IP, then the peer.
	if realIP := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); realIP != nil {
		return realIP.String()
	}
	return peer.String()
}

// rightToLeftHops splits an X-Forwarded-For value into trimmed hops, right-most
// first.
func rightToLeftHops(header string) []string {
	if header == "" {
		return nil
	}
	parts := strings.Split(header, ",")
	hops := make([]string, 0, len(parts))
	for i := len(parts) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(parts[i])
		if hop == "" {
			continue
		}
		// A hop may carry a port (some proxies emit "ip:port"), and IPv6 hops may
		// be bracketed. Normalise both before parsing.
		hops = append(hops, normaliseHop(hop))
	}
	return hops
}

// normaliseHop strips brackets and a trailing port from one X-Forwarded-For
// entry. Bare IPv6 (the common case) is returned unchanged — SplitHostPort
// would reject it, and stripping at the last colon would corrupt it.
func normaliseHop(hop string) string {
	if strings.HasPrefix(hop, "[") {
		// "[::1]:8080" or "[::1]"
		if host, _, err := net.SplitHostPort(hop); err == nil {
			return host
		}
		return strings.Trim(hop, "[]")
	}
	// Bare IPv6 contains multiple colons and has no port.
	if strings.Count(hop, ":") > 1 {
		return hop
	}
	if host, _, err := net.SplitHostPort(hop); err == nil {
		return host
	}
	return hop
}

// peerIP extracts the IP from a RemoteAddr. A real net/http RemoteAddr is
// always host:port, but a bracketed-IPv6-without-port form ("[::1]") shows up in
// hand-built test requests and was handled by the parser this replaces, so keep
// tolerating it.
func peerIP(remoteAddr string) net.IP {
	if remoteAddr == "" {
		return nil
	}
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return net.ParseIP(host)
	}
	return net.ParseIP(strings.Trim(remoteAddr, "[]"))
}

// ipInAny reports whether ip falls in any of the given networks.
func ipInAny(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIP returns the resolved client IP for a request, falling back to the
// peer address when the middleware did not run (direct handler tests). This is
// the ONE accessor downstream code should use — never re-read the headers.
func clientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(contextkeys.ClientIP).(string); ok && ip != "" {
		return ip
	}
	if ip := peerIP(r.RemoteAddr); ip != nil {
		return ip.String()
	}
	return ""
}

// warnIfProxyHeadersIgnored emits one startup line when a non-local deployment
// has no trusted proxies configured. Behind a reverse proxy that is a real
// misconfiguration — every request keys on the proxy's address, collapsing
// per-client limits into one bucket — and it is otherwise invisible until
// someone notices the limiter behaving oddly.
func warnIfProxyHeadersIgnored(logger *slog.Logger, trustedCount int, localDev bool) {
	if trustedCount > 0 || localDev {
		return
	}
	logger.Warn(
		"No server.trusted_proxies configured; X-Forwarded-For / X-Real-IP are ignored and " +
			"the peer address is used as the client IP. Correct for a directly-exposed instance. " +
			"If you run behind a reverse proxy, set server.trusted_proxies to its CIDR(s) or " +
			"per-client rate limits and access logs will all key on the proxy.",
	)
}
