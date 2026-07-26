package models

import (
	"time"
)

// Integration codes (replacing usage type constants)
const (
	IntegrationCodeCLI       = "cli"
	IntegrationCodeMCPServer = "mcp_server"
)

// Legacy usage type constants (kept for backward compatibility)
const (
	UsageTypeCLI        = "cli"
	UsageTypeMCP        = "mcp"
	UsageTypeEverything = "everything"
)

// API key prefixes - now unified
const (
	// Legacy prefixes (deprecated).
	//
	// PrefixAITools MUST NOT be removed even though the `ai_tools` integration scope
	// was retired with the hook feature (#614). middleware.go uses it to recognise a
	// bearer token as an API key at all, so deleting it would 401 every existing
	// `aait-`-prefixed key. The prefix is orthogonal to the scope.
	PrefixAITools    = "aait-"
	PrefixCLI        = "acli-"
	PrefixMCP        = "amcp-"
	PrefixEverything = "ak_"
	// New unified prefix
	PrefixVibeXPKey = "vxk_"
)

// ValidUsageTypes returns all valid usage types (legacy)
func ValidUsageTypes() []string {
	return []string{
		UsageTypeCLI,
		UsageTypeMCP,
		UsageTypeEverything,
	}
}

// ValidIntegrationCodes returns all valid integration codes
func ValidIntegrationCodes() []string {
	return []string{
		IntegrationCodeCLI,
		IntegrationCodeMCPServer,
	}
}

// APIKeyIntegration represents an available integration type
type APIKeyIntegration struct {
	ID              string    `json:"id" db:"id"`
	IntegrationCode string    `json:"integration_code" db:"integration_code"`
	IntegrationName string    `json:"integration_name" db:"integration_name"`
	Description     string    `json:"description" db:"description"`
	IsActive        bool      `json:"is_active" db:"is_active"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

// APIKeyIntegrationPermission represents a granted permission for an API key
type APIKeyIntegrationPermission struct {
	ID              string    `json:"id" db:"id"`
	APIKeyID        string    `json:"api_key_id" db:"api_key_id"`
	IntegrationCode string    `json:"integration_code" db:"integration_code"`
	GrantedAt       time.Time `json:"granted_at" db:"granted_at"`
}

type APIKey struct {
	ID             string            `json:"id" db:"id"`
	UserID         string            `json:"user_id" db:"user_id"`
	Name           string            `json:"name" db:"name"`
	KeyHash        string            `json:"-" db:"key_hash"`
	KeyPrefix      string            `json:"key_prefix" db:"key_prefix"`
	UsageType      string            `json:"usage_type,omitempty" db:"usage_type"` // Deprecated, kept for backward compat
	Integrations   JSONArray[string] `json:"integrations" db:"-"`                  // Array of integration codes
	IsLegacy       bool              `json:"is_legacy" db:"is_legacy"`
	MigrationNotes *string           `json:"migration_notes,omitempty" db:"migration_notes"`
	LastUsedAt     *time.Time        `json:"last_used_at" db:"last_used_at"`
	ExpiresAt      *time.Time        `json:"expires_at" db:"expires_at"` // nil means the key never expires
	CreatedAt      time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at" db:"updated_at"`
	Version        int64             `json:"version" db:"version"`
}

type CreateAPIKeyRequest struct {
	Name             string   `json:"name" validate:"required,min=1,max=255"`
	IntegrationCodes []string `json:"integration_codes" validate:"required,min=1"`
}

type CreateAPIKeyResponse struct {
	APIKey    APIKey `json:"api_key"`
	FullKey   string `json:"full_key"`
	KeyPrefix string `json:"key_prefix"`
}
