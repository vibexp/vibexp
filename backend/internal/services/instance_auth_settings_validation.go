package services

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"

	"github.com/vibexp/vibexp/internal/models"
)

// ErrInvalidInstanceAuthProvider is wrapped by every
// ValidateInstanceAuthProvider failure, together with a *SettingsFieldError
// naming the field at fault.
var ErrInvalidInstanceAuthProvider = errors.New("invalid instance auth provider")

// ErrInvalidInstanceAuthAllowlist is wrapped by every
// ValidateInstanceAuthAllowlist failure, together with a *SettingsFieldError
// naming the list at fault and a message quoting the offending entry.
var ErrInvalidInstanceAuthAllowlist = errors.New("invalid instance auth allowlist")

// instanceAuthProviderSlugPattern is a provider slug: lower-case, url-safe,
// 1–63 characters, not starting with a hyphen. It mirrors the CHECK on
// instance_auth_providers.slug (migration 023); change both together.
var instanceAuthProviderSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// instanceAuthDomainLabel is one DNS label of an allowlisted email domain.
var instanceAuthDomainLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// maxInstanceAuthDomainLength is the longest DNS name (RFC 1035).
const maxInstanceAuthDomainLength = 253

// ValidateInstanceAuthProvider checks a provider before it is stored, by the
// same rules for the admin API (#1238) and the boot-time import (#1232): a
// known type, a url-safe slug, a display name and client id, and an issuer URL
// on exactly the OIDC providers — an absolute https:// URL (http:// only for
// localhost / 127.0.0.1) with no query or fragment. It does not inspect the
// client secret, which is ciphertext by the time a provider reaches storage.
func ValidateInstanceAuthProvider(p models.InstanceAuthProvider) error {
	if !p.Type.Valid() {
		return invalidInstanceAuthProvider("type", "type must be one of google, github, oidc, got %q", p.Type)
	}
	if !instanceAuthProviderSlugPattern.MatchString(p.Slug) {
		return invalidInstanceAuthProvider("slug",
			"slug must be 1-63 lower-case letters, digits or hyphens, not starting with a hyphen, got %q", p.Slug)
	}
	if strings.TrimSpace(p.DisplayName) == "" {
		return invalidInstanceAuthProvider("display_name", "display_name is required")
	}
	if strings.TrimSpace(p.ClientID) == "" {
		return invalidInstanceAuthProvider("client_id", "client_id is required")
	}
	return validateInstanceAuthIssuerURL(p.Type, p.IssuerURL)
}

// validateInstanceAuthIssuerURL enforces "issuer URL iff OIDC" and the URL's
// shape.
func validateInstanceAuthIssuerURL(providerType models.InstanceAuthProviderType, issuer *string) error {
	if providerType != models.InstanceAuthProviderOIDC {
		if issuer != nil {
			return invalidInstanceAuthProvider("issuer_url", "issuer_url is only allowed for oidc providers")
		}
		return nil
	}
	if issuer == nil || strings.TrimSpace(*issuer) == "" {
		return invalidInstanceAuthProvider("issuer_url", "issuer_url is required for oidc providers")
	}

	return validateInstanceAuthIssuerShape(*issuer)
}

// validateInstanceAuthIssuerShape checks a present issuer: absolute, no
// credentials, query or fragment, https (http only on a loopback host).
func validateInstanceAuthIssuerShape(issuer string) error {
	u, err := url.Parse(issuer)
	if err != nil || u.Host == "" || u.Opaque != "" {
		return invalidInstanceAuthProvider("issuer_url", "issuer_url must be an absolute URL, got %q", issuer)
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return invalidInstanceAuthProvider("issuer_url",
			"issuer_url must not carry credentials, a query or a fragment, got %q", issuer)
	}
	if u.Scheme == "https" || (u.Scheme == "http" && isInstanceAuthLoopbackHost(u.Hostname())) {
		return nil
	}
	return invalidInstanceAuthProvider("issuer_url",
		"issuer_url must use https (http is allowed only for localhost), got %q", issuer)
}

// isInstanceAuthLoopbackHost reports whether host is one of the local hosts a
// plain-http issuer is allowed on (local development only).
func isInstanceAuthLoopbackHost(host string) bool {
	return strings.EqualFold(host, "localhost") || host == "127.0.0.1"
}

func invalidInstanceAuthProvider(field, format string, args ...any) error {
	return fmt.Errorf("%w: %w", ErrInvalidInstanceAuthProvider,
		settingsInvalidValueError([]string{field}, format, args...))
}

// ValidateInstanceAuthAllowlist validates and normalizes an allowlist, by the
// same rules for the admin API (#1238) and the boot-time import (#1232), and
// returns the lists to store. Every entry is trimmed and lower-cased;
// duplicates are dropped case-insensitively, keeping the first occurrence's
// position; blank entries are skipped, the way a blank entry in a
// comma-separated env var is. A domain must be a DNS name of at least two
// labels (no leading "@"); an email must be a bare address. The first invalid
// entry fails the whole allowlist, quoted in the error. The returned slices are
// never nil.
func ValidateInstanceAuthAllowlist(domains, emails []string) (normDomains, normEmails []string, err error) {
	normDomains, err = normalizeInstanceAuthAllowlistEntries("domains", domains, isInstanceAuthDomain)
	if err != nil {
		return nil, nil, err
	}
	normEmails, err = normalizeInstanceAuthAllowlistEntries("emails", emails, isInstanceAuthEmail)
	if err != nil {
		return nil, nil, err
	}
	return normDomains, normEmails, nil
}

// normalizeInstanceAuthAllowlistEntries trims, lower-cases, validates and
// dedupes one list.
func normalizeInstanceAuthAllowlistEntries(
	field string, entries []string, valid func(string) bool,
) ([]string, error) {
	normalized := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		value := strings.ToLower(strings.TrimSpace(entry))
		if value == "" {
			continue
		}
		if !valid(value) {
			return nil, fmt.Errorf("%w: %w", ErrInvalidInstanceAuthAllowlist,
				settingsInvalidValueError([]string{field}, "%s contains an invalid entry %q", field, entry))
		}
		if _, dup := seen[value]; dup {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized, nil
}

// isInstanceAuthDomain reports whether a normalized value is an email domain:
// a DNS name of at least two labels.
func isInstanceAuthDomain(domain string) bool {
	if len(domain) > maxInstanceAuthDomainLength {
		return false
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !instanceAuthDomainLabel.MatchString(label) {
			return false
		}
	}
	return true
}

// isInstanceAuthEmail reports whether a normalized value is a bare email
// address (no display name, no angle brackets) whose domain is an email domain.
func isInstanceAuthEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Name != "" || addr.Address != email {
		return false
	}
	at := strings.LastIndexByte(email, '@')
	return at > 0 && isInstanceAuthDomain(email[at+1:])
}
