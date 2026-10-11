-- Reverts 021_move_privacy_url_to_site_info: brings back
-- instance_email_provider.privacy_policy_url and refills it from site info's
-- privacy_url when an email provider row exists. Site info is left as it is,
-- so nothing an admin entered there since the upgrade is lost.
ALTER TABLE instance_email_provider ADD COLUMN privacy_policy_url text;

UPDATE instance_email_provider
   SET privacy_policy_url = (SELECT privacy_url FROM instance_site_info);
