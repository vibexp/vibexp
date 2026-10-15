package oauthserver

import (
	"context"

	"github.com/ory/fosite"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// Audit log records of the Authorization Server (#1514): which MCP clients
// registered and which users let an agent in (or refused it). Nothing secret
// is recorded: no codes, tokens or consent CSRF values.

// auditClientRegistered records a Dynamic Client Registration. Registration is
// anonymous by design (RFC 7591), and it is an MCP client's first step.
func (s *Service) auditClientRegistered(ctx context.Context, client *models.OAuthClient) {
	audit.SafeRecord(ctx, s.audit, audit.Event{
		Operation:    models.AuditOpAuthMCPClientRegistered,
		Actor:        &audit.Actor{Type: models.AuditActorAnonymous},
		Source:       models.AuditSourceMCP,
		ResourceType: models.AuditResourceTypeOAuthClient, ResourceID: client.ID, ResourceName: client.ClientName,
		Metadata: map[string]any{
			"client_id": client.ID, "client_name": client.ClientName, "redirect_uris": client.RedirectURIs,
		},
	})
}

// auditConsentDecision records a user granting or denying an MCP client. The
// consent endpoint is reached through the login session rather than the
// request's own authentication, so the actor is the session's user.
func (s *Service) auditConsentDecision(ctx context.Context, ar fosite.AuthorizeRequester, action, userID string) {
	op := models.AuditOpAuthMCPConsentDenied
	if action == consentApprove {
		op = models.AuditOpAuthMCPConsentGranted
	}
	clientID := ar.GetClient().GetID()
	clientName := s.clientDisplayName(ctx, clientID)
	audit.SafeRecord(ctx, s.audit, audit.Event{
		Operation:    op,
		Actor:        &audit.Actor{UserID: &userID},
		ResourceType: models.AuditResourceTypeOAuthClient, ResourceID: clientID, ResourceName: clientName,
		Metadata: map[string]any{
			"client_id": clientID, "client_name": clientName, "scopes": []string(ar.GetRequestedScopes()),
		},
	})
}
