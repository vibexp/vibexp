package models

import (
	"encoding/json"
	"time"
)

type PromptGalleryTemplate struct {
	ID          string          `json:"id" db:"id"`
	Title       string          `json:"title" db:"title"`
	Description string          `json:"description" db:"description"`
	Content     string          `json:"content" db:"content"`
	Category    string          `json:"category" db:"category"`
	Tags        json.RawMessage `json:"tags" db:"tags"`
	Metadata    json.RawMessage `json:"metadata" db:"metadata"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

type PromptGalleryCategory struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

type PromptGalleryListResponse struct {
	Prompts    []PromptGalleryTemplate `json:"prompts"`
	TotalCount int                     `json:"total_count"`
	Page       int                     `json:"page"`
	PerPage    int                     `json:"per_page"`
	TotalPages int                     `json:"total_pages"`
}

type PromptGalleryUsageRequest struct {
	PromptID string `json:"prompt_id" validate:"required"`
}
