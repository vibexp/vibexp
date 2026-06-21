package feature_flags

import "context"

// FeatureFlagEvaluator defines the interface for individual feature flag implementations
type FeatureFlagEvaluator interface {
	// Evaluate checks if the feature flag is enabled for the given context
	Evaluate(ctx context.Context) bool

	// Name returns the unique identifier for this feature flag
	Name() string
}

// FeatureFlagServiceInterface defines the interface for the feature flag service
type FeatureFlagServiceInterface interface {
	// IsEnabled checks if a specific feature flag is enabled for the given context
	IsEnabled(ctx context.Context, flagName string) bool

	// RegisterFlag registers a new feature flag evaluator
	RegisterFlag(flag FeatureFlagEvaluator)
}
