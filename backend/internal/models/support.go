package models

// SupportRequest represents a support request from an authenticated user
type SupportRequest struct {
	Text            string            `json:"text" validate:"required,min=10,max=2000"`
	AdditionalInfo  map[string]string `json:"additional_info,omitempty"`
	Acknowledgement bool              `json:"acknowledgement"`
}

// SupportResponse represents the response for a support request
type SupportResponse struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}
