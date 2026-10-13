-- Instance authentication policy (#1429, epic #1426).
--
-- Two instance-wide switches an admin sets in Admin -> Settings ->
-- Authentication, next to the sign-in providers and the access allowlist:
--
--   password_login_enabled  whether people may sign in with an email and
--                           password. Off by default: password login is opt-in.
--   registration_mode       who may get an account. 'open' is today's
--                           behaviour (anyone the allowlist admits may sign up
--                           through a provider); 'invite_only' admits only
--                           people who were invited or created by an admin.
--
-- No row is seeded. No row stored means the defaults (password login off,
-- registration open), which is exactly how the instance behaved before this
-- migration, so the upgrade changes nothing. This mirrors
-- instance_auth_allowlist, where no row means open access.
--
-- Every write also bumps instance_auth_settings_version in the same
-- transaction, which is the cache key of the resolver that serves the policy.
CREATE TABLE instance_auth_policy (
    -- Singleton key: always true, so the primary key admits exactly one row.
    id                      boolean PRIMARY KEY DEFAULT true CHECK (id),
    password_login_enabled  boolean NOT NULL DEFAULT false,
    -- The values mirror models.RegistrationMode*; change both together.
    registration_mode       text NOT NULL DEFAULT 'open'
        CONSTRAINT instance_auth_policy_registration_mode_check
        CHECK (registration_mode IN ('open', 'invite_only')),
    created_at              timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by              uuid REFERENCES users (id) ON DELETE SET NULL,
    version                 bigint NOT NULL DEFAULT 1
);

COMMENT ON TABLE instance_auth_policy IS
    'Instance authentication policy: the password login switch and the registration mode. At most one row (singleton key id = true); no row = the defaults (password login off, registration open).';
