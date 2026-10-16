package services

import (
	"context"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// Team events in the audit log (#1515). They are recorded by the services
// after the change succeeds; the actor is the user the service acted for.

// TeamServiceOption customizes a TeamService at construction.
type TeamServiceOption func(*TeamService)

// WithTeamAuditRecorder makes the service record team changes in the audit
// log.
func WithTeamAuditRecorder(recorder audit.Recorder) TeamServiceOption {
	return func(s *TeamService) { s.audit = recorder }
}

// auditActorID names the acting user as the actor of an audit event.
func auditActorID(userID string) *audit.Actor {
	if userID == "" {
		return nil
	}
	return &audit.Actor{UserID: &userID}
}

// recordTeamEvent records op on team, with the team as the resource.
func (s *TeamService) recordTeamEvent(
	ctx context.Context, actorID string, team *models.Team, op models.AuditOperation, metadata map[string]any,
) {
	audit.SafeRecord(ctx, s.audit, audit.Event{
		Operation: op, Actor: auditActorID(actorID), TeamID: team.ID, TeamName: team.Name,
		ResourceType: models.AuditResourceTypeTeam, ResourceID: team.ID, ResourceName: team.Name,
		Metadata: metadata,
	})
}

// auditMember is the member a membership event is about.
type auditMember struct {
	userID, email string
	metadata      map[string]any
}

// recordMemberEvent records op on a team member, with the member as the
// resource. The target's id and email are also in the metadata, so the row
// reads on its own.
func (s *TeamService) recordMemberEvent(
	ctx context.Context, actorID string, team *models.Team, op models.AuditOperation, m auditMember,
) {
	metadata := map[string]any{"target_user_id": m.userID, "target_email": m.email}
	for k, v := range m.metadata {
		metadata[k] = v
	}
	audit.SafeRecord(ctx, s.audit, audit.Event{
		Operation: op, Actor: auditActorID(actorID), TeamID: team.ID, TeamName: team.Name,
		ResourceType: models.AuditResourceTypeTeamMember, ResourceID: m.userID, ResourceName: m.email,
		Metadata: metadata,
	})
}

// roleChange is the member a role change is about, with the role before and
// after.
func roleChange(userID, email, oldRole string, newRole models.TeamMemberRole) auditMember {
	return auditMember{
		userID: userID, email: email,
		metadata: map[string]any{"old_role": oldRole, "new_role": string(newRole)},
	}
}

// memberRoleForAudit reads a member's role ahead of a change, only when a
// recorder would use it. A failed read leaves it empty.
func (s *TeamService) memberRoleForAudit(ctx context.Context, teamID, userID string) string {
	if s.audit == nil {
		return ""
	}
	member, err := s.teamMemberRepo.GetByTeamAndUser(ctx, teamID, userID)
	if err != nil {
		return ""
	}
	return string(member.Role)
}

// memberForAudit reads the member a removal is about before it happens: after
// it the role is gone. Only when a recorder would use it.
func (s *TeamService) memberForAudit(ctx context.Context, teamID, userID string) auditMember {
	m := auditMember{userID: userID}
	if s.audit == nil {
		return m
	}
	if role := s.memberRoleForAudit(ctx, teamID, userID); role != "" {
		m.metadata = map[string]any{"role": role}
	}
	if user, err := s.userRepo.GetByID(ctx, userID); err == nil && user != nil {
		m.email = user.Email
	}
	return m
}

// teamChanges is the metadata of a team update: old and new values of the
// fields that changed.
func teamChanges(before, after *models.Team) map[string]any {
	changes := map[string]any{}
	if before.Name != after.Name {
		changes["name"] = map[string]any{"old": before.Name, "new": after.Name}
	}
	if before.Slug != after.Slug {
		changes["slug"] = map[string]any{"old": before.Slug, "new": after.Slug}
	}
	if before.Description != after.Description {
		changes["description"] = map[string]any{"old": before.Description, "new": after.Description}
	}
	return map[string]any{"changes": changes}
}
