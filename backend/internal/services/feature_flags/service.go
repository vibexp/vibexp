package feature_flags

import (
	"context"
	"log/slog"
	"sync"
)

// Structured-logging field values shared by this package's log lines.
const (
	logServiceName           = "vibexp-api"
	logComponentFeatureFlags = "feature-flags"
)

// FeatureFlagService is the core implementation of the feature flag system
type FeatureFlagService struct {
	flags  map[string]FeatureFlagEvaluator
	mu     sync.RWMutex
	logger *slog.Logger
}

// Ensure FeatureFlagService implements FeatureFlagServiceInterface
var _ FeatureFlagServiceInterface = (*FeatureFlagService)(nil)

// NewFeatureFlagService creates a new instance of FeatureFlagService
func NewFeatureFlagService(logger *slog.Logger) *FeatureFlagService {
	return &FeatureFlagService{
		flags:  make(map[string]FeatureFlagEvaluator),
		logger: logger,
	}
}

// RegisterFlag registers a new feature flag evaluator
// This method is thread-safe and can be called during initialization
func (s *FeatureFlagService) RegisterFlag(flag FeatureFlagEvaluator) {
	s.mu.Lock()
	defer s.mu.Unlock()

	flagName := flag.Name()
	s.flags[flagName] = flag

	s.logger.With(
		"service", logServiceName,
		"component", logComponentFeatureFlags,
		"flag_name", flagName,
	).Info("Feature flag registered")
}

// IsEnabled checks if a specific feature flag is enabled for the given context
// Returns false if the flag is not registered or evaluation errors occur
// This method is thread-safe and safe for concurrent use
func (s *FeatureFlagService) IsEnabled(ctx context.Context, flagName string) bool {
	s.mu.RLock()
	flag, exists := s.flags[flagName]
	s.mu.RUnlock()

	if !exists {
		s.logger.With(
			"service", logServiceName,
			"component", logComponentFeatureFlags,
			"flag_name", flagName,
		).Debug("Feature flag not registered, returning false")
		return false
	}

	// Evaluate the flag with error recovery
	enabled := s.evaluateFlag(ctx, flag)

	s.logger.With(
		"service", logServiceName,
		"component", logComponentFeatureFlags,
		"flag_name", flagName,
		"enabled", enabled,
	).Debug("Feature flag evaluated")

	return enabled
}

// evaluateFlag safely evaluates a flag with panic recovery
func (s *FeatureFlagService) evaluateFlag(ctx context.Context, flag FeatureFlagEvaluator) bool {
	defer func() {
		if r := recover(); r != nil {
			s.logger.With(
				"service", logServiceName,
				"component", logComponentFeatureFlags,
				"flag_name", flag.Name(),
				"panic", r,
			).Error("Feature flag evaluation panicked, returning false")
		}
	}()

	return flag.Evaluate(ctx)
}
