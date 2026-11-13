-- Provider-sourced instance admins (#1644, epic #1640).
--
-- A third source of instance admin, beside the root admins in config.yaml's
-- auth.instance_admins and the grants in instance_admins: a row here says the
-- user's last sign-in through the provider carried a claim value listed in the
-- provider's admin_values (024_oidc_provider_role_mapping). Only the role
-- mapping writes and deletes these rows: the sign-in that evaluates it, and a
-- settings save that clears the mapping or deletes the provider. A row of a
-- disabled provider is kept but grants nothing.

CREATE TABLE instance_provider_admins (
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider_id   uuid NOT NULL REFERENCES instance_auth_providers (id) ON DELETE CASCADE,
    -- The admin_values entry the claim matched at the last evaluation.
    matched_value text NOT NULL,
    evaluated_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, provider_id)
);

CREATE INDEX idx_instance_provider_admins_provider_id ON instance_provider_admins (provider_id);

COMMENT ON TABLE instance_provider_admins IS
    'Instance admins sourced from a sign-in provider''s role mapping. Written and deleted only by the mapping (sign-in, clearing the mapping, deleting the provider); never by a manual grant or revoke. Deleting the user or the provider removes the row.';
