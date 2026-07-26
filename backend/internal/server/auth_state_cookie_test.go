package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/services"
)

// stateCookieContainer supplies the one dependency the state-cookie writers
// touch: the environment service, which decides the Secure attribute.
type stateCookieContainer struct {
	*BaseMockContainer
	env *services.EnvironmentService
}

func (c stateCookieContainer) EnvironmentService() *services.EnvironmentService {
	return c.env
}

func stateCookieServer(frontendBaseURL string) *Server {
	cfg := &config.Config{
		Frontend: config.FrontendConfig{BaseURL: frontendBaseURL},
	}
	return &Server{
		container: stateCookieContainer{
			BaseMockContainer: &BaseMockContainer{},
			env:               services.NewEnvironmentService(cfg),
		},
	}
}

func findCookie(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	require.FailNowf(t, "cookie not found", "no cookie named %q in response", name)
	return nil
}

// TestWriteStateCookie_Attributes pins the CSRF state cookie's security
// attributes.
//
// gosec's G124 fires on both state-cookie writes and is suppressed there,
// because it demands a literal `Secure: true` and cannot evaluate the
// environment-derived value (see the #nosec justifications in
// auth_handlers.go). This test is what actually guards the attributes, mirroring
// the coverage the session cookie already has in internal/auth/session (#553).
func TestWriteStateCookie_Attributes(t *testing.T) {
	tests := []struct {
		name           string
		frontendURL    string
		expectedSecure bool
	}{
		{
			name:           "production frontend sets Secure",
			frontendURL:    "https://app.example.com",
			expectedSecure: true,
		},
		{
			name:           "local development does not set Secure",
			frontendURL:    "http://localhost:5173",
			expectedSecure: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			stateCookieServer(tt.frontendURL).writeStateCookie(rec, "state-value")

			c := findCookie(t, rec, stateCookieName)
			assert.Equal(t, "state-value", c.Value)
			assert.True(t, c.HttpOnly, "state cookie must not be readable from JS")
			assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
			assert.Equal(t, "/", c.Path)
			assert.Equal(t, stateCookieMaxAge, c.MaxAge)
			assert.Equal(t, tt.expectedSecure, c.Secure,
				"Secure must track the deployment environment")
		})
	}
}

// TestClearStateCookie_Attributes covers the expiry write in handleCallback.
// It must carry the same attributes as the original or the browser will not
// match and drop it.
func TestClearStateCookie_Attributes(t *testing.T) {
	srv := stateCookieServer("https://app.example.com")

	rec := httptest.NewRecorder()
	srv.clearStateCookie(rec)

	c := findCookie(t, rec, stateCookieName)
	assert.Empty(t, c.Value)
	assert.Negative(t, c.MaxAge, "clearing must expire the cookie immediately")
	assert.True(t, c.HttpOnly)
	assert.True(t, c.Secure)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
	assert.Equal(t, "/", c.Path)
}
