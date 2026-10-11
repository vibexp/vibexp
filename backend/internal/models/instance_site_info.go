package models

import "time"

// Length caps for the instance site info values, counted in characters. They
// live here, in the leaf models package, because they are shared by the service
// validator (services.ValidateInstanceSiteInfo) and the instance_site_info
// CHECK constraints (migration 028). Change them together.
const (
	// MaxSiteInfoCompanyNameLength caps the company name.
	MaxSiteInfoCompanyNameLength = 120
	// MaxSiteInfoEmailLength caps the company email at the longest address
	// SMTP can carry.
	MaxSiteInfoEmailLength = 254
	// MaxSiteInfoURLLength caps every URL value.
	MaxSiteInfoURLLength = 2048
)

// InstanceSiteInfo is the instance's company branding as stored (#1353, epic
// #1352): the values an instance admin edits so the sign-in page, the app shell
// and emails show the organization rather than placeholder text.
//
// The table is a singleton (primary key `id boolean CHECK (id)`), so there is
// no ID field: the row is only present or absent. Every value is optional, and
// nil means "not configured".
type InstanceSiteInfo struct {
	CompanyName  *string `json:"company_name" db:"company_name"`
	CompanyEmail *string `json:"company_email" db:"company_email"`
	Website      *string `json:"website" db:"website"`
	SupportURL   *string `json:"support_url" db:"support_url"`
	TermsURL     *string `json:"terms_url" db:"terms_url"`
	PrivacyURL   *string `json:"privacy_url" db:"privacy_url"`
	LogoURL      *string `json:"logo_url" db:"logo_url"`
	// LogoDarkURL is the optional dark-mode logo; consumers fall back to
	// LogoURL when it is not set.
	LogoDarkURL *string   `json:"logo_dark_url" db:"logo_dark_url"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	// UpdatedBy is the user who last saved the values; nil once that user has
	// been deleted.
	UpdatedBy *string `json:"updated_by,omitempty" db:"updated_by"`
	Version   int64   `json:"version" db:"version"`
}

// InstanceSiteInfoValues is the complete set of site info values, without the
// storage metadata carried by InstanceSiteInfo. An empty string means "not
// configured"; the zero value is what an instance with nothing stored resolves
// to.
type InstanceSiteInfoValues struct {
	CompanyName  string `json:"company_name"`
	CompanyEmail string `json:"company_email"`
	Website      string `json:"website"`
	SupportURL   string `json:"support_url"`
	TermsURL     string `json:"terms_url"`
	PrivacyURL   string `json:"privacy_url"`
	LogoURL      string `json:"logo_url"`
	LogoDarkURL  string `json:"logo_dark_url"`
}

// InstanceSiteInfoView is the read model for the site info: the values plus
// the stored row's metadata.
type InstanceSiteInfoView struct {
	Values InstanceSiteInfoValues
	// UpdatedAt, UpdatedBy and Version describe the stored row; all three are
	// nil when no row is stored. UpdatedBy is also nil once the saving user has
	// been deleted. Version is the optimistic-lock counter Update's
	// expectedVersion is compared with.
	UpdatedAt *time.Time
	UpdatedBy *string
	Version   *int64
}
