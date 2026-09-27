-- Instance-level outbound email provider (#1186, epic #1185).
--
-- Instance mail configuration has only ever lived in the `email:` section of
-- config.yaml, built once at wire time, so an instance admin could not change it
-- without file access and a restart, and there was nowhere to persist delivery
-- health or the instance-only contact/privacy fields. This table is that
-- storage. It is shaped like team_email_providers (011_consolidated) with three
-- deliberate differences:
--
--   * a singleton key instead of team_id -> the instance has exactly one
--                                          provider or none, and the database,
--                                          not application code, guarantees it
--   * secret_encrypted is NULLABLE       -> unauthenticated SMTP relays (and the
--                                          dev Mailpit setup) are legitimate
--                                          instance configurations; NULL means
--                                          "no credential"
--   * instance-only fields               -> contact_recipient_address and
--                                          privacy_policy_url have no per-team
--                                          counterpart
--
-- Storage only: nothing reads this table until the service/resolver lands
-- (#1188), so existing behaviour is unchanged.

CREATE TABLE public.instance_email_provider (
    -- Singleton key. The column can only ever hold `true` (CHECK below) and is
    -- the primary key, so a second row is a unique violation and no code path
    -- can create one. Every write is therefore naturally ON CONFLICT (id).
    id boolean DEFAULT true NOT NULL,
    -- One of smtp|mailgun|postmark|sendgrid, matching the values accepted by
    -- implementations.NewEmailProvider. A varchar rather than an enum so adding
    -- a provider stays a code change with no migration.
    provider_type character varying(50) NOT NULL,
    -- Non-secret per-type fields (SMTP host/port/username, Mailgun domain and
    -- base URL, Postmark message stream), shaped per provider_type.
    settings jsonb DEFAULT '{}'::jsonb NOT NULL,
    -- The provider's single credential (SMTP password, Mailgun sending key,
    -- Postmark server token, SendGrid API key) as base64 AES-256-GCM
    -- ciphertext. NULLABLE, unlike team_email_providers: NULL means the
    -- instance sends without a credential (e.g. an internal SMTP relay).
    -- Whether a given provider_type requires one is the service's decision.
    secret_encrypted text,
    -- Envelope identity. 320 = 64-octet local part + "@" + 255-octet domain.
    from_address character varying(320) NOT NULL,
    from_name character varying(255),
    reply_to character varying(320),
    -- Instance-only. Where the contact form delivers, and the privacy policy
    -- linked from outbound mail. Both nullable: empty means "fall back".
    contact_recipient_address character varying(320),
    privacy_policy_url text,
    -- Delivery health, kept as two timestamps rather than one status column so
    -- the current state is derived (later timestamp wins) while the last
    -- failure stays readable after recovery.
    last_success_at timestamp with time zone,
    last_error text,
    last_error_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    -- Last editor. NULL for the boot-time config.yaml import (no actor), and
    -- SET NULL on user deletion: removing an admin must not remove the
    -- instance's mail configuration.
    updated_by uuid,
    version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT instance_email_provider_pkey PRIMARY KEY (id),
    CONSTRAINT instance_email_provider_singleton CHECK (id),
    CONSTRAINT fk_instance_email_provider_updated_by
        FOREIGN KEY (updated_by) REFERENCES public.users(id) ON DELETE SET NULL
);

COMMENT ON TABLE public.instance_email_provider IS
    'Instance-level outbound email provider. At most one row (singleton key id = true); row absent = no stored instance configuration.';
COMMENT ON COLUMN public.instance_email_provider.id IS
    'Singleton key: always true (instance_email_provider_singleton), so the primary key admits exactly one row.';
COMMENT ON COLUMN public.instance_email_provider.secret_encrypted IS
    'Encrypted provider credential. NULL = no credential (e.g. unauthenticated SMTP relay).';
