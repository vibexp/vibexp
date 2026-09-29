-- DB-managed authentication settings (#1231, epic #1230).
--
-- Identity providers, the sign-in access allowlist and instance admins have
-- only ever lived in config.yaml / env, read once at boot, so an instance admin
-- could not change them without file access and a restart. These four tables
-- are the storage that lets them be managed at runtime.
--
-- The invariants every writer must respect -- at most one google and one
-- github provider, a unique url-safe slug, an issuer URL on exactly the OIDC
-- providers -- are enforced here, so no code path (admin API, boot import,
-- break-glass CLI) can store an inconsistent provider set.
--
-- Storage only: nothing reads these tables until the resolvers land (#1232,
-- #1233, #1234, #1235), so login behaviour is unchanged.

CREATE TABLE instance_auth_providers (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type                    character varying(20) NOT NULL
                                CHECK (type IN ('google', 'github', 'oidc')),
    -- Immutable, url-safe identifier used in login/callback routes. The
    -- pattern mirrors services.instanceAuthProviderSlugPattern; change both
    -- together.
    slug                    character varying(63) NOT NULL UNIQUE
                                CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    display_name            text    NOT NULL,
    enabled                 boolean NOT NULL DEFAULT false,
    sort_order              integer NOT NULL DEFAULT 0,
    client_id               text    NOT NULL,
    -- Ciphertext from the encryption service; never plaintext. NULL when no
    -- secret is stored.
    client_secret_encrypted text,
    issuer_url              text,
    created_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Last editor. NULL for the boot-time config.yaml import, and SET NULL
    -- when that user is deleted.
    updated_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    -- An OIDC provider is discovered from its issuer; google and github have
    -- fixed endpoints and must not carry one.
    CONSTRAINT instance_auth_providers_issuer_iff_oidc CHECK (
        (type = 'oidc') = (issuer_url IS NOT NULL))
);

-- At most one google and one github provider: their endpoints are fixed, so a
-- second row of either could only be a duplicate. Any number of OIDC rows.
CREATE UNIQUE INDEX instance_auth_providers_one_per_builtin_type
    ON instance_auth_providers (type) WHERE type IN ('google', 'github');

CREATE INDEX instance_auth_providers_order ON instance_auth_providers (sort_order, slug);

COMMENT ON TABLE instance_auth_providers IS
    'DB-managed sign-in identity providers. At most one google and one github row; any number of oidc rows.';
COMMENT ON COLUMN instance_auth_providers.slug IS
    'Immutable url-safe identifier, ^[a-z0-9][a-z0-9-]{0,62}$. The pattern mirrors services.instanceAuthProviderSlugPattern; change both together.';
COMMENT ON COLUMN instance_auth_providers.client_secret_encrypted IS
    'OAuth client secret as ciphertext from the encryption service. Never plaintext; NULL when none is stored.';
COMMENT ON COLUMN instance_auth_providers.issuer_url IS
    'OIDC issuer URL. Required for type oidc and forbidden otherwise.';

CREATE TABLE instance_auth_allowlist (
    -- Singleton key: always true, so the primary key admits exactly one row.
    id          boolean PRIMARY KEY DEFAULT true CHECK (id),
    -- Normalized (trimmed, lower-cased, deduplicated) by
    -- services.ValidateInstanceAuthAllowlist before they are stored.
    domains     text[] NOT NULL DEFAULT '{}',
    emails      text[] NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    version     bigint NOT NULL DEFAULT 1
);

COMMENT ON TABLE instance_auth_allowlist IS
    'Sign-in access allowlist (email domains and addresses). At most one row (singleton key id = true); no row = open access.';

CREATE TABLE instance_auth_settings_version (
    id          boolean PRIMARY KEY DEFAULT true CHECK (id),
    version     bigint NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE instance_auth_settings_version IS
    'Change counter for instance_auth_providers and instance_auth_allowlist: every write to either bumps it in the same transaction. Seeded with one row so readers never see none; the cache key of the provider and allowlist resolvers.';

INSERT INTO instance_auth_settings_version (id, version) VALUES (true, 1);

CREATE TABLE instance_admins (
    user_id     uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    -- Who granted it. NULL for a grant with no actor (boot import, CLI), and
    -- SET NULL when that user is deleted: the grant outlives its grantor.
    granted_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE instance_admins IS
    'Instance admins granted in the database, in addition to those named in config.yaml. Deleting the user removes the grant.';
