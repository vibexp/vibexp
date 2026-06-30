package events

import "time"

// Config holds event bus configuration. It is populated from the event_bus
// section of config.yaml (the koanf tags name the keys).
// This configuration is separate from EventBusConfig which includes runtime dependencies
// like Logger and Metrics that are injected via dependency injection.
type Config struct {
	// WorkerCount is the number of concurrent workers processing events
	// Set via event_bus.worker_count (default: 20)
	// Optimized for production use; reduce to 5-10 for development
	WorkerCount int `koanf:"worker_count"`

	// BufferSize is the event queue buffer size before blocking publishers
	// Set via event_bus.buffer_size (default: 500)
	// Handles traffic bursts; reduce to 100 for development
	BufferSize int `koanf:"buffer_size"`

	// MaxRetries is the maximum retry attempts for failed event handlers
	// Set via event_bus.max_retries (default: 3)
	// Balanced for reliability without excessive delays
	MaxRetries int `koanf:"max_retries"`

	// RetryBackoff is the base delay between retries (exponential backoff)
	// Set via event_bus.retry_backoff (default: 200ms)
	// Formula: delay = backoff * 2^attempt (e.g., 200ms -> 400ms -> 800ms)
	// Optimized to give external services breathing room
	RetryBackoff time.Duration `koanf:"retry_backoff"`

	// RetryJitter adds randomness to backoff to prevent thundering herd
	// Set via event_bus.retry_jitter (default: true)
	// Adds ±10% randomness to prevent simultaneous retries across instances
	// Essential for multi-instance production deployments
	RetryJitter bool `koanf:"retry_jitter"`
}
