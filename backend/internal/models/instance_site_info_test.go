package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInstanceSiteInfoValues_EmailBranding(t *testing.T) {
	cases := []struct {
		name   string
		values InstanceSiteInfoValues
		want   EmailBranding
	}{
		{
			name:   "nothing configured falls back to the product",
			values: InstanceSiteInfoValues{},
			want:   EmailBranding{DisplayName: ProductName, LogoURL: ProductLogoURL},
		},
		{
			name: "everything configured",
			values: InstanceSiteInfoValues{
				CompanyName: "Acme Inc.", CompanyEmail: "hello@acme.test", Website: "https://acme.test",
				SupportURL: "https://acme.test/help", TermsURL: "https://acme.test/terms",
				PrivacyURL: "https://acme.test/privacy", LogoURL: "https://cdn.acme.test/logo.png",
				LogoDarkURL: "https://cdn.acme.test/logo-dark.png",
			},
			want: EmailBranding{
				DisplayName: "Acme Inc.", HasCompany: true, LogoURL: "https://cdn.acme.test/logo.png",
				SupportHref: "https://acme.test/help", TermsURL: "https://acme.test/terms",
				PrivacyURL: "https://acme.test/privacy",
			},
		},
		{
			name:   "support falls back to the company email",
			values: InstanceSiteInfoValues{CompanyEmail: "hello@acme.test"},
			want: EmailBranding{
				DisplayName: ProductName, LogoURL: ProductLogoURL, SupportHref: "mailto:hello@acme.test",
			},
		},
		{
			name:   "a support URL wins over the company email",
			values: InstanceSiteInfoValues{CompanyEmail: "hello@acme.test", SupportURL: "https://acme.test/help"},
			want: EmailBranding{
				DisplayName: ProductName, LogoURL: ProductLogoURL, SupportHref: "https://acme.test/help",
			},
		},
		{
			name:   "only the dark logo is configured: emails keep the product logo",
			values: InstanceSiteInfoValues{LogoDarkURL: "https://cdn.acme.test/logo-dark.png"},
			want:   EmailBranding{DisplayName: ProductName, LogoURL: ProductLogoURL},
		},
		{
			name:   "a name with line breaks and control characters becomes one line",
			values: InstanceSiteInfoValues{CompanyName: "Acme\r\nBcc: evil@example.com\t\x00 Inc."},
			want: EmailBranding{
				DisplayName: "Acme Bcc: evil@example.com Inc.", HasCompany: true, LogoURL: ProductLogoURL,
			},
		},
		{
			name:   "a name of only whitespace is not a company",
			values: InstanceSiteInfoValues{CompanyName: " \n\t "},
			want:   EmailBranding{DisplayName: ProductName, LogoURL: ProductLogoURL},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.values.EmailBranding())
		})
	}
}
