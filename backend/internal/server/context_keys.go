package server

import "github.com/vibexp/vibexp/internal/contextkeys"

// Backward compatibility aliases - all context keys now use the shared contextkeys package
// These constants reference the shared package to ensure type consistency across the codebase
const (
	contextKeyUser        = contextkeys.User
	contextKeyUserID      = contextkeys.UserID
	contextKeyUserEmail   = contextkeys.UserEmail
	contextKeyAgentID     = contextkeys.AgentID
	contextKeyExecutionID = contextkeys.ExecutionID
)
