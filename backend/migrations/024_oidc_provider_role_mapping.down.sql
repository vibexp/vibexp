-- Reverts 024_oidc_provider_role_mapping: drops the role mapping settings.
-- Any stored mapping is lost.
ALTER TABLE instance_auth_providers
    DROP CONSTRAINT IF EXISTS instance_auth_providers_role_mapping_complete,
    DROP CONSTRAINT IF EXISTS instance_auth_providers_role_mapping_oidc_only,
    DROP COLUMN IF EXISTS admin_values,
    DROP COLUMN IF EXISTS roles_claim;
