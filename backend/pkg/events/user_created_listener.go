package events

import (
	"context"
	"log/slog"

	"github.com/vibexp/vibexp/internal/logging"
)

// UserCreatedListener handles user.created events
type UserCreatedListener struct {
	logger *slog.Logger
}

// NewUserCreatedListener creates a new user created listener
func NewUserCreatedListener(logger *slog.Logger) *UserCreatedListener {
	if logger == nil {
		logger = logging.New(logging.Config{})
	}
	return &UserCreatedListener{
		logger: logger,
	}
}

// Handle processes the user.created event
func (l *UserCreatedListener) Handle(ctx context.Context, event Event) error {
	l.logger.With(
		"service", "vibexp-api",
		"component", "user-created-listener",
	).Info("user.created event received and processed locally")

	return nil
}

// EventTypes returns the event types this listener handles
func (l *UserCreatedListener) EventTypes() []string {
	return []string{EventTypeUserCreated}
}
