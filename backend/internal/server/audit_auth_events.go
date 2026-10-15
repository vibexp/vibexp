package server

import (
	"context"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// Authentication and security events in the audit log (#1514).
//
// The auth routes are public (no contextkeys.UserID), so every call names its
// actor explicitly: the user who signed in or whose credential changed, or for
// a failed sign-in the attempted email with no user id. IP, user agent and
// request id still come from the context. Passwords, tokens, links, codes,
// cookies and API key secrets never go into an event.

// Sign-in methods recorded in an auth event's metadata.
const (
	auditAuthMethodOIDC            = "oidc"
	auditAuthMethodLocalCredential = "password"
	auditAuthMethodDev             = "dev"
	// auditMethodSetupToken is the first-run setup token (#1236), which opens
	// a userless setup session rather than signing anyone in.
	auditMethodSetupToken = "setup_token"
)

// Metadata keys of auth events.
const (
	auditMetaMethod   = "method"
	auditMetaProvider = "provider"
	auditMetaReason   = "reason"
	auditMetaNewUser  = "new_user"
	auditMetaBy       = "by"
	auditMetaTarget   = "target_user"
)

// Who changed a password: the account's owner or an instance admin.
const (
	auditBySelf  = "self"
	auditByAdmin = "admin"
)

// recordAudit hands an event to the audit recorder through audit.SafeRecord,
// so a broken recorder can neither fail nor stall the response.
func (s *Server) recordAudit(ctx context.Context, e audit.Event) {
	audit.SafeRecord(ctx, s.auditRecorder(), e)
}

// auditUserActor names user as the actor of an auth event.
func auditUserActor(user *models.User) *audit.Actor {
	if user == nil {
		return nil
	}
	id := user.ID
	return &audit.Actor{UserID: &id, Email: user.Email, Name: user.Name}
}

// auditLogin describes a successful sign-in.
type auditLogin struct {
	method   string
	provider string
	// via is "setup" or "reset" for a sign-in through a one-time link.
	via     string
	newUser bool
}

// auditLoginSucceeded records a sign-in that issued a session.
func (s *Server) auditLoginSucceeded(ctx context.Context, user *models.User, l auditLogin) {
	metadata := map[string]any{auditMetaMethod: l.method, auditMetaNewUser: l.newUser}
	if l.provider != "" {
		metadata[auditMetaProvider] = l.provider
	}
	if l.via != "" {
		metadata[auditMetaVia] = l.via
	}
	s.recordAudit(ctx, audit.Event{
		Operation: models.AuditOpAuthLogin, Actor: auditUserActor(user), Metadata: metadata,
	})
}

// auditLoginFailure describes a refused sign-in. email is the address the
// caller submitted, when there was one; userID is set only when the refusal
// identified the account (the password matched a suspended account, say).
type auditLoginFailure struct {
	method   string
	provider string
	reason   string
	email    string
	userID   string
}

// auditLoginFailed records a refused sign-in. An unknown account is recorded
// as an anonymous actor carrying the submitted email (capped by the recorder);
// nothing the caller presented as a credential is ever recorded.
func (s *Server) auditLoginFailed(ctx context.Context, f auditLoginFailure) {
	actor := &audit.Actor{Email: f.email, Type: models.AuditActorAnonymous}
	if f.userID != "" {
		actor = &audit.Actor{UserID: &f.userID, Email: f.email}
	}
	metadata := map[string]any{auditMetaMethod: f.method, auditMetaReason: f.reason}
	if f.provider != "" {
		metadata[auditMetaProvider] = f.provider
	}
	s.recordAudit(ctx, audit.Event{
		Operation: models.AuditOpAuthLoginFailed, Outcome: models.AuditOutcomeFailure,
		Actor: actor, Metadata: metadata,
	})
}

// auditUserEvent records op on a user account (a password or identity
// change, a reset link), with the account as the resource. The actor is the
// authenticated caller from the context: the user, or the admin acting on them.
func (s *Server) auditUserEvent(ctx context.Context, op models.AuditOperation, t auditTarget) {
	s.recordAudit(ctx, audit.Event{
		Operation: op, Metadata: t.metadata,
		ResourceType: models.AuditResourceTypeUser, ResourceID: t.userID, ResourceName: t.email,
	})
}

// auditTarget is the account an auth event acts on.
type auditTarget struct {
	userID   string
	email    string
	metadata map[string]any
}

// auditSelf is the caller's own account as the target of a self-service
// change.
func auditSelf(userID string, metadata map[string]any) auditTarget {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata[auditMetaBy] = auditBySelf
	return auditTarget{userID: userID, metadata: metadata}
}

// auditAPIKey records an API key being created or revoked (deleting a key is
// its revocation). The key is the resource, named by its label; the secret is
// never seen here. The actor is the authenticated owner from the context.
func (s *Server) auditAPIKey(ctx context.Context, op models.AuditOperation, keyID, keyName string) {
	s.recordAudit(ctx, audit.Event{
		Operation: op, ResourceType: models.AuditResourceTypeAPIKey, ResourceID: keyID, ResourceName: keyName,
	})
}

// apiKeyAuditName reads a key's name ahead of its deletion, only when a
// recorder would use it. A failed read leaves the name empty.
func (s *Server) apiKeyAuditName(ctx context.Context, userID, keyID string) string {
	if s.auditRecorder() == nil {
		return ""
	}
	keys, err := s.container.APIKeyService().GetAPIKeysByUserID(ctx, userID)
	if err != nil {
		return ""
	}
	for i := range keys {
		if keys[i].ID == keyID {
			return keys[i].Name
		}
	}
	return ""
}

// Purposes of an issued account link.
const (
	auditLinkPurposeSetup = "setup"
	auditLinkPurposeReset = "reset"
)

// auditLink is one setup or reset link an admin asked for.
type auditLink struct {
	userID, email, purpose, delivery string
	// invalidated reports that the old password, sessions and links were
	// stopped first; issued that a link actually went out. A reset whose
	// invalidation succeeded but whose link then failed is still recorded,
	// as a failure, because the account did change.
	invalidated, issued bool
}

// auditLinkIssued records an admin issuing a setup or reset link for a user.
// The link and its token are never recorded: only who, for whom, how it was
// delivered, whether the old credentials were stopped and whether a link went
// out. The actor is the admin from the context.
func (s *Server) auditLinkIssued(ctx context.Context, l auditLink) {
	outcome := models.AuditOutcomeSuccess
	if !l.issued {
		outcome = models.AuditOutcomeFailure
	}
	s.recordAudit(ctx, audit.Event{
		Operation: models.AuditOpAuthLocalCredentialResetLinkIssued, Outcome: outcome,
		ResourceType: models.AuditResourceTypeUser, ResourceID: l.userID, ResourceName: l.email,
		Metadata: map[string]any{
			"purpose": l.purpose, "delivery": l.delivery, "invalidated": l.invalidated, "issued": l.issued,
			auditMetaBy: auditByAdmin, auditMetaTarget: l.userID,
		},
	})
}
