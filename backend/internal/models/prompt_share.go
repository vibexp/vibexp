package models

import "time"

type PromptShare struct {
	ID          string     `json:"id" db:"id"`
	PromptID    string     `json:"prompt_id" db:"prompt_id"`
	ShareToken  string     `json:"share_token" db:"share_token"`
	ShareType   string     `json:"share_type" db:"share_type"` // "public" or "restricted"
	CreatedBy   string     `json:"created_by" db:"created_by"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	IsActive    bool       `json:"is_active" db:"is_active"`
	AccessCount int        `json:"access_count" db:"access_count"`
	Version     int64      `json:"version" db:"version"`
}

type PromptShareAccess struct {
	ID        string    `json:"id" db:"id"`
	ShareID   string    `json:"share_id" db:"share_id"`
	Email     string    `json:"email" db:"email"`
	GrantedAt time.Time `json:"granted_at" db:"granted_at"`
}

type CreateShareRequest struct {
	ShareType string   `json:"share_type" validate:"required,oneof=public restricted"`
	Emails    []string `json:"emails,omitempty" validate:"omitempty,dive,email"`
}

type ShareResponse struct {
	ShareToken string    `json:"share_token"`
	ShareURL   string    `json:"share_url"`
	ShareType  string    `json:"share_type"`
	Emails     []string  `json:"emails,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type SharedPromptResponse struct {
	Prompt       Prompt `json:"prompt"`
	ShareType    string `json:"share_type"`
	RenderedBody string `json:"rendered_body"`
}
