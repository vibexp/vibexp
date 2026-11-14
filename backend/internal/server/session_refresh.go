package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/auth/idp"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// Access re-check (#1647).
//
// A cookie session that holds an identity-provider refresh token is refreshed
// when its access token has expired or when auth.session_recheck_interval has
// passed since its last refresh, whichever comes first. A provider that refuses
// the refresh (access removed, account disabled, refresh token revoked) ends
// the session, so access removed at the provider reaches VibeXP within the
// interval. A session without a refresh token is sliding and never re-checked.
//
// The refresh runs under a lock on the session's row (services.SessionStore
// .RefreshIfDue), which holds across replicas: concurrent requests on one due
// session wait for the first, then find the row refreshed and reuse it, so a
// rotated refresh token is never presented twice.

// defaultSessionRecheckInterval applies when the configured interval is not
// positive, which config validation rejects; it covers a zero-value Config.
const defaultSessionRecheckInterval = 15 * time.Minute

// sessionRefreshTimeout bounds one locked refresh, including the wait for the
// row lock. It is independent of the request: a client that goes away must not
// abandon a refresh after the provider rotated the token but before the new
// one is stored.
const sessionRefreshTimeout = 10 * time.Second

// sessionRefreshRetryAfter is the Retry-After of a 503 answered when the
// identity provider could not be reached.
const sessionRefreshRetryAfter = "5"

// errSessionRefreshTransient marks a refresh failure that keeps the session.
var errSessionRefreshTransient = errors.New("session refresh failed transiently")

// sessionRecheckInterval returns the configured re-check interval.
func (s *Server) sessionRecheckInterval() time.Duration {
	if s.config != nil && s.config.Auth.SessionRecheckInterval > 0 {
		return s.config.Auth.SessionRecheckInterval
	}
	return defaultSessionRecheckInterval
}

// refreshDue reports whether sess must be refreshed at now: it has a refresh
// token, and its access token has expired or the re-check interval has passed
// since its last refresh.
func (s *Server) refreshDue(sess *sesslib.Session, now time.Time) bool {
	if sess.RefreshToken == "" {
		return false
	}
	return !now.Before(sess.ExpiresAt) || now.Sub(sess.LastRefreshedAt) >= s.sessionRecheckInterval()
}

// refreshIfDue refreshes an authenticated session when refreshDue says so and
// reports whether the request may go on. It updates sess in place: the request
// context holds the same session, so a handler that re-issues the cookie
// writes the current tokens and not spent ones.
//
// A refused refresh deletes the row, clears the cookie, answers 401 and is
// audited; an unreachable provider answers 503 with Retry-After and keeps the
// session.
func (s *Server) refreshIfDue(
	w http.ResponseWriter, r *http.Request, sess *sesslib.Session, logger *slog.Logger,
) bool {
	if !s.refreshDue(sess, time.Now()) {
		return true
	}
	log := logger.With("middleware", "authenticateWithSession", "user_id", sess.UserID)
	store := s.sessionStore()
	if store == nil {
		log.Error("No session store to refresh the session with")
		apierrors.WriteJSONError(w, r, apierrors.NewInternalError(""))
		return false
	}
	log.Debug("Session refresh due, re-checking with the identity provider")

	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), sessionRefreshTimeout)
	defer cancel()
	refreshed, err := store.RefreshIfDue(ctx, sess,
		func(locked *sesslib.Session) bool { return s.refreshDue(locked, time.Now()) },
		func(ctx context.Context, locked *sesslib.Session) (*idp.Tokens, error) {
			return s.refreshAtProvider(ctx, locked, log)
		})
	if err != nil {
		s.answerRefreshFailure(w, r, sess, err, log)
		return false
	}
	if refreshed {
		if writeErr := s.sessionManager.Write(w, sess); writeErr != nil {
			log.With("error", writeErr).Warn("Failed to rewrite refreshed session cookie")
		}
	}
	return true
}

// refreshAtProvider calls the session's identity provider and classifies a
// failure: a transient one wraps errSessionRefreshTransient, anything else,
// including a refresh that answers for another subject, wraps
// services.ErrSessionRefreshRejected so the row is deleted.
func (s *Server) refreshAtProvider(
	ctx context.Context, sess *sesslib.Session, log *slog.Logger,
) (*idp.Tokens, error) {
	tokens, claims, err := s.container.AuthService().RefreshTokens(ctx, sess.Provider, sess.RefreshToken)
	if err != nil {
		if isTransientRefreshError(err) {
			return nil, fmt.Errorf("%w: %w", errSessionRefreshTransient, err)
		}
		return nil, fmt.Errorf("%w: %w", services.ErrSessionRefreshRejected, err)
	}
	if claims != nil && sess.IDPSubject != "" && claims.Subject != sess.IDPSubject {
		return nil, fmt.Errorf("%w: the refreshed identity names another subject", services.ErrSessionRefreshRejected)
	}
	if tokens.RefreshToken == "" {
		// A rotating provider returns a new refresh token on rotation. Empty
		// here means upstream behaviour changed; the stored refresh token is
		// kept, and the next refresh may fail with it.
		log.Warn("Refresh response missing new refresh_token; subsequent refreshes may fail")
	}
	// The claims are the provider's current view of the user; acting on them
	// (the role mapping on refresh) is #1648.
	return tokens, nil
}

// answerRefreshFailure writes the response for a failed locked refresh.
func (s *Server) answerRefreshFailure(
	w http.ResponseWriter, r *http.Request, sess *sesslib.Session, err error, log *slog.Logger,
) {
	log = log.With("error", fmt.Sprintf("%+v", err))
	switch {
	case errors.Is(err, services.ErrSessionRefreshRejected):
		log.Warn("Identity provider refused the session refresh; session ended")
		s.sessionManager.Clear(w)
		s.auditSessionEndedByProvider(r.Context(), sess, err)
		apierrors.WriteJSONError(w, r, apierrors.NewAuthInvalidError("Session expired"))
	case errors.Is(err, services.ErrSessionNotFound):
		log.Info("Session no longer exists; clearing the cookie")
		s.sessionManager.Clear(w)
		apierrors.WriteJSONError(w, r, apierrors.NewAuthInvalidError(sessionRevokedAuthDetail))
	case errors.Is(err, errSessionRefreshTransient) || isTransientRefreshError(err):
		log.Warn("Transient refresh failure; keeping session, asking client to retry")
		w.Header().Set("Retry-After", sessionRefreshRetryAfter)
		apierrors.WriteJSONError(w, r,
			apierrors.NewServiceUnavailableError("Authentication provider temporarily unavailable"))
	default:
		log.Error("Failed to refresh the session")
		apierrors.WriteJSONError(w, r, apierrors.NewInternalError(""))
	}
}

// auditSessionEndedByProvider records a session the identity provider ended.
// It is best effort and never delays the 401.
func (s *Server) auditSessionEndedByProvider(ctx context.Context, sess *sesslib.Session, cause error) {
	if s.auditRecorder() == nil {
		return
	}
	userID := sess.UserID
	actor := &audit.Actor{UserID: &userID}
	if repo := s.container.UserRepository(); repo != nil {
		if user, err := repo.GetByID(ctx, userID); err == nil && user != nil {
			actor = auditUserActor(user)
		}
	}
	metadata := map[string]any{auditMetaMethod: sess.AuthMethod, "error_class": refreshErrorClass(cause)}
	if sess.Provider != "" {
		metadata[auditMetaProvider] = sess.Provider
	}
	s.recordAudit(ctx, audit.Event{
		Operation:    models.AuditOpAuthSessionEndedByProvider,
		Actor:        actor,
		ResourceType: models.AuditResourceTypeUser,
		ResourceID:   userID,
		Metadata:     metadata,
	})
}

// refreshErrorClass names why a refresh was refused, without the provider's
// error text: "invalid_grant" when the provider said so, "subject_mismatch"
// when it answered for another user, else "rejected".
func refreshErrorClass(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "invalid_grant"):
		return "invalid_grant"
	case strings.Contains(msg, "another subject"):
		return "subject_mismatch"
	default:
		return "rejected"
	}
}
