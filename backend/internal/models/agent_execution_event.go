package models

import "time"

// AgentExecutionEvent represents an event in the agent execution streaming process
type AgentExecutionEvent struct {
	ID             string                 `json:"id" db:"id"`
	ExecutionID    string                 `json:"execution_id" db:"execution_id"`
	EventType      string                 `json:"event_type" db:"event_type"` // task, status-update, artifact-update
	EventData      map[string]interface{} `json:"event_data" db:"event_data"`
	SequenceNumber int                    `json:"sequence_number" db:"sequence_number"`
	ReceivedAt     time.Time              `json:"received_at" db:"received_at"`
}

// AgentExecutionEventListResponse represents a paginated list of agent execution events
type AgentExecutionEventListResponse struct {
	Events     []AgentExecutionEvent `json:"events"`
	TotalCount int                   `json:"total_count"`
	Page       int                   `json:"page"`
	Limit      int                   `json:"limit"`
}
