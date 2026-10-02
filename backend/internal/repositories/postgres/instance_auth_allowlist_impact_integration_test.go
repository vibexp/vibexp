//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Impact preview of a candidate access allowlist (#1235) against real Postgres.
// The SQL has to reach the same verdict as the resolver's in-memory matcher
// (services.compiledAllowlist), so the fixtures are that matcher's own edge
// cases: case and padding, sub/lookalike/superstring domains, and emails with
// no "@" or several.

func seedAllowlistImpactUser(t *testing.T, email, status string) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO users (id, email, name, status) VALUES ($1, $2, $3, $4)",
		uuid.New().String(), email, "Impact Fixture", status)
	require.NoError(t, err)
}

func TestIntegrationInstanceAuthAllowlist_CountUsersOutside(t *testing.T) {
	resetInstanceAuthSettings(t)
	ctx := context.Background()
	repo := NewInstanceAuthAllowlistRepository(integrationDB)

	for _, email := range []string{
		// Admitted by domain example.com.
		"dev@example.com", "Dev2@EXAMPLE.com", " padded@example.com ", `"x@y"@example.com`,
		// Admitted by exact email.
		"Guest@Other.com",
		// Exempt root admins.
		"root@corp.example", "ROOT2@corp.example",
		// Not admitted.
		"a@sub.example.com", "a@evil-example.com", "a@example.com.attacker.com",
		"not-an-email", "example.com", "outsider@elsewhere.test",
	} {
		seedAllowlistImpactUser(t, email, models.UserStatusActive)
	}
	// Suspended users cannot authenticate, so tightening the list costs them nothing.
	seedAllowlistImpactUser(t, "suspended@elsewhere.test", models.UserStatusSuspended)

	domains := []string{"example.com"}
	emails := []string{"guest@other.com"}
	exempt := []string{"root@corp.example", "root2@corp.example"}
	outside := []string{
		"a@evil-example.com", "a@example.com.attacker.com", "a@sub.example.com",
		"example.com", "not-an-email", "outsider@elsewhere.test",
	}

	t.Run("counts the active users matching neither list", func(t *testing.T) {
		count, sample, err := repo.CountUsersOutside(ctx, domains, emails, exempt, 20)
		require.NoError(t, err)
		assert.Equal(t, len(outside), count)
		assert.Equal(t, outside, sample, "alphabetical")
	})

	t.Run("caps the sample but not the count", func(t *testing.T) {
		count, sample, err := repo.CountUsersOutside(ctx, domains, emails, exempt, 3)
		require.NoError(t, err)
		assert.Equal(t, len(outside), count)
		assert.Equal(t, outside[:3], sample)
	})

	t.Run("without exemptions the root admins are counted", func(t *testing.T) {
		count, _, err := repo.CountUsersOutside(ctx, domains, emails, nil, 20)
		require.NoError(t, err)
		assert.Equal(t, len(outside)+2, count)
	})

	t.Run("an email-only candidate matches no domain", func(t *testing.T) {
		count, sample, err := repo.CountUsersOutside(ctx, nil, []string{"dev@example.com"}, nil, 1)
		require.NoError(t, err)
		assert.Equal(t, 12, count, "13 active users, one listed")
		assert.Len(t, sample, 1)
	})

	t.Run("nobody outside yields zero and an empty sample", func(t *testing.T) {
		count, sample, err := repo.CountUsersOutside(ctx,
			[]string{"example.com", "sub.example.com", "evil-example.com", "example.com.attacker.com",
				"elsewhere.test", "other.com", "corp.example"},
			[]string{"not-an-email", "example.com"}, nil, 20)
		require.NoError(t, err)
		assert.Zero(t, count)
		assert.Equal(t, []string{}, sample)
	})

	t.Run("it stores nothing", func(t *testing.T) {
		assert.Empty(t, authAuditEntries(t, models.InstanceSettingAuthAllowlist))
		_, err := repo.Get(ctx)
		assert.Error(t, err, "no allowlist row was written")
	})
}
