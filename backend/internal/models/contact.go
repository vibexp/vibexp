package models

import (
	"time"
)

type ContactMessage struct {
	ID          int       `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Email       string    `json:"email" db:"email"`
	PhoneNumber *string   `json:"phone_number,omitempty" db:"phone_number"`
	Message     string    `json:"message" db:"message"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type ContactFormRequest struct {
	Name        string  `json:"name" validate:"required,min=2,max=100"`
	Email       string  `json:"email" validate:"required,email"`
	PhoneNumber *string `json:"phone_number,omitempty" validate:"omitempty,min=10,max=20"`
	Message     string  `json:"message" validate:"required,min=10,max=1000"`
}

type ContactFormResponse struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}
