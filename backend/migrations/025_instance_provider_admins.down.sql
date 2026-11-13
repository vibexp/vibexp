-- Reverts 025_instance_provider_admins: provider-sourced admins lose their
-- admin status; root admins and manual grants are untouched.
DROP TABLE IF EXISTS instance_provider_admins;
