package postgres

// adminTeamRoleCountsCTE is the per-team member aggregate of the admin team
// listing (#1138): total members plus how many hold the owner and admin roles.
//
// It reads team_members.role as DATA for cross-tenant admin reporting — a
// display projection behind instance-admin middleware — and never authorizes
// anything, so it is the one role-reading query outside the role column's own
// CRUD. It lives alone in this file because TestNoRolePredicatesInRepositorySQL
// exempts the file (roleAsDataFiles) and TestRoleAsDataFilesStayNarrow pins it
// to this single constant. Keep it a GROUP BY aggregate: an EXISTS over
// team_members would be checked (and rejected) by
// TestTeamAccessPredicatesAreCanonical.
const adminTeamRoleCountsCTE = `mc AS (SELECT team_id,
		COUNT(*) AS n,
		COUNT(*) FILTER (WHERE role = 'owner') AS owners,
		COUNT(*) FILTER (WHERE role = 'admin') AS admins
		FROM team_members GROUP BY team_id)`
