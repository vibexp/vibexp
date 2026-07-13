package models

import "time"

// InvitationStatus represents the status of a team invitation
type InvitationStatus string

const (
	// InvitationStatusPending represents a pending invitation
	InvitationStatusPending InvitationStatus = "pending"
	// InvitationStatusAccepted represents an accepted invitation
	InvitationStatusAccepted InvitationStatus = "accepted"
	// InvitationStatusRejected represents a rejected invitation
	InvitationStatusRejected InvitationStatus = "rejected"
	// InvitationStatusRevoked represents a revoked invitation
	InvitationStatusRevoked InvitationStatus = "revoked"
)

// IsValid checks if the status is a valid InvitationStatus
func (s InvitationStatus) IsValid() bool {
	switch s {
	case InvitationStatusPending, InvitationStatusAccepted, InvitationStatusRejected, InvitationStatusRevoked:
		return true
	default:
		return false
	}
}

// String returns the string representation of the status
func (s InvitationStatus) String() string {
	return string(s)
}

// TeamInvitation represents an invitation to join a team
type TeamInvitation struct {
	ID           string           `json:"id" db:"id"`
	TeamID       string           `json:"team_id" db:"team_id"`
	InviterID    string           `json:"inviter_id" db:"inviter_id"`
	InviteeEmail string           `json:"invitee_email" db:"invitee_email"`
	Role         TeamMemberRole   `json:"role" db:"role"`
	Token        string           `json:"token" db:"token"`
	Status       InvitationStatus `json:"status" db:"status"`
	ExpiresAt    time.Time        `json:"expires_at" db:"expires_at"`
	CreatedAt    time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at" db:"updated_at"`
}

// SendInvitationsRequest represents the request body for sending team invitations
type SendInvitationsRequest struct {
	Emails []string `json:"emails" validate:"required,min=1,max=50,dive,email"`
	Role   string   `json:"role" validate:"required,oneof=member admin"`
}

// InviterInfo represents information about the user who sent the invitation
type InviterInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// InvitationResponse represents the response for a team invitation
type InvitationResponse struct {
	ID           string       `json:"id"`
	Token        string       `json:"token"`
	TeamID       string       `json:"team_id"`
	TeamName     string       `json:"team_name"`
	InviteeEmail string       `json:"invitee_email"`
	Role         string       `json:"role"`
	Status       string       `json:"status"`
	ExpiresAt    string       `json:"expires_at"`
	CreatedAt    string       `json:"created_at"`
	InvitedBy    *InviterInfo `json:"invited_by,omitempty"`
}

// PendingInvitationsListResponse represents the response for listing pending invitations
type PendingInvitationsListResponse struct {
	Invitations JSONArray[InvitationResponse] `json:"invitations"`
	TotalCount  int                           `json:"total_count"`
	Page        int                           `json:"page"`
	PageSize    int                           `json:"page_size"`
}

// InvitationDetailsResponse wraps a single invitation for the GET /api/v1/invitations/{token} endpoint.
type InvitationDetailsResponse struct {
	Invitation InvitationResponse `json:"invitation"`
}

// AcceptInvitationResponse represents the response for accepting a team invitation
type AcceptInvitationResponse struct {
	TeamID   string `json:"team_id"`
	TeamName string `json:"team_name"`
	Message  string `json:"message"`
}
