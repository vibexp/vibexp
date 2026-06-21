package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/internal/config"
)

// TestBackofficeAuthMiddleware exercises the constant-time admin key comparison:
// a matching key passes, a mismatching key is 401, and an unconfigured key is 500.
func TestBackofficeAuthMiddleware(t *testing.T) {
	const adminKey = "bo-admin-key-constant-time-test" // #nosec G101 - test credential

	tests := []struct {
		name           string
		configuredKey  string
		authHeader     string
		wantStatus     int
		wantNextCalled bool
	}{
		{
			name:           "valid key passes",
			configuredKey:  adminKey,
			authHeader:     "Bearer " + adminKey,
			wantStatus:     http.StatusOK,
			wantNextCalled: true,
		},
		{
			name:           "invalid key is rejected",
			configuredKey:  adminKey,
			authHeader:     "Bearer wrong-key",
			wantStatus:     http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "missing header is rejected",
			configuredKey:  adminKey,
			authHeader:     "",
			wantStatus:     http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "unconfigured key returns server error",
			configuredKey:  "",
			authHeader:     "Bearer anything",
			wantStatus:     http.StatusInternalServerError,
			wantNextCalled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := &Server{config: &config.Config{BackofficeAdminAPIKey: tc.configuredKey}}

			nextCalled := false
			handler := srv.backofficeAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/bo/test", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
			assert.Equal(t, tc.wantNextCalled, nextCalled)
		})
	}
}
