package events

import "time"

// Config holds event bus configuration loaded from environment variables.
// This configuration is separate from EventBusConfig which includes runtime dependencies
// like Logger and Metrics that are injected via dependency injection.
type Config struct {
	// WorkerCount is the number of concurrent workers processing events
	// Set via EVENT_BUS_WORKER_COUNT environment variable (default: 20)
	// Optimized for production use; reduce to 5-10 for development
	WorkerCount int `envconfig:"EVENT_BUS_WORKER_COUNT" default:"20"`

	// BufferSize is the event queue buffer size before blocking publishers
	// Set via EVENT_BUS_BUFFER_SIZE environment variable (default: 500)
	// Handles traffic bursts; reduce to 100 for development
	BufferSize int `envconfig:"EVENT_BUS_BUFFER_SIZE" default:"500"`

	// MaxRetries is the maximum retry attempts for failed event handlers
	// Set via EVENT_BUS_MAX_RETRIES environment variable (default: 3)
	// Balanced for reliability without excessive delays
	MaxRetries int `envconfig:"EVENT_BUS_MAX_RETRIES" default:"3"`

	// RetryBackoff is the base delay between retries (exponential backoff)
	// Set via EVENT_BUS_RETRY_BACKOFF environment variable (default: 200ms)
	// Formula: delay = backoff * 2^attempt (e.g., 200ms -> 400ms -> 800ms)
	// Optimized to give external services breathing room
	RetryBackoff time.Duration `envconfig:"EVENT_BUS_RETRY_BACKOFF" default:"200ms"`

	// RetryJitter adds randomness to backoff to prevent thundering herd
	// Set via EVENT_BUS_RETRY_JITTER environment variable (default: true)
	// Adds ±10% randomness to prevent simultaneous retries across instances
	// Essential for multi-instance production deployments
	RetryJitter bool `envconfig:"EVENT_BUS_RETRY_JITTER" default:"true"`
}
