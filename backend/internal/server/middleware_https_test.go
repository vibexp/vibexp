package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequireHTTPSMiddleware(t *testing.T) {
	const okBody = "reached handler"
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(okBody))
		assert.NoError(t, err)
	})

	tests := []struct {
		name       string
		isLocalDev bool
		setup      func(*http.Request)
		wantStatus int
		wantBody   bool
	}{
		{
			name:       "local dev bypasses enforcement over plain HTTP",
			isLocalDev: true,
			setup:      func(*http.Request) {},
			wantStatus: http.StatusOK,
			wantBody:   true,
		},
		{
			name:       "production rejects plain HTTP",
			isLocalDev: false,
			setup:      func(*http.Request) {},
			wantStatus: http.StatusForbidden,
			wantBody:   false,
		},
		{
			name:       "production rejects X-Forwarded-Proto http",
			isLocalDev: false,
			setup:      func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "http") },
			wantStatus: http.StatusForbidden,
			wantBody:   false,
		},
		{
			name:       "production allows X-Forwarded-Proto https",
			isLocalDev: false,
			setup:      func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") },
			wantStatus: http.StatusOK,
			wantBody:   true,
		},
		{
			name:       "production allows X-Forwarded-Proto HTTPS (case-insensitive)",
			isLocalDev: false,
			setup:      func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "HTTPS") },
			wantStatus: http.StatusOK,
			wantBody:   true,
		},
		{
			name:       "production allows a direct TLS connection",
			isLocalDev: false,
			setup:      func(r *http.Request) { r.TLS = &tls.ConnectionState{} },
			wantStatus: http.StatusOK,
			wantBody:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := requireHTTPSMiddleware(tc.isLocalDev)(next)
			req := httptest.NewRequest(http.MethodGet, "/oauth2/token", nil)
			tc.setup(req)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
			if tc.wantBody {
				assert.Equal(t, okBody, rr.Body.String())
			} else {
				assert.NotContains(t, rr.Body.String(), okBody)
			}
		})
	}
}
