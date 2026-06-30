package services

import (
	"strings"

	"github.com/vibexp/vibexp/internal/config"
)

// EnvironmentService provides methods to detect and manage different environments
type EnvironmentService struct {
	config *config.Config
}

// NewEnvironmentService creates a new EnvironmentService instance
func NewEnvironmentService(cfg *config.Config) *EnvironmentService {
	return &EnvironmentService{
		config: cfg,
	}
}

// IsDevelopment checks if the application is running in development mode
// based on the FRONTEND_BASE_URL pointing at localhost or 127.0.0.1.
// Empty FRONTEND_BASE_URL is treated as NOT development (fail-closed) so
// misconfigured environments cannot accidentally enable dev-only paths. It
// delegates to config.IsLocalDevelopment, the single source of truth for the dev
// heuristic shared with the dev-only config derivation.
func (s *EnvironmentService) IsDevelopment() bool {
	return s.config.IsLocalDevelopment()
}

// IsProduction checks if the application is running in production mode
func (s *EnvironmentService) IsProduction() bool {
	return !s.IsDevelopment() && !s.IsStaging()
}

// IsStaging checks if the application is running in staging mode
// It checks if FRONTEND_BASE_URL contains "staging"
func (s *EnvironmentService) IsStaging() bool {
	url := strings.ToLower(s.config.Frontend.BaseURL)
	return strings.Contains(url, "staging")
}

// GetEnvironmentName returns the current environment name
func (s *EnvironmentService) GetEnvironmentName() string {
	if s.IsDevelopment() {
		return "development"
	}
	if s.IsStaging() {
		return "staging"
	}
	return "production"
}

// IsDevLoginEnabled determines if the /api/v1/auth/dev/login endpoint
// should respond. Both conditions must be true:
//  1. DEV_LOGIN_ENABLED env var is explicitly set to true (defaults to false).
//  2. The runtime environment is detected as development.
//
// This double-gate prevents a single env-var misconfiguration (e.g. a
// FRONTEND_BASE_URL accidentally containing "localhost") from exposing
// unauthenticated user impersonation in production.
func (s *EnvironmentService) IsDevLoginEnabled() bool {
	return s.config.Auth.DevLoginEnabled && s.IsDevelopment()
}
