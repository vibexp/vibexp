package server

import (
	"net/http"

	"github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
)

// AuthProvider describes one enabled login provider for the login UI's provider
// picker: the slug to pass back as ?provider=, a human label and the provider
// type.
type AuthProvider struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
}

// PasswordLogin tells the login UI whether email and password sign-in is
// switched on, and how a forgotten password is reset: "email" when the instance
// has an email provider configured, "admin" when the user must ask an
// administrator for a reset link.
type PasswordLogin struct {
	Enabled bool   `json:"enabled"`
	Reset   string `json:"reset"`
}

// ProvidersResponse is the JSON body returned by GET /api/v1/auth/providers.
type ProvidersResponse struct {
	Providers models.JSONArray[AuthProvider] `json:"providers"`
	Password  PasswordLogin                  `json:"password"`
}

// logAuthPolicyResolveFailed is the log message of a sign-in options request
// that could not read the authentication policy or the password reset channel.
const logAuthPolicyResolveFailed = "Failed to resolve the authentication policy"

// msgAuthPolicyUnavailable is the client-facing detail of that failure.
const msgAuthPolicyUnavailable = "Sign-in options are temporarily unavailable"

// handleListProviders returns the deployment's enabled login providers with
// display metadata, so the login screen can render a provider picker without
// hardcoding the list. The list is in the admin-defined sort order and may be
// empty when no provider is enabled. The providers are resolved from the
// database on every call (#1234), so a change applies with no restart.
//
// The response also carries the password sign-in options (#1429): whether
// password login is switched on, from the instance authentication policy, and
// how a forgotten password is reset. A failure to read either answers 503 like
// a failure to read the providers: a sign-in page that guessed "password login
// is off" would hide the only way in on a password-only instance.
//
// GET /api/v1/auth/providers
func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	enabled, err := s.container.AuthService().EnabledProviders(r.Context())
	if err != nil {
		s.logAuthError("handleListProviders", logIdentityProvidersResolveFailed, err)
		errors.WriteJSONError(w, r, errors.NewServiceUnavailableError(msgIdentityProvidersUnavailable))
		return
	}
	providers := make([]AuthProvider, len(enabled))
	for i, p := range enabled {
		providers[i] = AuthProvider{Name: p.Slug, DisplayName: p.DisplayName, Type: p.Type}
	}
	password, err := s.passwordLogin(r)
	if err != nil {
		s.logAuthError("handleListProviders", logAuthPolicyResolveFailed, err)
		errors.WriteJSONError(w, r, errors.NewServiceUnavailableError(msgAuthPolicyUnavailable))
		return
	}
	writeOK(w, ProvidersResponse{Providers: providers, Password: password}, s.logger)
}

// passwordLogin reads the password sign-in options for the login UI. The reset
// channel is reported whether or not password login is enabled; it comes from
// the stored email configuration alone, and no mail is sent to find it out.
func (s *Server) passwordLogin(r *http.Request) (PasswordLogin, error) {
	resolver := s.container.AuthPolicyResolver()
	policy, err := resolver.Policy(r.Context())
	if err != nil {
		return PasswordLogin{}, err
	}
	reset, err := resolver.PasswordResetChannel(r.Context())
	if err != nil {
		return PasswordLogin{}, err
	}
	return PasswordLogin{Enabled: policy.PasswordLoginEnabled, Reset: reset}, nil
}
