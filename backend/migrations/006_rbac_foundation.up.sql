-- RBAC foundation (issue #221, epic #220).
--
-- Prepares team_members.role to become the single source of truth for
-- authorization. Neither change alters behavior on its own: nothing reads the
-- new guarantees until the enforcement issues (#222/#223) land.
--
--   1. Backfill: every teams.owner_id has a team_members row with role 'owner'.
--   2. Add the missing CHECK on team_invitations.role.

-- 1. Guarantee an owner membership row for every team (issue #221).
--
-- The team-creation flow always inserts one, but legacy rows predating it may
-- not have one. Once roles are the authorization source of truth, a team owner
-- without an 'owner' membership row would silently lose access to their own
-- team. teams.owner_id is NOT NULL, so every team yields a candidate row.
--
-- Insert only where the owner has no membership row at all; the ON CONFLICT
-- guard makes the statement safe to re-run against team_members_unique
-- (team_id, user_id).
INSERT INTO team_members (team_id, user_id, role)
SELECT t.id, t.owner_id, 'owner'
FROM teams t
WHERE NOT EXISTS (
    SELECT 1 FROM team_members m
    WHERE m.team_id = t.id AND m.user_id = t.owner_id
)
ON CONFLICT ON CONSTRAINT team_members_unique DO NOTHING;

-- Where the owner already has a membership row under a lesser role, upgrade it.
-- teams.owner_id stays authoritative for *who* the owner is (it is demoted to
-- referential data, not dropped), so the role column must agree with it before
-- the role becomes the thing we enforce on.
UPDATE team_members m
SET role = 'owner',
    updated_at = CURRENT_TIMESTAMP
FROM teams t
WHERE m.team_id = t.id
  AND m.user_id = t.owner_id
  AND m.role <> 'owner';

-- 2. Restrict invitation roles to member|admin (issue #221).
--
-- team_members.role has team_members_role_check ('owner','admin','member'),
-- but team_invitations.role has no constraint at all -- the database would
-- accept any string up to 20 chars, including 'owner'. An invitation must
-- never be able to mint a second owner, since exactly one owner per team is a
-- cross-cutting rule of the epic.
--
-- Pre-clean nonconforming rows first: ADD CONSTRAINT validates existing rows
-- and would fail the migration on legacy data.
UPDATE team_invitations
SET role = 'member',
    updated_at = CURRENT_TIMESTAMP
WHERE role NOT IN ('member', 'admin');

-- Postgres has no ADD CONSTRAINT IF NOT EXISTS; the catalog guard keeps this
-- re-runnable, matching the IF NOT EXISTS guards used by migration 005.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'team_invitations_role_check'
    ) THEN
        ALTER TABLE team_invitations
            ADD CONSTRAINT team_invitations_role_check CHECK (role IN ('member', 'admin'));
    END IF;
END
$$;
