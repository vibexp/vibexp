-- Reverts 025_instance_audit_settings: drops the instance audit-log settings
-- table. Any stored settings are lost; the built-in defaults apply again.
DROP TABLE IF EXISTS instance_audit_settings;
