package feature_flags

import (
	"context"
	"log/slog"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockFeatureFlag is a simple mock for testing
type mockFeatureFlag struct {
	name    string
	enabled bool
}

func (m *mockFeatureFlag) Name() string {
	return m.name
}

func (m *mockFeatureFlag) Evaluate(ctx context.Context) bool {
	return m.enabled
}

func TestNewFeatureFlagService(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	service := NewFeatureFlagService(logger)

	assert.NotNil(t, service)
	assert.NotNil(t, service.flags)
	assert.NotNil(t, service.logger)
}

func TestFeatureFlagService_RegisterFlag(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	service := NewFeatureFlagService(logger)

	flag := &mockFeatureFlag{name: "test_flag", enabled: true}
	service.RegisterFlag(flag)

	assert.Len(t, service.flags, 1)
	assert.Contains(t, service.flags, "test_flag")
	assert.Equal(t, flag, service.flags["test_flag"])
}

func TestFeatureFlagService_RegisterMultipleFlags(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	service := NewFeatureFlagService(logger)

	flag1 := &mockFeatureFlag{name: "flag_1", enabled: true}
	flag2 := &mockFeatureFlag{name: "flag_2", enabled: false}
	flag3 := &mockFeatureFlag{name: "flag_3", enabled: true}

	service.RegisterFlag(flag1)
	service.RegisterFlag(flag2)
	service.RegisterFlag(flag3)

	assert.Len(t, service.flags, 3)
	assert.Contains(t, service.flags, "flag_1")
	assert.Contains(t, service.flags, "flag_2")
	assert.Contains(t, service.flags, "flag_3")
}

func TestFeatureFlagService_IsEnabled_FlagExists(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	service := NewFeatureFlagService(logger)

	tests := []struct {
		name     string
		flag     *mockFeatureFlag
		expected bool
	}{
		{
			name:     "enabled flag",
			flag:     &mockFeatureFlag{name: "enabled_flag", enabled: true},
			expected: true,
		},
		{
			name:     "disabled flag",
			flag:     &mockFeatureFlag{name: "disabled_flag", enabled: false},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service.RegisterFlag(tt.flag)
			ctx := context.Background()
			result := service.IsEnabled(ctx, tt.flag.Name())
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFeatureFlagService_IsEnabled_FlagNotRegistered(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	service := NewFeatureFlagService(logger)

	ctx := context.Background()
	result := service.IsEnabled(ctx, "non_existent_flag")
	assert.False(t, result)
}

func TestFeatureFlagService_IsEnabled_OverwriteFlag(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	service := NewFeatureFlagService(logger)

	// Register initial flag
	flag1 := &mockFeatureFlag{name: "test_flag", enabled: true}
	service.RegisterFlag(flag1)

	ctx := context.Background()
	assert.True(t, service.IsEnabled(ctx, "test_flag"))

	// Overwrite with new flag
	flag2 := &mockFeatureFlag{name: "test_flag", enabled: false}
	service.RegisterFlag(flag2)

	assert.False(t, service.IsEnabled(ctx, "test_flag"))
}

// panicFlag is a mock that panics during evaluation
type panicFlag struct {
	name string
}

func (p *panicFlag) Name() string {
	return p.name
}

func (p *panicFlag) Evaluate(ctx context.Context) bool {
	panic("test panic")
}

func TestFeatureFlagService_IsEnabled_PanicRecovery(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	service := NewFeatureFlagService(logger)

	flag := &panicFlag{name: "panic_flag"}
	service.RegisterFlag(flag)

	ctx := context.Background()
	result := service.IsEnabled(ctx, "panic_flag")

	// Should return false on panic
	assert.False(t, result)
}

func TestFeatureFlagService_ConcurrentAccess(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	service := NewFeatureFlagService(logger)

	flag := &mockFeatureFlag{name: "concurrent_flag", enabled: true}
	service.RegisterFlag(flag)

	ctx := context.Background()
	done := make(chan bool, 10)

	// Spawn multiple goroutines to test concurrent access
	for i := 0; i < 10; i++ {
		go func() {
			result := service.IsEnabled(ctx, "concurrent_flag")
			assert.True(t, result)
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestFeatureFlagService_ConcurrentRegistration(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	service := NewFeatureFlagService(logger)

	done := make(chan bool, 10)

	// Spawn multiple goroutines to register flags concurrently
	for i := 0; i < 10; i++ {
		i := i
		go func() {
			flag := &mockFeatureFlag{name: "flag_" + strconv.Itoa(i), enabled: true}
			service.RegisterFlag(flag)
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	assert.Len(t, service.flags, 10)
}
