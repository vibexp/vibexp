package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/api/idtoken"

	"github.com/vibexp/vibexp/internal/contextkeys"
)

// pubSubOIDCMiddleware validates OIDC tokens from Google Cloud Pub/Sub push subscriptions
func (s *Server) pubSubOIDCMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := s.extractPubSubToken(r)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		payload, err := s.validateOIDCToken(r, token)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		email, err := s.verifyServiceAccount(r, payload)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		s.logger.With(
			"service", "vibexp-api",
			"middleware", "pubsub-oidc",
			"path", r.URL.Path,
			"service_account", email,
			"issuer", payload.Issuer,
		).Debug("OIDC token validated successfully")

		ctx := context.WithValue(r.Context(), contextkeys.PubSubServiceAccount, email)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) extractPubSubToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		s.logger.With(
			"service", "vibexp-api",
			"middleware", "pubsub-oidc",
			"path", r.URL.Path,
		).Warn("Missing Authorization header")
		return "", http.ErrNoCookie
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == authHeader {
		s.logger.With(
			"service", "vibexp-api",
			"middleware", "pubsub-oidc",
			"path", r.URL.Path,
		).Warn("Invalid Authorization header format")
		return "", http.ErrNoCookie
	}

	return token, nil
}

func (s *Server) validateOIDCToken(r *http.Request, token string) (*idtoken.Payload, error) {
	audience := s.config.PubSubPushAudience
	payload, err := idtoken.Validate(context.Background(), token, audience)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"middleware", "pubsub-oidc",
			"path", r.URL.Path,
			"error", fmt.Sprintf("%+v", err),
		).Error("OIDC token validation failed")
		return nil, err
	}

	if payload.Issuer != "https://accounts.google.com" {
		s.logger.With(
			"service", "vibexp-api",
			"middleware", "pubsub-oidc",
			"path", r.URL.Path,
			"issuer", payload.Issuer,
		).Error("Invalid token issuer")
		return nil, http.ErrNoCookie
	}

	return payload, nil
}

func (s *Server) verifyServiceAccount(r *http.Request, payload *idtoken.Payload) (string, error) {
	email, ok := payload.Claims["email"].(string)
	if !ok {
		s.logger.With(
			"service", "vibexp-api",
			"middleware", "pubsub-oidc",
			"path", r.URL.Path,
		).Error("Missing email in token claims")
		return "", http.ErrNoCookie
	}

	// Only accept tokens minted by a service account in the project's IAM domain.
	// The expected suffix (e.g. "@<project>.iam.gserviceaccount.com") is supplied
	// via PUBSUB_PUSH_SERVICE_ACCOUNT_SUFFIX so no tenant-specific identity is
	// hardcoded. The broad "@accounts.google.com" suffix is intentionally NOT
	// accepted: it would admit any Google-issued identity, not just the project's
	// Pub/Sub push SA. When the suffix is empty, this check is skipped (issuer,
	// signature and audience are still enforced upstream).
	if suffix := s.config.PubSubPushServiceAccountSuffix; suffix != "" && !strings.HasSuffix(email, suffix) {
		s.logger.With(
			"service", "vibexp-api",
			"middleware", "pubsub-oidc",
			"path", r.URL.Path,
			"service_account", email,
		).Warn("Service account not from expected project")
		return "", http.ErrNoCookie
	}

	return email, nil
}
