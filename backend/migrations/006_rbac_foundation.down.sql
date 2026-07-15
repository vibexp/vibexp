-- Revert the RBAC foundation migration (issue #221).

-- Revert the invitation-role CHECK (issue #221).
ALTER TABLE team_invitations
    DROP CONSTRAINT IF EXISTS team_invitations_role_check;

-- The owner-membership backfill is deliberately NOT reverted.
--
-- It is not possible to tell a backfilled row from one the application wrote:
-- the team-creation flow has always inserted an owner membership row, so the
-- rows the backfill adds are exactly the rows pre-006 code expects to exist.
-- Deleting them (or downgrading a role) on a rollback would destroy real
-- memberships and lock owners out of their teams. The backfill only ever adds
-- data that is correct under both the old and the new model, so leaving it in
-- place is safe and reverting it is not.
