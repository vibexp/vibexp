-- OIDC provider role mapping settings (#1643, epic #1640).
--
-- Two optional settings on a sign-in provider: roles_claim names the ID token
-- claim that carries the user's roles or groups (a dotted path such as
-- realm_access.roles), and admin_values lists the claim values that make a
-- user an instance admin. A NULL roles_claim (and no values) means the mapping
-- is off, which is what every existing provider reads back as.
--
-- Storage only: nothing evaluates the mapping yet (#1644), so sign-in
-- behaviour is unchanged. The checks mirror services.validateInstanceAuthRoleMapping
-- (internal/services/instance_auth_settings_validation.go);
-- change both together.

ALTER TABLE instance_auth_providers
    ADD COLUMN roles_claim  text,
    ADD COLUMN admin_values text[] NOT NULL DEFAULT '{}';

-- google and github have fixed claims, so only an oidc provider may map roles.
ALTER TABLE instance_auth_providers
    ADD CONSTRAINT instance_auth_providers_role_mapping_oidc_only CHECK (
        type = 'oidc' OR (roles_claim IS NULL AND cardinality(admin_values) = 0));

-- A claim without admin values (or values without a claim) maps nobody, so
-- the two are set together or not at all.
ALTER TABLE instance_auth_providers
    ADD CONSTRAINT instance_auth_providers_role_mapping_complete CHECK (
        (roles_claim IS NULL) = (cardinality(admin_values) = 0));

COMMENT ON COLUMN instance_auth_providers.roles_claim IS
    'ID token claim carrying the user''s roles or groups, as a dotted path (groups, realm_access.roles). NULL turns the role mapping off. oidc only.';
COMMENT ON COLUMN instance_auth_providers.admin_values IS
    'Claim values that make a user an instance admin, compared exactly (case-sensitive). Empty exactly when roles_claim is NULL. oidc only.';
