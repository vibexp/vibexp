-- Reverts 025_instance_auth_policy: drops the stored authentication policy.
-- The instance goes back to the behaviour it has with no row stored: password
-- login off and open registration. Audit entries of the policy are kept.
DROP TABLE IF EXISTS instance_auth_policy;
