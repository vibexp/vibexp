package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/auth/idp"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/activities"
)

const (
	// stateCookieName is the short-lived HMAC-signed cookie used to carry
	// the CSRF state value between the login redirect and the callback.
	stateCookieName = "vx_state"

	// stateCookieMaxAge is 10 minutes — enough for a human to complete the
	// WorkOS login flow and be redirected back to the callback.
	stateCookieMaxAge = 10 * 60 // 10 minutes in seconds
)

// allowedProviders is the allowlist of WorkOS provider slugs accepted by the
// login endpoint. Unknown values are silently dropped to the empty string so
// the WorkOS wrapper falls back to its default (GoogleOAuth).
var allowedProviders = map[string]struct{}{
	"GoogleOAuth":    {},
	"GitHubOAuth":    {},
	"MicrosoftOAuth": {},
	"AppleOAuth":     {},
	"authkit":        {},
}

// LoginResponse is the JSON body returned by GET /api/v1/auth/login.
type LoginResponse struct {
	URL string `json:"url"`
}

// LogoutResponse is the JSON body returned by POST /api/v1/auth/logout.
type LogoutResponse struct {
	Message string `json:"message"`
}

// handleLogin generates a CSRF state, stores it in a signed cookie, and
// returns the WorkOS AuthKit authorization URL.
//
// GET /api/v1/auth/login
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	s.logger.WithFields(logrus.Fields{
		"service":    "vibexp-api",
		"handler":    "handleLogin",
		"user_agent": r.Header.Get("User-Agent"),
		"remote_ip":  r.RemoteAddr,
	}).Info("Login request received")

	state, err := generateRandomState()
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleLogin",
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to generate state")
		apiErr := errors.NewInternalError("Failed to generate authentication state")
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	// Sign the state to create a tamper-evident cookie value.
	signedState := s.signState(state)
	secure := !s.container.EnvironmentService().IsDevelopment()

	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    signedState,
		Path:     "/",
		MaxAge:   stateCookieMaxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	provider := r.URL.Query().Get("provider")
	if _, ok := allowedProviders[provider]; !ok {
		provider = ""
	}
	authURL := s.container.AuthService().GetLoginURL(state, provider)
	if authURL == "" {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleLogin",
		}).Warn("Identity provider not configured; login unavailable")
		apiErr := errors.NewServiceUnavailableError(
			"Authentication provider not configured. Use /auth/dev/login in local development.",
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleLogin",
		"state":   state,
	}).Info("Generated WorkOS login URL")

	writeOK(w, LoginResponse{URL: authURL}, s.logger)
}

// handleCallback validates the CSRF state cookie, exchanges the authorization
// code via WorkOS, looks up or creates the user, writes the session cookie,
// and redirects the browser to the frontend home page.
//
// GET /api/v1/auth/callback
func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	s.logAuthRequest("handleCallback", "WorkOS callback", r)

	code := r.URL.Query().Get("code")
	stateParam := r.URL.Query().Get("state")

	if code == "" {
		s.logAuthError("handleCallback", "Authorization code is required", nil)
		apiErr := errors.NewBadRequestError("Authorization code is required")
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	// Validate CSRF state cookie
	if err := s.validateStateCookie(r, stateParam); err != nil {
		s.logAuthError("handleCallback", "State validation failed", err)
		apiErr := errors.NewAuthInvalidError("Invalid or expired state parameter")
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	// Clear state cookie
	secure := !s.container.EnvironmentService().IsDevelopment()
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	user, idpTokens, isNewUser, err := s.container.AuthService().HandleCallback(r.Context(), code)
	if err != nil {
		s.handleCallbackFailure(w, r, stateParam, err)
		return
	}

	s.handleCallbackSuccess(w, r, user, idpTokens, stateParam, isNewUser)
}

func (s *Server) handleCallbackFailure(w http.ResponseWriter, r *http.Request, state string, err error) {
	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleCallback",
		"error":   fmt.Sprintf("%+v", err),
		"state":   state,
	}).Error("Failed to handle WorkOS callback")

	if s.metrics != nil {
		s.metrics.RecordUserLoginFailed(r.Context(), "workos_auth_failed")
	}

	apiErr := errors.NewIDPAuthError("WorkOS authentication failed")
	errors.WriteJSONError(w, r, apiErr)
}

func (s *Server) handleCallbackSuccess(
	w http.ResponseWriter,
	r *http.Request,
	user *models.User,
	tokens *idp.Tokens,
	state string,
	isNewUser bool,
) {
	s.logger.WithFields(logrus.Fields{
		"service":     "vibexp-api",
		"handler":     "handleCallback",
		"user_id":     user.ID,
		"email":       user.Email,
		"is_new_user": isNewUser,
	}).Info("WorkOS authentication successful")

	if s.metrics != nil {
		if isNewUser {
			s.metrics.RecordUserCreated(r.Context())
		}
		s.metrics.RecordUserLoginSuccessful(r.Context())
	}

	// Write session cookie
	if s.sessionManager != nil {
		idpSubject := ""
		if user.IDPSubject != nil {
			idpSubject = *user.IDPSubject
		}
		sess := &sesslib.Session{
			AccessToken:  tokens.AccessToken,
			RefreshToken: tokens.RefreshToken,
			ExpiresAt:    tokens.ExpiresAt,
			IDPSubject:   idpSubject,
			UserID:       user.ID,
		}
		if err := s.sessionManager.Write(w, sess); err != nil {
			s.logger.WithError(err).Error("Failed to write session cookie")
			apiErr := errors.NewInternalError("Failed to create session")
			errors.WriteJSONError(w, r, apiErr)
			return
		}
	}

	ar := NewActivityRecorder(s.container.ActivityService())
	sessionID := state
	metadata := map[string]interface{}{
		"provider": "workos",
		"email":    user.Email,
	}
	ar.RecordAuthActivity(r.Context(), user.ID, activities.ActivityTypeAuthLogin, &sessionID, metadata, r)

	// Redirect to frontend home after successful authentication
	http.Redirect(w, r, s.config.FrontendBaseURL+"/", http.StatusFound)
}

// handleLogout clears the session cookie and returns a JSON confirmation.
//
// POST /api/v1/auth/logout
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleLogout",
	}).Info("Logout request received")

	if s.sessionManager != nil {
		s.sessionManager.Clear(w)
	}
	// If sessionManager is nil there is no encrypted cookie that could
	// have been issued (Server.New returns early on Manager construction
	// failure in production paths), so there is nothing to clear here.

	writeOK(w, LogoutResponse{Message: "logged out"}, s.logger)
}

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleGetMe",
		"user_id": userID,
	}).Info("Get user profile request")

	user, err := s.container.AuthService().GetUserByID(r.Context(), userID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleGetMe",
			"user_id": userID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to get user")
		apiErr := errors.NewResourceNotFoundError("user", "User not found")
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	writeOK(w, user, s.logger)
}

// DevLoginRequest is the expected body for POST /api/v1/auth/dev/login.
type DevLoginRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func (s *Server) handleDevLogin(w http.ResponseWriter, r *http.Request) {
	if !s.container.EnvironmentService().IsDevLoginEnabled() {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleDevLogin",
		}).Warn("Dev login attempted in non-development environment")
		apiErr := errors.NewResourceNotFoundError("endpoint", "Endpoint not found")
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logAuthRequest("handleDevLogin", "Dev login", r)

	var req DevLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logAuthError("handleDevLogin", "Failed to decode request body", err)
		apiErr := errors.NewBadRequestError("Invalid request body")
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	if req.Email == "" {
		s.logAuthError("handleDevLogin", "Email is required", nil)
		validationErrs := []errors.ValidationError{
			errors.NewRequiredFieldError("email"),
		}
		apiErr := errors.NewValidationError("Request validation failed", validationErrs)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	if req.Name == "" {
		req.Name = "Dev User"
	}

	user, err := s.container.AuthService().HandleDevLogin(r.Context(), req.Email, req.Name)
	if err != nil {
		s.handleDevLoginFailure(w, r, req.Email, err)
		return
	}

	s.handleDevLoginSuccess(w, r, user)
}

func (s *Server) handleDevLoginFailure(w http.ResponseWriter, r *http.Request, email string, err error) {
	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleDevLogin",
		"email":   email,
		"error":   fmt.Sprintf("%+v", err),
	}).Error("Failed to handle dev login")

	apiErr := errors.NewInternalError("Authentication failed")
	errors.WriteJSONError(w, r, apiErr)
}

func (s *Server) handleDevLoginSuccess(w http.ResponseWriter, r *http.Request, user *models.User) {
	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleDevLogin",
		"user_id": user.ID,
		"email":   user.Email,
	}).Info("Dev authentication successful")

	ar := NewActivityRecorder(s.container.ActivityService())
	metadata := map[string]interface{}{
		"provider": "dev",
		"email":    user.Email,
	}
	ar.RecordAuthActivity(r.Context(), user.ID, activities.ActivityTypeAuthLogin, nil, metadata, r)

	// Build and write session cookie for dev login. The access token is
	// a non-validating marker — middleware never sends it to WorkOS, and
	// RefreshToken is empty so the refresh path short-circuits on the
	// "no refresh token available" branch when the session expires.
	if s.sessionManager != nil {
		devSubject := fmt.Sprintf("dev_%s", user.Email)
		sess := &sesslib.Session{
			AccessToken:  fmt.Sprintf("dev:%s", user.Email),
			RefreshToken: "",
			ExpiresAt:    time.Now().Add(24 * time.Hour),
			IDPSubject:   devSubject,
			UserID:       user.ID,
		}
		if err := s.sessionManager.Write(w, sess); err != nil {
			s.logger.WithError(err).Error("Failed to write dev session cookie")
			apiErr := errors.NewInternalError("Failed to create session")
			errors.WriteJSONError(w, r, apiErr)
			return
		}
	}

	writeOK(w, user, s.logger)
}

func generateRandomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// signState produces an HMAC-SHA256 signature of the state value and returns
// "state.signature" — both hex-encoded. The signing key is derived from the
// session manager's master key with a domain-separation tag, ensuring it
// is independent of the AES-GCM session-encryption key (per NIST SP 800-108
// key-separation guidance).
func (s *Server) signState(state string) string {
	key := s.stateMACKey()
	mac := hmac.New(sha256.New, key)
	// hmac.Hash.Write never returns an error.
	_, _ = mac.Write([]byte(state))
	sig := hex.EncodeToString(mac.Sum(nil))
	return state + "." + sig
}

// stateMACKey returns the per-session-manager derived HMAC key. Falls back
// to the raw password bytes when no session manager is configured (test
// builds and the no-session-manager fallback path).
func (s *Server) stateMACKey() []byte {
	if s.sessionManager != nil {
		return s.sessionManager.DeriveStateMACKey()
	}
	return []byte(s.config.WorkOSCookiePassword)
}

// validateStateCookie checks that the vx_state cookie carries the expected
// state and its HMAC signature is valid.
func (s *Server) validateStateCookie(r *http.Request, state string) error {
	cookie, err := r.Cookie(stateCookieName)
	if err != nil {
		return fmt.Errorf("state cookie missing: %w", err)
	}

	expected := s.signState(state)
	if !hmac.Equal([]byte(cookie.Value), []byte(expected)) {
		return fmt.Errorf("state cookie mismatch")
	}
	return nil
}

func (s *Server) logAuthRequest(handler, description string, r *http.Request) {
	s.logger.WithFields(logrus.Fields{
		"service":    "vibexp-api",
		"handler":    handler,
		"user_agent": r.Header.Get("User-Agent"),
		"remote_ip":  r.RemoteAddr,
	}).Info(description + " request received")
}

func (s *Server) logAuthError(handler, message string, err error) {
	fields := logrus.Fields{
		"service": "vibexp-api",
		"handler": handler,
	}
	if err != nil {
		fields["error"] = err
	}
	s.logger.WithFields(fields).Error(message)
}

func (s *Server) handleMarkOnboardingCompleted(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleMarkOnboardingCompleted",
		"user_id": userID,
	}).Info("Mark onboarding completed request")

	err := s.container.UserRepository().MarkOnboardingCompleted(r.Context(), userID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleMarkOnboardingCompleted",
			"user_id": userID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to mark onboarding completed")
		apiErr := errors.NewInternalError("Failed to mark onboarding completed")
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	// Fetch updated user to return
	user, err := s.container.UserRepository().GetByID(r.Context(), userID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleMarkOnboardingCompleted",
			"user_id": userID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to fetch user after marking onboarding completed")
		apiErr := errors.NewInternalError("Failed to fetch user")
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleMarkOnboardingCompleted",
		"user_id": userID,
	}).Info("Onboarding marked as completed successfully")

	writeOK(w, user, s.logger)
}
