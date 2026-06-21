-- Revert all teams back to non-personal (team workspaces)
UPDATE teams
SET is_personal = false
WHERE is_personal = true;
