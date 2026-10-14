package providers

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// #1485: a resolver that cannot be invalidated fails the start rather than
// silently bringing back the 2 s window after every write.
func TestProvideAuthSettingsInvalidator(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	policy := services.NewAuthPolicyResolver(services.AuthPolicyResolverDeps{Logger: logger})
	allowlist := services.NewAccessAllowlistResolver(services.AccessAllowlistResolverDeps{Logger: logger})

	inv, err := ProvideAuthSettingsInvalidator(policy, allowlist)
	require.NoError(t, err)
	assert.NotNil(t, inv)

	_, err = ProvideAuthSettingsInvalidator(servicesmocks.NewMockAuthPolicyResolver(t), allowlist)
	assert.ErrorContains(t, err, "auth policy resolver")

	_, err = ProvideAuthSettingsInvalidator(policy, servicesmocks.NewMockAccessAllowlistResolver(t))
	assert.ErrorContains(t, err, "access allowlist resolver")
}
