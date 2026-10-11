-- Instance site info: admin-managed company branding (#1353, epic #1352).
--
-- The company name, contact address and links shown on the sign-in page, the
-- app shell and in emails have only ever come from `frontend.*` config keys and
-- hardcoded example.com fallbacks. This table is the storage that lets an
-- instance admin edit them at runtime.
--
-- It has the same singleton shape as instance_search_settings (022). Every
-- value is optional: NULL means "not configured", and the surface that would
-- show it is hidden. No row means nothing is configured, so DELETE is the
-- reset.
--
-- The length CHECKs mirror models.MaxSiteInfoCompanyNameLength,
-- models.MaxSiteInfoEmailLength and models.MaxSiteInfoURLLength; a change on
-- either side must change both. They are the storage backstop. The service
-- validates first, and it alone checks the URL scheme and the email format.
--
-- Storage only: nothing reads this table until the admin and public APIs land
-- (#1354, #1355), so existing behaviour is unchanged.

CREATE TABLE instance_site_info (
    -- Singleton key. It can only ever hold `true` (CHECK) and is the primary
    -- key, so a second row is a unique violation and no code path can create
    -- one.
    id            boolean PRIMARY KEY DEFAULT true CHECK (id),
    company_name  text,
    company_email text,
    website       text,
    support_url   text,
    terms_url     text,
    privacy_url   text,
    logo_url      text,
    logo_dark_url text,
    created_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Last editor. SET NULL on user deletion: removing an admin must not reset
    -- the instance's branding.
    updated_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    version     bigint NOT NULL DEFAULT 1,
    -- Mirrors models.MaxSiteInfoCompanyNameLength.
    CONSTRAINT instance_site_info_company_name_length CHECK (char_length(company_name) <= 120),
    -- Mirrors models.MaxSiteInfoEmailLength.
    CONSTRAINT instance_site_info_company_email_length CHECK (char_length(company_email) <= 254),
    -- The six below mirror models.MaxSiteInfoURLLength.
    CONSTRAINT instance_site_info_website_length CHECK (char_length(website) <= 2048),
    CONSTRAINT instance_site_info_support_url_length CHECK (char_length(support_url) <= 2048),
    CONSTRAINT instance_site_info_terms_url_length CHECK (char_length(terms_url) <= 2048),
    CONSTRAINT instance_site_info_privacy_url_length CHECK (char_length(privacy_url) <= 2048),
    CONSTRAINT instance_site_info_logo_url_length CHECK (char_length(logo_url) <= 2048),
    CONSTRAINT instance_site_info_logo_dark_url_length CHECK (char_length(logo_dark_url) <= 2048)
);

COMMENT ON TABLE instance_site_info IS
    'Instance company branding. At most one row (singleton key id = true); no row = nothing configured, so DELETE is the reset.';
COMMENT ON COLUMN instance_site_info.id IS
    'Singleton key: always true, so the primary key admits exactly one row.';
COMMENT ON COLUMN instance_site_info.company_name IS
    'Company name, at most 120 characters. The bound mirrors models.MaxSiteInfoCompanyNameLength; change both together. NULL = not configured.';
COMMENT ON COLUMN instance_site_info.company_email IS
    'Company contact address, at most 254 characters. The bound mirrors models.MaxSiteInfoEmailLength; change both together. NULL = not configured.';
COMMENT ON COLUMN instance_site_info.logo_dark_url IS
    'Optional dark-mode logo. NULL = use logo_url. Like every URL column, at most 2048 characters (models.MaxSiteInfoURLLength).';
COMMENT ON COLUMN instance_site_info.updated_by IS
    'Last editor. SET NULL when that user is deleted.';
