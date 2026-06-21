package models

import (
	"time"
)

// ResourceUsageItem represents usage for a single resource type
type ResourceUsageItem struct {
	ResourceType    string `json:"resource_type"`
	Count           int    `json:"count"`
	Limit           int    `json:"limit"`
	IndividualLimit int    `json:"individual_limit"`
	TeamQuota       int    `json:"team_quota"`
	Percentage      int    `json:"percentage"`
}

// ResourceUsageResponse represents resource usage information
type ResourceUsageResponse struct {
	UserID      string              `json:"user_id"`
	PeriodStart time.Time           `json:"period_start"`
	PeriodEnd   time.Time           `json:"period_end"`
	Resources   []ResourceUsageItem `json:"resources"`
}
