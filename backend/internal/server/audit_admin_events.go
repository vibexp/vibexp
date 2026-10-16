package server

import (
	"context"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// Admin user mutations in the audit log (#1515). They are recorded next to the
// existing activity rows, after the change succeeded; the actor is the admin
// from the context. Reset and setup links and password removal are auth
// events (#1514) and are not recorded again here.

// auditAdminUserEvent records op on a user account, with the account as the
// resource.
func (s *Server) auditAdminUserEvent(
	ctx context.Context, op models.AuditOperation, userID, email string, metadata map[string]any,
) {
	s.recordAudit(ctx, audit.Event{
		Operation: op, ResourceType: models.AuditResourceTypeUser, ResourceID: userID, ResourceName: email,
		Metadata: metadata,
	})
}

// auditAdminUserCreated records an admin creating a user and, for each team
// the user was put in, the membership it gained. Recorded only after the
// creation fully succeeded, so a creation the service rolled back records
// nothing.
func (s *Server) auditAdminUserCreated(
	ctx context.Context, userID, email string, teams []services.AdminTeamAssignment,
) {
	teamIDs := make([]string, 0, len(teams))
	for _, t := range teams {
		teamIDs = append(teamIDs, t.TeamID)
	}
	s.auditAdminUserEvent(ctx, models.AuditOpAdminUserCreated, userID, email,
		map[string]any{"email": email, "teams": teamIDs})
	for _, t := range teams {
		s.recordAudit(ctx, audit.Event{
			Operation: models.AuditOpTeamMemberAdded, TeamID: t.TeamID,
			ResourceType: models.AuditResourceTypeTeamMember, ResourceID: userID, ResourceName: email,
			Metadata: map[string]any{"target_user_id": userID, "target_email": email, "role": string(t.Role)},
		})
	}
}

// adminUserNameForAudit reads a user's name ahead of a rename, only when a
// recorder would use it. A failed read leaves it empty.
func (s *Server) adminUserNameForAudit(ctx context.Context, userID string) string {
	if s.auditRecorder() == nil {
		return ""
	}
	users := s.container.UserRepository()
	if users == nil {
		return ""
	}
	user, err := users.GetByID(ctx, userID)
	if err != nil || user == nil {
		return ""
	}
	return user.Name
}
