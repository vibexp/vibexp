package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/vibexp/vibexp/internal/auth/idp"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/services"
)

// Server-side sessions (#1646).
//
// Every vx_session cookie names a row of the sessions table, and the row holds
// the identity-provider tokens, encrypted; the cookie carries only the session
// id and the fields the authentication chokepoint reads (user id, epoch,
// provider, sign-in method and time). A cookie whose row is gone, expired or
// unreadable no longer authenticates, so signing out ends that session for
// every copy of its cookie.
//
// The cost is one primary-key read of the row per cookie-authenticated
// request, on top of the user read the chokepoint already makes.
//
// A cookie minted before #1646 has no session id and carries its tokens
// itself. The first request that authenticates with it converts it: once the
// chokepoint has accepted it (current epoch, account allowed), its tokens move
// to a new row and the cookie is re-issued without them. Nobody is signed out
// by the upgrade, and a revoked or suspended legacy cookie is never converted.

// errSessionStoreUnavailable is returned when the container provides no
// session store (no encryption key is configured). Cookie sessions then fail
// closed: no sign-in is issued and no cookie authenticates.
var errSessionStoreUnavailable = errors.New("session store unavailable")

// sessionStore returns the container's session store, or nil.
func (s *Server) sessionStore() services.SessionStoreInterface {
	if s.container == nil {
		return nil
	}
	return s.container.SessionStore()
}

// newSessionCookie stores a session that has just signed in as a new row and
// returns the cookie that names it. A failure leaves no row behind.
func (s *Server) newSessionCookie(ctx context.Context, sess *sesslib.Session) (*http.Cookie, error) {
	store := s.sessionStore()
	if store == nil {
		return nil, errSessionStoreUnavailable
	}
	if err := store.Create(ctx, sess); err != nil {
		return nil, err
	}
	cookie, err := s.sessionManager.Cookie(sess)
	if err != nil {
		s.deleteSession(ctx, sess)
		return nil, err
	}
	return cookie, nil
}

// loadSession fills sess's tokens from its row. services.ErrSessionNotFound
// means the session no longer authenticates; any other error is an
// infrastructure failure.
func (s *Server) loadSession(ctx context.Context, sess *sesslib.Session) error {
	store := s.sessionStore()
	if store == nil {
		return errSessionStoreUnavailable
	}
	return store.Load(ctx, sess)
}

// deleteSession removes sess's row, best effort: a failure is logged, and the
// row then expires and is swept. A legacy session has no row.
func (s *Server) deleteSession(ctx context.Context, sess *sesslib.Session) {
	if sess == nil || sess.IsLegacy() {
		return
	}
	store := s.sessionStore()
	if store == nil {
		return
	}
	if err := store.Delete(ctx, sess.ID); err != nil {
		s.logger.With("user_id", sess.UserID, "error", err.Error()).Warn("Failed to delete a session row")
	}
}

// sessionStillStored reports whether sess, read from a cookie, still has its
// row (#1646), filling its tokens from it. A legacy cookie has no row and
// carries its tokens itself, so it counts as stored.
func (s *Server) sessionStillStored(ctx context.Context, sess *sesslib.Session) bool {
	return sess.IsLegacy() || s.loadSession(ctx, sess) == nil
}

// removeSession deletes sess's row and returns the session as the row held it
// when it was deleted, so its refresh token is the latest one even when a
// refresh rotated it a moment ago (#1649). A legacy session has no row and is
// returned as it is. A failure is logged and returns nil, as does a row that
// was already gone or belongs to another user.
func (s *Server) removeSession(ctx context.Context, sess *sesslib.Session) *sesslib.Session {
	if sess.IsLegacy() {
		return sess
	}
	store := s.sessionStore()
	if store == nil {
		return nil
	}
	removed, err := store.Remove(ctx, sess.ID)
	if err != nil {
		s.logger.With("user_id", sess.UserID, "error", err.Error()).Warn("Failed to delete a session row")
		return nil
	}
	if removed == nil || removed.UserID != sess.UserID {
		return nil
	}
	return removed
}

// loadSessionOrReject loads the row of a cookie session for a required-auth
// request and reports whether the request may go on. A missing, expired or
// unreadable row is answered 401 with the cookie cleared; a failure to read it
// is a 500, never a 401, so a database outage does not sign anyone out.
func (s *Server) loadSessionOrReject(
	w http.ResponseWriter, r *http.Request, sess *sesslib.Session, logger *slog.Logger,
) bool {
	err := s.loadSession(r.Context(), sess)
	if err == nil {
		return true
	}
	log := logger.With("middleware", "authenticateWithSession", "user_id", sess.UserID, "error", err.Error())
	if errors.Is(err, services.ErrSessionNotFound) {
		log.Info("Session no longer exists; clearing the cookie")
		s.sessionManager.Clear(w)
		apierrors.WriteJSONError(w, r, apierrors.NewAuthInvalidError(sessionRevokedAuthDetail))
		return false
	}
	log.Error("Failed to load the session row")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(""))
	return false
}

// convertLegacySession moves the tokens of a legacy cookie (one minted before
// #1646) into a new row and re-issues the cookie without them. The caller has
// already accepted the session at the chokepoint. A failure to store the row is
// a 500 and keeps the cookie, so the next request tries again; a failure to
// re-issue the cookie is only logged, since the session in hand is valid.
func (s *Server) convertLegacySession(
	w http.ResponseWriter, r *http.Request, sess *sesslib.Session, logger *slog.Logger,
) bool {
	cookie, err := s.newSessionCookie(r.Context(), sess)
	if err != nil {
		logger.With("user_id", sess.UserID, "error", err.Error()).Error("Failed to convert a legacy session cookie")
		apierrors.WriteJSONError(w, r, apierrors.NewInternalError(""))
		return false
	}
	http.SetCookie(w, cookie)
	logger.With("user_id", sess.UserID).Info("Converted a legacy session cookie to a server-side session")
	return true
}

// providerRevocationTimeout bounds the identity-provider revocation calls of
// one sign-out (#1649), so an issuer that is slow or down delays it by at most
// this much.
const providerRevocationTimeout = 3 * time.Second

// revokeProviderTokens revokes, best effort, the identity-provider refresh
// token of each of sessions at the provider that issued it (RFC 7009, #1649).
// It runs after the sessions' rows are gone, so nothing it does can keep a
// session alive. The calls share one providerRevocationTimeout and are not
// cancelled with the request: a browser navigating away must not abort them
// halfway. A session without a refresh token (a sliding or password session)
// is skipped; every outcome is logged and none is returned.
func (s *Server) revokeProviderTokens(ctx context.Context, sessions ...*sesslib.Session) {
	if s.container == nil {
		return
	}
	var pending []*sesslib.Session
	for _, sess := range sessions {
		if sess != nil && sess.RefreshToken != "" {
			pending = append(pending, sess)
		}
	}
	if len(pending) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), providerRevocationTimeout)
	defer cancel()
	auth := s.container.AuthService()
	for _, sess := range pending {
		log := s.logger.With("user_id", sess.UserID, "provider", sess.Provider)
		err := auth.RevokeRefreshToken(ctx, sess.Provider, sess.RefreshToken)
		switch {
		case err == nil:
			log.Info("Revoked the identity provider's refresh token at sign-out")
		case errors.Is(err, idp.ErrRevocationUnsupported):
			log.Debug("The identity provider does not support token revocation; nothing revoked at sign-out")
		default:
			log.With("error", err.Error()).Warn("Failed to revoke the identity provider's refresh token at sign-out")
		}
	}
}
