package services_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

func validOIDCProvider() models.InstanceAuthProvider {
	return models.InstanceAuthProvider{
		Type:        models.InstanceAuthProviderOIDC,
		Slug:        "corp-sso",
		DisplayName: "Corp SSO",
		ClientID:    "client-id",
		IssuerURL:   strPtr("https://sso.example.com/realms/corp"),
	}
}

func TestValidateInstanceAuthProvider(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(p *models.InstanceAuthProvider)
		field  string // "" = valid
	}{
		{"a valid oidc provider", func(*models.InstanceAuthProvider) {}, ""},
		{"a valid google provider", func(p *models.InstanceAuthProvider) {
			p.Type, p.Slug, p.IssuerURL = models.InstanceAuthProviderGoogle, "google", nil
		}, ""},
		{"a valid github provider", func(p *models.InstanceAuthProvider) {
			p.Type, p.Slug, p.IssuerURL = models.InstanceAuthProviderGitHub, "github", nil
		}, ""},
		{"an unknown type", func(p *models.InstanceAuthProvider) { p.Type = "saml" }, "type"},
		{"an empty type", func(p *models.InstanceAuthProvider) { p.Type = "" }, "type"},
		{"a single-character slug", func(p *models.InstanceAuthProvider) { p.Slug = "a" }, ""},
		{"a 63-character slug", func(p *models.InstanceAuthProvider) { p.Slug = strings.Repeat("a", 63) }, ""},
		{"a 64-character slug", func(p *models.InstanceAuthProvider) { p.Slug = strings.Repeat("a", 64) }, "slug"},
		{"an empty slug", func(p *models.InstanceAuthProvider) { p.Slug = "" }, "slug"},
		{"an upper-case slug", func(p *models.InstanceAuthProvider) { p.Slug = "Corp" }, "slug"},
		{"a slug starting with a hyphen", func(p *models.InstanceAuthProvider) { p.Slug = "-corp" }, "slug"},
		{"a slug with an underscore", func(p *models.InstanceAuthProvider) { p.Slug = "corp_sso" }, "slug"},
		{"a slug with a slash", func(p *models.InstanceAuthProvider) { p.Slug = "corp/sso" }, "slug"},
		{"a blank display name", func(p *models.InstanceAuthProvider) { p.DisplayName = "  " }, "display_name"},
		{"a blank client id", func(p *models.InstanceAuthProvider) { p.ClientID = " " }, "client_id"},
		{"oidc without an issuer", func(p *models.InstanceAuthProvider) { p.IssuerURL = nil }, "issuer_url"},
		{"oidc with a blank issuer", func(p *models.InstanceAuthProvider) { p.IssuerURL = strPtr(" ") }, "issuer_url"},
		{"google with an issuer", func(p *models.InstanceAuthProvider) {
			p.Type, p.Slug = models.InstanceAuthProviderGoogle, "google"
		}, "issuer_url"},
		{"github with an empty issuer", func(p *models.InstanceAuthProvider) {
			p.Type, p.Slug, p.IssuerURL = models.InstanceAuthProviderGitHub, "github", strPtr("")
		}, "issuer_url"},
		{"a relative issuer", func(p *models.InstanceAuthProvider) { p.IssuerURL = strPtr("/realms/corp") }, "issuer_url"},
		{"an issuer with no scheme", func(p *models.InstanceAuthProvider) {
			p.IssuerURL = strPtr("sso.example.com")
		}, "issuer_url"},
		{"an http issuer on a public host", func(p *models.InstanceAuthProvider) {
			p.IssuerURL = strPtr("http://sso.example.com")
		}, "issuer_url"},
		{"an http issuer on localhost", func(p *models.InstanceAuthProvider) {
			p.IssuerURL = strPtr("http://localhost:8081/realms/dev")
		}, ""},
		{"an http issuer on 127.0.0.1", func(p *models.InstanceAuthProvider) {
			p.IssuerURL = strPtr("http://127.0.0.1:5556/dex")
		}, ""},
		{"an http issuer on a localhost look-alike", func(p *models.InstanceAuthProvider) {
			p.IssuerURL = strPtr("http://localhost.example.com")
		}, "issuer_url"},
		{"an ftp issuer", func(p *models.InstanceAuthProvider) { p.IssuerURL = strPtr("ftp://sso.example.com") }, "issuer_url"},
		{"an issuer with a query", func(p *models.InstanceAuthProvider) {
			p.IssuerURL = strPtr("https://sso.example.com?tenant=a")
		}, "issuer_url"},
		{"an issuer with a fragment", func(p *models.InstanceAuthProvider) {
			p.IssuerURL = strPtr("https://sso.example.com#x")
		}, "issuer_url"},
		{"an issuer with credentials", func(p *models.InstanceAuthProvider) {
			p.IssuerURL = strPtr("https://user:pass@sso.example.com")
		}, "issuer_url"},
		{"an unparseable issuer", func(p *models.InstanceAuthProvider) { p.IssuerURL = strPtr("https://%zz") }, "issuer_url"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := validOIDCProvider()
			tc.mutate(&p)

			err := services.ValidateInstanceAuthProvider(p)
			if tc.field == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, services.ErrInvalidInstanceAuthProvider)
			var fieldErr *services.SettingsFieldError
			require.True(t, errors.As(err, &fieldErr), "the error names its field")
			assert.Equal(t, []string{tc.field}, fieldErr.Fields)
			assert.Equal(t, services.SettingsFieldInvalidValue, fieldErr.Code)
		})
	}
}

func TestValidateInstanceAuthAllowlist_Normalizes(t *testing.T) {
	domains, emails, err := services.ValidateInstanceAuthAllowlist(
		[]string{" Example.COM ", "corp.example.org", "example.com", "", "  ", "EXAMPLE.com"},
		[]string{"Alice@Example.com", " bob@corp.example.org", "alice@example.COM", ""},
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"example.com", "corp.example.org"}, domains,
		"trimmed, lower-cased, deduped case-insensitively in first-seen order, blanks skipped")
	assert.Equal(t, []string{"alice@example.com", "bob@corp.example.org"}, emails)
}

func TestValidateInstanceAuthAllowlist_EmptyListsAreNonNil(t *testing.T) {
	domains, emails, err := services.ValidateInstanceAuthAllowlist(nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, domains)
	assert.NotNil(t, emails)
	assert.Empty(t, domains)
	assert.Empty(t, emails)
}

func TestValidateInstanceAuthAllowlist_RejectsInvalidEntries(t *testing.T) {
	tests := []struct {
		name    string
		domains []string
		emails  []string
		field   string
		bad     string
	}{
		{"a single-label domain", []string{"localhost"}, nil, "domains", "localhost"},
		{"a domain with a leading @", []string{"@example.com"}, nil, "domains", "@example.com"},
		{"a domain with a scheme", []string{"https://example.com"}, nil, "domains", "https://example.com"},
		{"a domain with an empty label", []string{"example..com"}, nil, "domains", "example..com"},
		{"a domain label starting with a hyphen", []string{"-bad.example.com"}, nil, "domains", "-bad.example.com"},
		{"a domain with a space", []string{"exa mple.com"}, nil, "domains", "exa mple.com"},
		{"a 64-character label", []string{strings.Repeat("a", 64) + ".com"}, nil, "domains", strings.Repeat("a", 64) + ".com"},
		{"an email without @", nil, []string{"alice.example.com"}, "emails", "alice.example.com"},
		{"an email with a display name", nil, []string{"Alice <alice@example.com>"}, "emails", "Alice <alice@example.com>"},
		{"an email with a single-label domain", nil, []string{"alice@localhost"}, "emails", "alice@localhost"},
		{"an email with consecutive dots", nil, []string{"john..doe@example.com"}, "emails", "john..doe@example.com"},
		{"a bare domain in emails", nil, []string{"example.com"}, "emails", "example.com"},
		{"one bad entry among good ones", []string{"example.com", "bad_domain"}, nil, "domains", "bad_domain"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			domains, emails, err := services.ValidateInstanceAuthAllowlist(tc.domains, tc.emails)
			require.ErrorIs(t, err, services.ErrInvalidInstanceAuthAllowlist)
			assert.Nil(t, domains)
			assert.Nil(t, emails)
			assert.Contains(t, err.Error(), tc.bad, "the error quotes the offending entry")
			var fieldErr *services.SettingsFieldError
			require.True(t, errors.As(err, &fieldErr))
			assert.Equal(t, []string{tc.field}, fieldErr.Fields)
			assert.Equal(t, services.SettingsFieldInvalidValue, fieldErr.Code)
		})
	}
}
