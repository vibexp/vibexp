-- Reverts 020_instance_site_info: drops the instance branding table. Any
-- stored branding is lost; the instance shows nothing configured again.
DROP TABLE IF EXISTS instance_site_info;
