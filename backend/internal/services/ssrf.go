package services

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// normalizeHost lower-cases the host and strips a single trailing FQDN dot so the
// name blocklist cannot be bypassed via casing or "metadata.google.internal.".
func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

// blockedHostnames are names that must never be resolved/dialed regardless of the
// IP they currently point at (cloud metadata endpoints are the classic SSRF target).
var blockedHostnames = map[string]bool{
	"metadata.google.internal": true,
	"metadata":                 true,
}

// ssrfGuard decides whether an outbound destination is allowed. allowPrivate is set
// only in tests (which target loopback httptest servers); production guards keep it
// false so loopback/private/link-local ranges are rejected.
type ssrfGuard struct {
	allowPrivate bool
}

// defaultSSRFGuard is the production policy: reject all reserved ranges.
var defaultSSRFGuard = &ssrfGuard{}

// isBlockedIP reports whether ip falls into a range that outbound requests must
// never reach: loopback, private, link-local (incl. cloud metadata 169.254.169.254),
// unique-local IPv6, unspecified, and multicast.
func (g *ssrfGuard) isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if g.allowPrivate {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsPrivate() {
		return true
	}
	// IPv6 unique-local (fc00::/7) is covered by IsPrivate on modern Go, but guard
	// explicitly to be safe across versions.
	if v6 := ip.To16(); v6 != nil && ip.To4() == nil {
		if v6[0]&0xfe == 0xfc {
			return true
		}
	}
	return false
}

// validateOutboundHost resolves host and rejects it if its name is blocklisted or
// any resolved IP is in a reserved range. Call this before issuing a request to a
// user/agent-supplied URL to mitigate SSRF (it also rejects obviously private literals).
func (g *ssrfGuard) validateOutboundHost(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Normalize the host before the name blocklist check so casing and a trailing
	// FQDN dot ("Metadata.Google.Internal." etc.) cannot slip past it.
	host := normalizeHost(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("URL has no host")
	}
	if blockedHostnames[host] {
		return fmt.Errorf("host %q is not allowed", host)
	}

	// If the host is an IP literal, check it directly.
	if literal := net.ParseIP(host); literal != nil {
		if g.isBlockedIP(literal) {
			return fmt.Errorf("host resolves to a disallowed address range")
		}
		return nil
	}

	resolver := &net.Resolver{}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("failed to resolve host: %w", err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("host did not resolve to any address")
	}
	for _, addr := range addrs {
		if g.isBlockedIP(addr.IP) {
			return fmt.Errorf("host resolves to a disallowed address range")
		}
	}
	return nil
}

// control is a net.Dialer.Control hook that re-checks the IP the connection is
// actually about to dial. This defeats DNS rebinding: even if a name passed an
// earlier validation, the connect-time IP is verified again here.
func (g *ssrfGuard) control(_ string, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid dial address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("dial address is not an IP")
	}
	if g.isBlockedIP(ip) {
		return fmt.Errorf("connection to disallowed address range blocked")
	}
	return nil
}

// newSSRFSafeTransport returns an http.Transport whose Control hook rejects
// connections to reserved IP ranges at connect time, layered on the supplied base.
func (g *ssrfGuard) newSSRFSafeTransport(base *http.Transport) *http.Transport {
	if base == nil {
		base = &http.Transport{}
	}
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   g.control,
	}
	base.DialContext = dialer.DialContext
	return base
}
