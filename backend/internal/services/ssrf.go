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

	"github.com/vibexp/vibexp/internal/config"
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

// outboundAllowlistHint is appended to every range-rejection error. Without it
// the failure is a dead end: the operator of a self-hosted embedding sidecar
// sees only "connection to disallowed address range blocked" with nothing
// naming the knob that fixes it (#745). It is safe to expose — the provider
// /validate responses map errors to fixed categories before returning them, so
// this text reaches logs and worker errors, not an API oracle.
const outboundAllowlistHint = " (declare the range in security.outbound_allowed_cidrs if it is yours)"

// ssrfGuard decides whether an outbound destination is allowed.
//
// allowPrivate is set only in tests (which target loopback httptest servers);
// production guards keep it false so loopback/private/link-local ranges are
// rejected. allowedCIDRs is the operator-declared exception list
// (security.outbound_allowed_cidrs): the ranges a self-hoster owns and wants
// reachable, typically the Docker subnet its embedding sidecar sits on. It can
// unblock loopback/private/unique-local addresses and nothing else — link-local
// (cloud metadata), multicast and the unspecified address stay blocked here
// regardless, and the config layer already refuses to load an entry overlapping
// them.
type ssrfGuard struct {
	allowPrivate bool
	allowedCIDRs []*net.IPNet
}

// defaultSSRFGuard is the production policy: reject all reserved ranges.
var defaultSSRFGuard = &ssrfGuard{}

// ssrfGuardForConfig selects the SSRF policy for cfg. In local development only
// (cfg.IsLocalDevelopment() — frontend.base_url points at localhost) it permits
// loopback/private/link-local destinations so a self-hosted local checkout can
// register and invoke a localhost A2A agent for testing. Every real deployment
// (and a nil cfg) gets the fail-closed production policy, narrowed only by the
// operator's own security.outbound_allowed_cidrs. The name blocklist (cloud
// metadata hostnames) still applies either way.
func ssrfGuardForConfig(cfg *config.Config) *ssrfGuard {
	if cfg == nil {
		return defaultSSRFGuard
	}
	if cfg.IsLocalDevelopment() {
		return &ssrfGuard{allowPrivate: true}
	}
	allowed := cfg.Security.ParsedOutboundAllowedCIDRs()
	if len(allowed) == 0 {
		return defaultSSRFGuard
	}
	return &ssrfGuard{allowedCIDRs: allowed}
}

// isBlockedIP reports whether ip falls into a range that outbound requests must
// never reach. Three tiers, in order:
//
//  1. link-local (incl. cloud metadata 169.254.169.254), multicast and the
//     unspecified address — blocked unconditionally, so no operator config can
//     reopen the SSRF hole;
//  2. loopback, private and IPv6 unique-local — blocked unless the operator
//     declared a covering prefix in security.outbound_allowed_cidrs;
//  3. everything else — allowed.
func (g *ssrfGuard) isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if g.allowPrivate {
		return false
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || isIPv6UniqueLocal(ip) {
		return !g.isOperatorAllowed(ip)
	}
	return false
}

// isIPv6UniqueLocal reports whether ip is in fc00::/7. Modern Go covers this in
// IsPrivate, but the check is kept explicit to be safe across versions.
func isIPv6UniqueLocal(ip net.IP) bool {
	v6 := ip.To16()
	if v6 == nil || ip.To4() != nil {
		return false
	}
	return v6[0]&0xfe == 0xfc
}

// isOperatorAllowed reports whether ip sits inside a prefix the operator
// declared in security.outbound_allowed_cidrs. Only tier-2 addresses ever reach
// it (see isBlockedIP), so an allowlist entry can never widen the policy beyond
// the reserved ranges a self-hoster legitimately owns.
func (g *ssrfGuard) isOperatorAllowed(ip net.IP) bool {
	for _, network := range g.allowedCIDRs {
		if network != nil && network.Contains(ip) {
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
			return fmt.Errorf("host resolves to a disallowed address range%s", outboundAllowlistHint)
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
			return fmt.Errorf("host resolves to a disallowed address range%s", outboundAllowlistHint)
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
		return fmt.Errorf("connection to disallowed address range blocked%s", outboundAllowlistHint)
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
