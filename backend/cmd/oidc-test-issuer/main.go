// Command oidc-test-issuer runs a scriptable OpenID Connect issuer. It exists
// so the VibeXP e2e suite can sign real users in through an identity provider,
// end to end (browser → app → issuer → callback → session → refresh), without
// a real identity provider or any secret (issue #1650).
//
// It serves the fake issuer of internal/testutils/fakeoidc (discovery, JWKS,
// /authorize, /token with rotating refresh tokens, /userinfo) at --issuer, plus
// an unauthenticated control API a spec scripts it with:
//
//   - POST /_control/user        {"sub","email","claims"}: who the next
//     sign-in is, and the extra ID-token claims (e.g. "groups") from now on
//   - POST /_control/claims      {"name","value"}: set one extra claim; a null
//     value removes it
//   - POST /_control/refresh     {"error":"invalid_grant"} revokes every refresh
//     token; {"status":503} answers every refresh with that status, 0 restores
//   - POST /_control/authorize-error {"error":"access_denied"}: refuse every
//     authorization with that code; "" restores
//   - POST /_control/reset       back to the flags' user, no claims, no refusal
//   - GET  /_control/stats       {"refresh_calls"}: refresh_token grants served
//     since the process started, so a spec can see a re-check happen
//   - GET  /healthz              the compose healthcheck
//
// /authorize answers {"code","state"} (or {"error","state"}) as JSON instead
// of redirecting to the caller's redirect_uri, which would be an open redirect;
// the e2e helper turns that answer into the browser's redirect.
//
// The control API is unauthenticated, which is safe only because the issuer
// runs on the e2e stack alone and is never part of the shipped image.
//
// Run: go run ./cmd/oidc-test-issuer --issuer http://localhost:9003 --port 9003
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/vibexp/vibexp/internal/testutils/fakeoidc"
)

// maxControlBytes bounds a control request body.
const maxControlBytes = 64 << 10

// userRequest is the body of POST /_control/user.
type userRequest struct {
	Sub    string         `json:"sub"`
	Email  string         `json:"email"`
	Claims map[string]any `json:"claims"`
}

// claimRequest is the body of POST /_control/claims.
type claimRequest struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// refreshRequest is the body of POST /_control/refresh.
type refreshRequest struct {
	Error  string `json:"error"`
	Status int    `json:"status"`
}

// authorizeErrorRequest is the body of POST /_control/authorize-error.
type authorizeErrorRequest struct {
	Error string `json:"error"`
}

// newHandler builds the issuer's routes and the control API over it.
func newHandler(opts fakeoidc.Options) (http.Handler, error) {
	iss, issuer, err := fakeoidc.NewHandler(opts)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/", issuer)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /_control/user", control(func(req userRequest) error {
		if req.Sub == "" || req.Email == "" {
			return errors.New("sub and email are required")
		}
		iss.SetUser(req.Sub, req.Email)
		iss.SetClaims(req.Claims)
		return nil
	}))
	mux.HandleFunc("POST /_control/claims", control(func(req claimRequest) error {
		if req.Name == "" {
			return errors.New("name is required")
		}
		iss.SetClaim(req.Name, req.Value)
		return nil
	}))
	mux.HandleFunc("POST /_control/refresh", control(func(req refreshRequest) error {
		switch {
		case req.Error == "invalid_grant":
			iss.RevokeRefreshTokens()
		case req.Error != "":
			return fmt.Errorf("unsupported refresh error %q", req.Error)
		default:
			iss.SetRefreshStatus(req.Status)
		}
		return nil
	}))
	mux.HandleFunc("POST /_control/authorize-error", control(func(req authorizeErrorRequest) error {
		iss.SetAuthorizeError(req.Error)
		return nil
	}))
	mux.HandleFunc("GET /_control/stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]int64{"refresh_calls": iss.RefreshCalls()}); err != nil {
			log.Printf("write stats: %v", err)
		}
	})
	mux.HandleFunc("POST /_control/reset", func(w http.ResponseWriter, _ *http.Request) {
		iss.Reset()
		w.WriteHeader(http.StatusNoContent)
	})
	return mux, nil
}

// control decodes a JSON body into T and applies it, answering 204, or 400
// with the reason when the body or apply fails.
func control[T any](apply func(T) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req T
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxControlBytes)).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := apply(req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func main() {
	issuerURL := flag.String("issuer", "http://localhost:9003", "Issuer URL, as both the browser and the app reach it")
	port := flag.Int("port", 9003, "Port to listen on")
	clientID := flag.String("client-id", "e2e-oidc", "The only client ID accepted")
	clientSecret := flag.String("client-secret", "e2e-oidc-client-credential", "The only client secret accepted")
	subject := flag.String("subject", "e2e-oidc-user", "Subject of the sign-ins until /_control/user changes it")
	email := flag.String("email", "e2e-oidc-user@vibexp.test", "Email of the sign-ins until /_control/user changes it")
	flag.Parse()

	handler, err := newHandler(fakeoidc.Options{
		Issuer: *issuerURL, ClientID: *clientID, ClientSecret: *clientSecret, Subject: *subject, Email: *email,
	})
	if err != nil {
		log.Fatalf("failed to build the issuer: %v", err)
	}

	// Bind on all interfaces so the issuer is reachable through the port the
	// e2e stack publishes. It holds no real identity.
	addr := fmt.Sprintf(":%d", *port)
	listener, err := net.Listen("tcp", addr) // #nosec G102 -- throwaway e2e issuer, binds all interfaces by design
	if err != nil {
		log.Fatalf("failed to bind to port %d: %v", *port, err)
	}

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	log.Printf("oidc-test-issuer serving %s on :%d", *issuerURL, *port)
	if err := srv.Serve(listener); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
