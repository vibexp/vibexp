-- Team invitation addresses are stored lower case (#1434, epic #1426).
--
-- users.email has been lower case since 023, while invitee_email kept whatever
-- casing the inviter typed, so every lookup of "the invitations for this
-- account" had to compare lower(invitee_email) and could not use
-- idx_team_invitations_invitee_email. With the address normalized on write
-- (models.NormalizeEmail) and pinned here, the lookup is a plain equality on
-- that index again. It now also decides who may register in invite_only mode,
-- so it must not depend on how an address was typed.
--
-- No unique constraint involves invitee_email, so lower-casing cannot conflict.
UPDATE team_invitations SET invitee_email = lower(invitee_email) WHERE invitee_email <> lower(invitee_email);

ALTER TABLE team_invitations
    ADD CONSTRAINT team_invitations_invitee_email_lowercase CHECK (invitee_email = lower(invitee_email));

COMMENT ON COLUMN team_invitations.invitee_email IS
    'Lower case (team_invitations_invitee_email_lowercase). Written through models.NormalizeEmail.';
