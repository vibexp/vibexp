-- Give the privacy policy URL one owner (#1363, epic #1352).
--
-- It has lived on instance_email_provider.privacy_policy_url since the email
-- settings moved into the database, feeding the email footer. Site info
-- (020_instance_site_info) carries a privacy_url of its own, used on every
-- branded surface. Two sources for one link drift, so this moves the stored
-- value into site info and drops the column.
--
-- The value is copied only when ALL of these hold:
--   - it is non-empty after trimming;
--   - it is not the legacy placeholder default the operator never chose
--     ('https://example.com/privacy-policy');
--   - it fits the site info length cap (2048, models.MaxSiteInfoURLLength), so
--     the copy can never violate instance_site_info's CHECK and fail the
--     upgrade;
--   - site info has no privacy URL yet (no row, or a row whose privacy_url is
--     NULL). A privacy URL already set in site info is never overwritten.
--
-- A copy is a change to site info, so it appends one instance_settings_audit
-- entry (setting 'site_info', action 'import', no actor), exactly like every
-- other write to that table. The snapshots use the shape the service writes:
-- the eight values, with '' for one that is not configured.

DO $$
DECLARE
    candidate   text;
    had_row     boolean;
    before_doc  jsonb;
    after_doc   jsonb;
BEGIN
    SELECT btrim(privacy_policy_url) INTO candidate FROM instance_email_provider;

    IF candidate IS NULL
       OR candidate = ''
       OR candidate = 'https://example.com/privacy-policy'
       OR char_length(candidate) > 2048 THEN
        RETURN;
    END IF;

    SELECT true, jsonb_build_object(
               'company_name',  COALESCE(company_name, ''),
               'company_email', COALESCE(company_email, ''),
               'website',       COALESCE(website, ''),
               'support_url',   COALESCE(support_url, ''),
               'terms_url',     COALESCE(terms_url, ''),
               'privacy_url',   COALESCE(privacy_url, ''),
               'logo_url',      COALESCE(logo_url, ''),
               'logo_dark_url', COALESCE(logo_dark_url, ''))
      INTO had_row, before_doc
      FROM instance_site_info;

    IF had_row IS TRUE AND before_doc ->> 'privacy_url' <> '' THEN
        RETURN;
    END IF;

    IF had_row IS TRUE THEN
        UPDATE instance_site_info
           SET privacy_url = candidate,
               updated_at  = CURRENT_TIMESTAMP,
               version     = version + 1;
        after_doc := before_doc || jsonb_build_object('privacy_url', candidate);
    ELSE
        INSERT INTO instance_site_info (privacy_url) VALUES (candidate);
        before_doc := NULL;
        after_doc := jsonb_build_object(
            'company_name', '', 'company_email', '', 'website', '', 'support_url', '',
            'terms_url', '', 'privacy_url', candidate, 'logo_url', '', 'logo_dark_url', '');
    END IF;

    INSERT INTO instance_settings_audit (setting, action, actor_user_id, before, after)
    VALUES ('site_info', 'import', NULL, before_doc, after_doc);
END
$$;

ALTER TABLE instance_email_provider DROP COLUMN privacy_policy_url;
