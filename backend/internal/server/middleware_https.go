package server

import (
	"net/http"
	"strings"
)

// requireHTTPSMiddleware rejects plain-HTTP requests to the embedded OAuth 2.1
// Authorization Server endpoints (issue #34). The MCP authorization spec lists
// "HTTPS-only endpoints" as a MUST; before this, the requirement was satisfied
// only by deployment assumption (HSTS + Secure cookies + a TLS-terminating
// proxy). This makes it explicit and unconditional in any deployment.
//
// VibeXP is deploy-anywhere OSS and assumes no specific platform: TLS is almost
// always terminated by an upstream reverse proxy that forwards the original
// scheme in X-Forwarded-Proto, while a direct TLS listener populates r.TLS.
// Either signal is accepted as HTTPS (see requestIsHTTPS).
//
// Local development is exempt: the dev loop and the Playwright e2e stack serve
// plain HTTP on localhost. The bypass is keyed on Config.IsLocalDevelopment()
// (FRONTEND_BASE_URL on localhost) — the single source of truth for the dev
// heuristic — which is fail-closed, so a misconfigured production never bypasses.
func requireHTTPSMiddleware(isLocalDev bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isLocalDev && !requestIsHTTPS(r) {
				http.Error(w,
					"the OAuth Authorization Server requires HTTPS; ensure TLS terminates "+
						"upstream and the proxy forwards X-Forwarded-Proto: https",
					http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requestIsHTTPS reports whether the request reached us over TLS, either
// directly (r.TLS set) or via a reverse proxy that forwarded the original
// scheme in X-Forwarded-Proto.
func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
