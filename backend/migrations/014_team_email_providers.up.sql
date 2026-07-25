-- Issue #501: per-team email provider configuration (epic #499).
--
-- Mail credentials live only in config.yaml today, so every team on an instance
-- sends through the operator's provider and from the operator's address. This
-- table holds a team's own provider so its transactional mail comes from its own
-- domain and its own sending reputation.
--
-- Deliberately absent:
--   * no enabled / is_active flag -> the ABSENCE OF A ROW is the instance
--                                   fallback, so a disabled row would be a
--                                   second way to express the same state
--   * no is_default flag         -> UNIQUE (team_id) means one provider per
--                                  team, so a default has nothing to choose from
--
-- This migration only stores configuration; encryption, validation, the send-time
-- resolver and the HTTP surface land in the sibling issues of the epic.

CREATE TABLE public.team_email_providers (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    team_id uuid NOT NULL,
    -- Last editor, informational only: it records who configured the provider
    -- and is never used to scope a read (the team FK is the tenancy boundary).
    user_id uuid,
    -- One of smtp|mailgun|postmark|sendgrid, matching the values accepted by
    -- implementations.NewEmailProvider. Left as a varchar rather than an enum so
    -- adding a provider stays a code change with no migration.
    provider_type character varying(50) NOT NULL,
    -- Non-secret per-type fields (SMTP host/port/username, Mailgun domain and
    -- base URL, Postmark message stream). Shaped per provider_type, so it is
    -- jsonb rather than a column per provider knob.
    settings jsonb DEFAULT '{}'::jsonb NOT NULL,
    -- The single credential every supported provider has (SMTP password,
    -- Mailgun sending key, Postmark server token, SendGrid API key), as
    -- base64 AES-256-GCM ciphertext. NOT NULL because a row always represents a
    -- fully configured provider -- there is no half-configured state to store.
    secret_encrypted text NOT NULL,
    -- Envelope identity. 320 = 64-octet local part + "@" + 255-octet domain,
    -- the maximum length of an email address.
    from_address character varying(320) NOT NULL,
    from_name character varying(255),
    reply_to character varying(320),
    -- Delivery health. last_success_at and last_error_at are kept side by side
    -- rather than collapsed into one status column so the CURRENT state is
    -- derived by comparing them (later timestamp wins) while the last failure
    -- stays readable for diagnosis after recovery.
    last_success_at timestamp with time zone,
    last_error text,
    last_error_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    version bigint DEFAULT 1 NOT NULL
);

ALTER TABLE public.team_email_providers
    ADD CONSTRAINT team_email_providers_pkey PRIMARY KEY (id);

ALTER TABLE public.team_email_providers
    ADD CONSTRAINT fk_team_email_providers_team
        FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;

-- One provider per team, enforced by the schema so no code path can create a
-- second. This is the deliberate divergence from model_providers /
-- embedding_providers, which are many-rows-per-team with a partial unique index
-- on (team_id) WHERE is_default.
ALTER TABLE public.team_email_providers
    ADD CONSTRAINT unique_team_email_provider UNIQUE (team_id);

-- No index beyond the PK and the unique constraint: every query this table
-- serves is keyed on team_id, which unique_team_email_provider already covers.

COMMENT ON TABLE public.team_email_providers IS
    'Optional per-team outbound email provider. Row absent = the team falls back to the instance provider from config.yaml. At most one row per team (unique_team_email_provider).';

COMMENT ON COLUMN public.team_email_providers.secret_encrypted IS
    'Base64 AES-256-GCM ciphertext of the provider''s single credential. Never logged, never serialized (models.TeamEmailProvider tags it json:"-").';

COMMENT ON COLUMN public.team_email_providers.settings IS
    'Non-secret per-type fields, shaped by provider_type (e.g. SMTP host/port/username, Mailgun domain/base_url, Postmark message_stream).';
