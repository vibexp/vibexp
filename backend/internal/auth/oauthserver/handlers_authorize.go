package oauthserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/ory/fosite"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/models"
)

// errProviderChoiceRequired is returned when several login providers are enabled
// and the request did not name one. A provider picker UI is tracked in #33.
var errProviderChoiceRequired = errors.New("multiple identity providers enabled; specify ?provider=")

const (
	// devLoginProvider tags the login session created by the AS development-login
	// bypass. That path never federates to an upstream IdP, so the value is only a
	// self-describing marker on the stashed session.
	devLoginProvider = idp.ProviderName("dev")
	// devLoginEmail and devLoginName identify the user provisioned by the AS
	// development-login bypass, mirroring the /auth/dev/login default identity so a
	// zero-config local MCP client authenticates as a stable dev user.
	devLoginEmail = "dev@localhost"
	devLoginName  = "Dev User"
)

// Authorize handles GET /oauth2/authorize. It validates the OAuth request, then
// delegates user authentication to an upstream IdP, stashing the request until
// the user returns and consents.
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
	// Fail fast on PKCE before sending the user through upstream login. fosite
	// re-enforces S256 when the code is issued (defense in depth).
	if perr := validatePKCE(r); perr != nil {
		s.provider.WriteAuthorizeError(ctx, w, ar, perr)
		return
	}
	provider, perr := s.resolveLoginProvider(r)
	if perr != nil {
		// Development fallback: with no identity providers configured, authenticate
		// the login leg via the dev-login bypass instead of an upstream IdP. Reached
		// only when dev login is enabled (development env) AND no provider is enabled,
		// so it is unreachable in production.
		if s.cfg.DevLoginEnabled && len(s.registry.Enabled()) == 0 {
			s.authorizeWithDevLogin(ctx, w, r, ar)
			return
		}
		s.provider.WriteAuthorizeError(ctx, w, ar, fosite.ErrInvalidRequest.WithHint(perr.Error()))
		return
	}

	loginID, err := s.startLogin(ctx, r, provider)
	if err != nil {
		s.provider.WriteAuthorizeError(ctx, w, ar, fosite.ErrServerError)
		return
	}

	p, _ := s.registry.Get(provider)
	http.Redirect(w, r, p.AuthorizeURL(loginID, s.idpCallbackURI(), string(provider)), http.StatusFound)
}

// authorizeWithDevLogin completes the /authorize login leg using the
// development-login bypass: it provisions/reuses the dev user (the same path as
// /api/v1/auth/dev/login), stashes the authorize request attached to that user,
// and sends the client straight to the consent screen — no upstream IdP redirect.
// The caller guarantees this runs only when dev login is enabled and no identity
// provider is configured, so it is never reachable in production.
func (s *Service) authorizeWithDevLogin(
	ctx context.Context, w http.ResponseWriter, r *http.Request, ar fosite.AuthorizeRequester,
) {
	user, err := s.provisioner.HandleDevLogin(ctx, devLoginEmail, devLoginName)
	if err != nil {
		s.logger.With("error", err).Error("oauth AS dev login provisioning failed")
		s.provider.WriteAuthorizeError(ctx, w, ar, fosite.ErrServerError)
		return
	}
	loginID, err := s.startLogin(ctx, r, devLoginProvider)
	if err != nil {
		s.provider.WriteAuthorizeError(ctx, w, ar, fosite.ErrServerError)
		return
	}
	if aerr := s.loginSessions.AttachUser(ctx, loginID, user.ID); aerr != nil {
		s.logger.With("error", aerr).Error("oauth AS dev login could not record user")
		s.provider.WriteAuthorizeError(ctx, w, ar, fosite.ErrServerError)
		return
	}
	http.Redirect(w, r, s.consentRedirect(loginID), http.StatusFound)
}

// startLogin persists the federated-login stash and returns its id (also used as
// the upstream `state`).
func (s *Service) startLogin(ctx context.Context, r *http.Request, provider idp.ProviderName) (string, error) {
	loginID, err := randomID()
	if err != nil {
		return "", err
	}
	ls := &models.OAuthLoginSession{
		ID:             loginID,
		AuthorizeQuery: r.URL.RawQuery,
		Provider:       string(provider),
		IDPState:       loginID,
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

// resolveLoginProvider picks the IdP for the login leg: an explicit ?provider=
// when valid, else the sole enabled provider.
func (s *Service) resolveLoginProvider(r *http.Request) (idp.ProviderName, error) {
	requested := r.URL.Query().Get("provider")
	if requested != "" {
		if _, ok := s.registry.Get(idp.ProviderName(requested)); ok {
			return idp.ProviderName(requested), nil
		}
		return "", errors.New("unknown or disabled identity provider")
	}
	enabled := s.registry.Enabled()
	switch len(enabled) {
	case 0:
		return "", errors.New("no identity providers are enabled")
	case 1:
		return enabled[0], nil
	default:
		return "", errProviderChoiceRequired
	}
}

// IdPCallback handles GET /oauth2/idp/callback: the upstream IdP redirect. It
// exchanges the upstream code, provisions the user, records it on the login
// session, and sends the user to the consent screen.
func (s *Service) IdPCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		s.renderError(w, http.StatusBadRequest, "missing code or state")
		return
	}
	ls, err := s.loginSessions.Get(ctx, state)
	if err != nil {
		s.renderError(w, http.StatusBadRequest, "invalid or expired login session")
		return
	}
	user, err := s.authenticateUpstream(ctx, ls.Provider, code)
	if err != nil {
		s.renderError(w, http.StatusBadGateway, "upstream login failed")
		return
	}
	if aerr := s.loginSessions.AttachUser(ctx, ls.ID, user.ID); aerr != nil {
		s.renderError(w, http.StatusInternalServerError, "could not record login")
		return
	}
	http.Redirect(w, r, s.consentRedirect(ls.ID), http.StatusFound)
}

// authenticateUpstream exchanges the IdP code (using the AS callback URI) and
// provisions the resulting user.
func (s *Service) authenticateUpstream(ctx context.Context, provider, code string) (*models.User, error) {
	p, ok := s.registry.Get(idp.ProviderName(provider))
	if !ok {
		return nil, errors.New("provider no longer available")
	}
	_, claims, err := p.ExchangeCode(ctx, code, s.idpCallbackURI())
	if err != nil {
		s.logger.With("error", err, "provider", provider).Warn("oauth AS upstream code exchange failed")
		return nil, err
	}
	user, err := s.provisioner.ProvisionFromClaims(ctx, string(p.Name()), claims)
	if err != nil {
		s.logger.With("error", err).Error("oauth AS failed to provision user")
		return nil, err
	}
	return user, nil
}

// ConsentDetails handles GET /api/v1/oauth/consent?login=ID. It returns the
// consent-screen contents as JSON so the frontend SPA can render the approval
// page with the design system. The opaque `login` id is the bearer of the
// consent session (single-use, short-lived); the CSRF token is echoed back by the
// SPA on the decision POST.
func (s *Service) ConsentDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	loginID := r.URL.Query().Get("login")
	ls, err := s.loginSessions.Get(ctx, loginID)
	if err != nil || ls.UserID == nil {
		s.writeJSONError(w, http.StatusBadRequest, "invalid or expired consent session")
		return
	}
	ar, err := s.reconstructAuthorizeRequest(ctx, ls)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "invalid authorization request")
		return
	}
	// The body carries the consent CSRF token; keep it out of any cache.
	w.Header().Set("Cache-Control", "no-store")
	s.writeJSON(w, http.StatusOK, consentDetailsResponse{
		ClientName:   s.clientDisplayName(ctx, ar.GetClient().GetID()),
		RedirectHost: hostOf(ar.GetRedirectURI()),
		Scopes:       ar.GetRequestedScopes(),
		CSRF:         s.signConsent(loginID),
	})
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
		s.writeJSONError(w, http.StatusBadRequest, "invalid or expired consent session")
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
	w.Header().Set("Cache-Control", "no-store")
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
		return "", errors.New("oauthserver: consent decision produced no redirect")
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

// consentDetailsResponse is the JSON body of GET /api/v1/oauth/consent: what the
// SPA needs to render the approval screen, plus the CSRF token to echo back.
type consentDetailsResponse struct {
	ClientName   string   `json:"client_name"`
	RedirectHost string   `json:"redirect_host"`
	Scopes       []string `json:"scopes"`
	CSRF         string   `json:"csrf"`
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
