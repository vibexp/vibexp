package services

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/internal/config"
)

func TestEnvironmentService_IsDevelopment(t *testing.T) {
	tests := []struct {
		name            string
		frontendBaseURL string
		expected        bool
	}{
		{
			name:            "localhost URL",
			frontendBaseURL: "http://localhost:5173",
			expected:        true,
		},
		{
			name:            "127.0.0.1 URL",
			frontendBaseURL: "http://127.0.0.1:5173",
			expected:        true,
		},
		{
			name:            "localhost with HTTPS",
			frontendBaseURL: "https://localhost:5173",
			expected:        true,
		},
		{
			name:            "production URL",
			frontendBaseURL: "https://app.vibexp.io",
			expected:        false,
		},
		{
			name:            "staging URL",
			frontendBaseURL: "https://staging.vibexp.io",
			expected:        false,
		},
		{
			name:            "uppercase localhost",
			frontendBaseURL: "http://LOCALHOST:5173",
			expected:        true,
		},
		{
			name:            "empty URL is not development (fail-closed)",
			frontendBaseURL: "",
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				FrontendBaseURL: tt.frontendBaseURL,
			}
			service := NewEnvironmentService(cfg)
			result := service.IsDevelopment()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnvironmentService_IsStaging(t *testing.T) {
	tests := []struct {
		name            string
		frontendBaseURL string
		expected        bool
	}{
		{
			name:            "staging URL",
			frontendBaseURL: "https://staging.vibexp.io",
			expected:        true,
		},
		{
			name:            "staging subdomain",
			frontendBaseURL: "https://staging-app.vibexp.io",
			expected:        true,
		},
		{
			name:            "uppercase staging",
			frontendBaseURL: "https://STAGING.vibexp.io",
			expected:        true,
		},
		{
			name:            "production URL",
			frontendBaseURL: "https://app.vibexp.io",
			expected:        false,
		},
		{
			name:            "localhost URL",
			frontendBaseURL: "http://localhost:5173",
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				FrontendBaseURL: tt.frontendBaseURL,
			}
			service := NewEnvironmentService(cfg)
			result := service.IsStaging()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnvironmentService_IsProduction(t *testing.T) {
	tests := []struct {
		name            string
		frontendBaseURL string
		expected        bool
	}{
		{
			name:            "production URL",
			frontendBaseURL: "https://app.vibexp.io",
			expected:        true,
		},
		{
			name:            "localhost URL",
			frontendBaseURL: "http://localhost:5173",
			expected:        false,
		},
		{
			name:            "staging URL",
			frontendBaseURL: "https://staging.vibexp.io",
			expected:        false,
		},
		{
			name:            "127.0.0.1 URL",
			frontendBaseURL: "http://127.0.0.1:5173",
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				FrontendBaseURL: tt.frontendBaseURL,
			}
			service := NewEnvironmentService(cfg)
			result := service.IsProduction()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnvironmentService_GetEnvironmentName(t *testing.T) {
	tests := []struct {
		name            string
		frontendBaseURL string
		expected        string
	}{
		{
			name:            "development localhost",
			frontendBaseURL: "http://localhost:5173",
			expected:        "development",
		},
		{
			name:            "development 127.0.0.1",
			frontendBaseURL: "http://127.0.0.1:5173",
			expected:        "development",
		},
		{
			name:            "staging environment",
			frontendBaseURL: "https://staging.vibexp.io",
			expected:        "staging",
		},
		{
			name:            "production environment",
			frontendBaseURL: "https://app.vibexp.io",
			expected:        "production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				FrontendBaseURL: tt.frontendBaseURL,
			}
			service := NewEnvironmentService(cfg)
			result := service.GetEnvironmentName()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnvironmentService_IsDevLoginEnabled(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		flag     bool
		expected bool
	}{
		{"enabled with flag and localhost", "http://localhost:5173", true, true},
		{"enabled with flag and 127.0.0.1", "http://127.0.0.1:5173", true, true},
		{"disabled when localhost but flag off", "http://localhost:5173", false, false},
		{"disabled with flag on but staging URL", "https://staging.vibexp.io", true, false},
		{"disabled with flag on but production URL", "https://app.vibexp.io", true, false},
		{"disabled in staging with flag off", "https://staging.vibexp.io", false, false},
		{"disabled with empty URL even with flag on", "", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{FrontendBaseURL: tt.url, DevLoginEnabled: tt.flag}
			service := NewEnvironmentService(cfg)
			assert.Equal(t, tt.expected, service.IsDevLoginEnabled())
		})
	}
}
