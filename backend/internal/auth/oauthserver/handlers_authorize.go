package oauthserver

import (
	"context"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/ory/fosite"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
)

// errInvalidConsentSession is the JSON error returned whenever an opaque login
// id does not resolve to a live consent session.
const errInvalidConsentSession = "invalid or expired consent session"

// Consent responses carry single-use secrets (CSRF token, authorization code),
// so every one is marked Cache-Control: no-store.
const (
	headerCacheControl  = "Cache-Control"
	cacheControlNoStore = "no-store"
)

// Authorize handles GET /oauth2/authorize. It validates the OAuth request and
// PKCE, stashes it as a USER-LESS login session, and redirects the browser to the
// SPA consent gate. The Authorization Server never authenticates anyone itself:
// the SPA reuses the logged-in app user (or requires an explicit login) and binds
// that user to the session via POST /api/v1/oauth/consent/attach before consent.
func (s *Service) Authorize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ar, err := s.provider.NewAuthorizeRequest(ctx, r)
	if err != nil {
		s.provider.WriteAuthorizeError(ctx, w, ar, err)
		return
	}
	if rerr := s.validateResource(r); rerr != nil {
		s.provider.WriteAuthorizeError(ctx, w, ar, rerr)
		return
	}
	// Fail fast on PKCE before sending the user to login. fosite re-enforces S256
	// when the code is issued (defense in depth).
	if perr := validatePKCE(r); perr != nil {
		s.provider.WriteAuthorizeError(ctx, w, ar, perr)
		return
	}
	loginID, err := s.startLogin(ctx, r)
	if err != nil {
		s.provider.WriteAuthorizeError(ctx, w, ar, fosite.ErrServerError)
		return
	}
	http.Redirect(w, r, s.consentRedirect(loginID), http.StatusFound)
}

// startLogin persists the authorize request as a user-less login session and
// returns its opaque, single-use id (the bearer of the consent flow). No user is
// attached here: that happens only via ConsentAttach once the app login is
// confirmed, so the AS never authenticates anyone on its own.
func (s *Service) startLogin(ctx context.Context, r *http.Request) (string, error) {
	loginID, err := randomID()
	if err != nil {
		return "", err
	}
	ls := &models.OAuthLoginSession{
		ID:             loginID,
		AuthorizeQuery: r.URL.RawQuery,
		ExpiresAt:      time.Now().Add(s.cfg.AuthCodeTTL),
	}
	if cerr := s.loginSessions.Create(ctx, ls); cerr != nil {
		s.logger.With("error", cerr).Error("oauth AS failed to create login session")
		return "", cerr
	}
	return loginID, nil
}

// validateResource enforces RFC 8707: any requested resource must be the one
// this AS serves. Absence is allowed; the issued token's audience is bound to the
// resource URI regardless.
func (s *Service) validateResource(r *http.Request) error {
	for _, res := range r.URL.Query()["resource"] {
		if res != s.cfg.ResourceURI {
			return fosite.ErrInvalidRequest.WithHintf(
				"Requested resource %q is not served by this authorization server.", res)
		}
	}
	return nil
}

// validatePKCE requires a code_challenge with the S256 method (OAuth 2.1).
func validatePKCE(r *http.Request) error {
	q := r.URL.Query()
	if q.Get("code_challenge") == "" {
		return fosite.ErrInvalidRequest.WithHint("PKCE code_challenge is required.")
	}
	if q.Get("code_challenge_method") != "S256" {
		return fosite.ErrInvalidRequest.WithHint("Only the S256 PKCE code_challenge_method is supported.")
	}
	return nil
}

// ConsentDetails handles GET /api/v1/oauth/consent?login=ID. The opaque `login`
// id is the bearer of the consent session (single-use, short-lived). Until an app
// user is bound to it (via ConsentAttach), it returns {authenticated:false} so the
// SPA gates on an app login; once bound, it returns the approval-screen contents.
// The CSRF token is echoed back by the SPA on the attach and decision POSTs.
func (s *Service) ConsentDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	loginID := r.URL.Query().Get("login")
	ls, err := s.loginSessions.Get(ctx, loginID)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, errInvalidConsentSession)
		return
	}
	// The body carries the consent CSRF token; keep it out of any cache.
	w.Header().Set(headerCacheControl, cacheControlNoStore)
	// No user bound yet: the AS never logs anyone in itself. Signal the SPA to
	// complete an app login and bind the user via /consent/attach; the CSRF token
	// authorizes that call.
	if ls.UserID == nil {
		s.writeJSON(w, http.StatusOK, consentDetailsResponse{
			Authenticated: false,
			CSRF:          s.signConsent(loginID),
		})
		return
	}
	ar, err := s.reconstructAuthorizeRequest(ctx, ls)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "invalid authorization request")
		return
	}
	s.writeJSON(w, http.StatusOK, consentDetailsResponse{
		Authenticated: true,
		ClientName:    s.clientDisplayName(ctx, ar.GetClient().GetID()),
		RedirectHost:  hostOf(ar.GetRedirectURI()),
		Scopes:        ar.GetRequestedScopes(),
		CSRF:          s.signConsent(loginID),
	})
}

// ConsentAttach handles POST /api/v1/oauth/consent/attach. It binds the
// authenticated app user — resolved from the vx_session by the standard /api auth
// middleware — to the user-less AS login session, so consent can proceed. This is
// the ONLY way a user becomes attached: /oauth2/authorize never authenticates. The
// request is CSRF-protected by the X-CSRF-Token bound to the login id and requires
// a logged-in caller (401 otherwise), so signing out of the app gates MCP auth.
func (s *Service) ConsentAttach(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := ctx.Value(contextkeys.UserID).(string)
	if !ok || userID == "" {
		s.writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var body consentAttachRequest
	if derr := decodeJSONBody(r, &body); derr != nil {
		s.writeJSONError(w, http.StatusBadRequest, "malformed request body")
		return
	}
	if !s.verifyConsent(body.Login, r.Header.Get("X-CSRF-Token")) {
		s.writeJSONError(w, http.StatusBadRequest, "invalid consent token")
		return
	}
	ls, err := s.loginSessions.Get(ctx, body.Login)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, errInvalidConsentSession)
		return
	}
	// Idempotent for the same user; never let one user rebind another's session.
	if ls.UserID != nil && *ls.UserID != userID {
		s.writeJSONError(w, http.StatusConflict, "consent session already bound to another user")
		return
	}
	// Re-check the access allowlist before binding (#217). Login-time enforcement
	// leaves a window in which a removed user's still-live session could authorize
	// a new MCP client; this is the choke point that closes it. Deliberately after
	// the CSRF/session checks so the user lookup only runs for a well-formed,
	// authenticated request.
	if !s.allowConsentUser(w, r, userID) {
		return
	}
	if aerr := s.loginSessions.AttachUser(ctx, body.Login, userID); aerr != nil {
		s.logger.With("error", aerr).Error("oauth AS failed to attach consent user")
		s.writeJSONError(w, http.StatusInternalServerError, "failed to record login")
		return
	}
	w.Header().Set(headerCacheControl, cacheControlNoStore)
	s.writeJSON(w, http.StatusOK, consentAttachResponse{Authenticated: true})
}

// allowConsentUser applies the access-allowlist re-check (#217) for userID. It
// reports whether the user may be bound; on denial (or an undecidable policy) it
// writes the response itself and reports false.
//
// No policy configured ⇒ allow, so an open instance is unaffected. A lookup
// failure fails CLOSED: a policy we cannot evaluate must never widen MCP access.
func (s *Service) allowConsentUser(w http.ResponseWriter, r *http.Request, userID string) bool {
	if s.access == nil {
		return true
	}

	allowed, err := s.access.AllowUser(r.Context(), userID)
	if err != nil {
		s.logger.With(
			"error", err,
			"user_id", userID,
		).Error("oauth AS failed to evaluate the consent access policy")
		s.writeJSONError(w, http.StatusInternalServerError, "failed to verify access")
		return false
	}
	if allowed {
		return true
	}

	s.logger.With(
		"user_id", userID,
		"reason", "access_allowlist",
	).Info("oauth AS consent denied: user is not permitted to sign in")
	// RFC 9457 body rather than this package's short {"error": ...} shape: the SPA
	// only surfaces an error code when the body carries BOTH `code` and `detail`
	// (see frontend/src/services/oauthService.ts), and the stable
	// access_restricted code is the point of the denial. Same code and shape the
	// dev-login denial writes, so every denial surface reads identically.
	errors.WriteJSONError(w, r, errors.NewAccessRestrictedError("Your account is not permitted to sign in"))
	return false
}

// ConsentDecision handles POST /api/v1/oauth/consent. On approve it issues the
// authorization code; on deny it returns access_denied. Either way it captures
// the OAuth redirect the protocol would emit and returns it as JSON
// ({redirect_to}) so the fetch-based SPA can navigate the browser there — the MCP
// client then receives the code (or error) at its callback.
func (s *Service) ConsentDecision(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body consentDecisionRequest
	if err := decodeJSONBody(r, &body); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "malformed request body")
		return
	}
	if body.Action != consentApprove && body.Action != consentDeny {
		s.writeJSONError(w, http.StatusBadRequest, "action must be approve or deny")
		return
	}
	if !s.verifyConsent(body.Login, body.CSRF) {
		s.writeJSONError(w, http.StatusBadRequest, "invalid consent token")
		return
	}
	ls, err := s.loginSessions.Get(ctx, body.Login)
	if err != nil || ls.UserID == nil {
		s.writeJSONError(w, http.StatusBadRequest, errInvalidConsentSession)
		return
	}
	ar, err := s.reconstructAuthorizeRequest(ctx, ls)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "invalid authorization request")
		return
	}
	// The login session is single-use regardless of the decision.
	defer func() {
		if derr := s.loginSessions.Delete(ctx, body.Login); derr != nil {
			s.logger.With("error", derr).Warn("oauth AS failed to delete login session")
		}
	}()

	redirectTo, err := s.captureClientRedirect(ctx, ar, body.Action, *ls.UserID)
	if err != nil {
		s.logger.With("error", err).Error("oauth AS failed to capture consent redirect")
		s.writeJSONError(w, http.StatusInternalServerError, "failed to complete authorization")
		return
	}
	// redirect_to embeds the single-use authorization code; never cache it.
	w.Header().Set(headerCacheControl, cacheControlNoStore)
	s.writeJSON(w, http.StatusOK, consentDecisionResponse{RedirectTo: redirectTo})
}

// captureClientRedirect runs the fosite authorize-response (approve) or
// access-denied (deny) writer against an in-memory recorder and extracts the
// resulting client redirect URL. fosite always writes the outcome as a 302 to the
// client's redirect_uri; capturing the Location lets the JSON API hand that URL to
// a fetch() client instead of emitting the redirect itself.
func (s *Service) captureClientRedirect(
	ctx context.Context, ar fosite.AuthorizeRequester, action, userID string,
) (string, error) {
	rec := httptest.NewRecorder()
	if action == consentApprove {
		s.issueCode(ctx, rec, ar, userID)
	} else {
		s.provider.WriteAuthorizeError(ctx, rec, ar, fosite.ErrAccessDenied)
	}
	loc := rec.Header().Get("Location")
	if loc == "" {
		return "", stderrors.New("oauthserver: consent decision produced no redirect")
	}
	return loc, nil
}

// issueCode grants the requested scopes plus the resource audience and writes the
// authorization-code redirect.
func (s *Service) issueCode(
	ctx context.Context, w http.ResponseWriter, ar fosite.AuthorizeRequester, userID string,
) {
	for _, scope := range ar.GetRequestedScopes() {
		ar.GrantScope(scope)
	}
	ar.GrantAudience(s.cfg.ResourceURI)

	resp, err := s.provider.NewAuthorizeResponse(ctx, ar, newIssuingSession(userID))
	if err != nil {
		// fosite returns a generic server_error to the client (debug messages are
		// not exposed), so log the underlying cause to keep failures diagnosable.
		s.logger.With("error", err, "debug", fosite.ErrorToRFC6749Error(err).DebugField).
			Error("oauth AS failed to issue authorization code")
		s.provider.WriteAuthorizeError(ctx, w, ar, err)
		return
	}
	s.provider.WriteAuthorizeResponse(ctx, w, ar, resp)
}

// reconstructAuthorizeRequest re-parses the stashed /authorize query, now that
// the user is authenticated.
func (s *Service) reconstructAuthorizeRequest(
	ctx context.Context, ls *models.OAuthLoginSession,
) (fosite.AuthorizeRequester, error) {
	// Build the request locally from the stashed query; fosite parses it in
	// process and never dispatches it, so there is no outbound request.
	req := (&http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: AuthorizePath, RawQuery: ls.AuthorizeQuery},
	}).WithContext(ctx)
	return s.provider.NewAuthorizeRequest(ctx, req)
}

// clientDisplayName returns a human label for the consent screen, falling back to
// the client id.
func (s *Service) clientDisplayName(ctx context.Context, clientID string) string {
	c, err := s.clients.GetByID(ctx, clientID)
	if err != nil || c.ClientName == "" {
		return clientID
	}
	return c.ClientName
}

// consentApprove and consentDeny are the accepted values of the consent decision
// POST body's `action` field.
const (
	consentApprove = "approve"
	consentDeny    = "deny"
)

// consentDetailsResponse is the JSON body of GET /api/v1/oauth/consent. When no
// app user is bound yet it carries only {authenticated:false, csrf} so the SPA
// gates on login; once bound it carries the approval-screen contents.
type consentDetailsResponse struct {
	Authenticated bool     `json:"authenticated"`
	ClientName    string   `json:"client_name,omitempty"`
	RedirectHost  string   `json:"redirect_host,omitempty"`
	Scopes        []string `json:"scopes,omitempty"`
	CSRF          string   `json:"csrf"`
}

// consentDecisionRequest is the JSON body of POST /api/v1/oauth/consent.
type consentDecisionRequest struct {
	Login  string `json:"login"`
	CSRF   string `json:"csrf"`
	Action string `json:"action"`
}

// consentDecisionResponse is the JSON body returned for a consent decision: the
// URL the SPA must navigate the browser to so the client receives the outcome.
type consentDecisionResponse struct {
	RedirectTo string `json:"redirect_to"`
}

// consentAttachRequest is the JSON body of POST /api/v1/oauth/consent/attach: the
// opaque login id whose session the authenticated caller is bound to.
type consentAttachRequest struct {
	Login string `json:"login"`
}

// consentAttachResponse confirms the caller is now bound to the login session.
type consentAttachResponse struct {
	Authenticated bool `json:"authenticated"`
}
