-- Reverts 029_invitee_email_lowercase: drops the lower-case CHECK. Addresses
-- stay lower case; their original casing was not kept.
ALTER TABLE team_invitations
    DROP CONSTRAINT IF EXISTS team_invitations_invitee_email_lowercase;

COMMENT ON COLUMN team_invitations.invitee_email IS NULL;
