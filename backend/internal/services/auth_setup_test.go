package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// memSetupRepo is an in-memory InstanceAuthSetupRepository with the stored
// repository's semantics: one row, mint-unless-live, generation bumps.
type memSetupRepo struct {
	mu       sync.Mutex
	row      *models.InstanceAuthSetup
	getErr   error
	writeErr error
	consumes []string
}

func (m *memSetupRepo) Get(context.Context) (*models.InstanceAuthSetup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.row == nil {
		return nil, repositories.ErrInstanceAuthSetupNotFound
	}
	cp := *m.row
	return &cp, nil
}

func (m *memSetupRepo) mint(hash []byte, expiresAt time.Time, rearm bool) *models.InstanceAuthSetup {
	next := &models.InstanceAuthSetup{TokenHash: hash, ExpiresAt: &expiresAt, Rearmed: rearm, Generation: 1,
		UpdatedAt: expiresAt.Add(-SetupTokenLifetime)}
	if m.row != nil {
		next.Generation = m.row.Generation + 1
		next.Rearmed = m.row.Rearmed || rearm
	}
	m.row = next
	cp := *next
	return &cp
}

func (m *memSetupRepo) MintIfAbsentOrExpired(
	_ context.Context, hash []byte, expiresAt, now time.Time,
) (*models.InstanceAuthSetup, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return nil, false, m.writeErr
	}
	if m.row != nil && m.row.HasLiveToken(now) {
		cp := *m.row
		return &cp, false, nil
	}
	return m.mint(hash, expiresAt, false), true, nil
}

func (m *memSetupRepo) ForceMint(
	_ context.Context, hash []byte, expiresAt time.Time,
) (*models.InstanceAuthSetup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return nil, m.writeErr
	}
	return m.mint(hash, expiresAt, true), nil
}

func (m *memSetupRepo) MintReplacing(
	_ context.Context, hash []byte, expiresAt time.Time,
) (*models.InstanceAuthSetup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return nil, m.writeErr
	}
	return m.mint(hash, expiresAt, false), nil
}

func (m *memSetupRepo) Consume(_ context.Context, userID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return false, m.writeErr
	}
	if m.row == nil || m.row.IsConsumed() {
		return false, nil
	}
	now := time.Now()
	m.row.TokenHash, m.row.ConsumedAt, m.row.ConsumedBy = nil, &now, &userID
	m.row.Rearmed = false
	m.row.Generation++
	m.consumes = append(m.consumes, userID)
	return true, nil
}

// setupProviderRows is an InstanceAuthProviderRepository serving fixed rows.
type setupProviderRows struct {
	repositories.InstanceAuthProviderRepository
	rows []*models.InstanceAuthProvider
	err  error
}

func (p *setupProviderRows) List(context.Context) ([]*models.InstanceAuthProvider, error) {
	return p.rows, p.err
}

type setupFixture struct {
	svc       *setupModeService
	repo      *memSetupRepo
	providers *setupProviderRows
	logs      *bytes.Buffer
	now       time.Time
}

const setupRootEmail = "root@example.com"

func newSetupFixture(t *testing.T, recoveryMode bool) *setupFixture {
	t.Helper()
	f := &setupFixture{
		repo: &memSetupRepo{}, providers: &setupProviderRows{}, logs: &bytes.Buffer{},
		now: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC),
	}
	f.svc = newSetupModeService(SetupModeDeps{
		Setup:        f.repo,
		Providers:    f.providers,
		IsRootAdmin:  func(email string) bool { return strings.EqualFold(email, setupRootEmail) },
		BaseURL:      "https://vibexp.example.com/",
		RecoveryMode: recoveryMode,
		Logger:       slog.New(slog.NewTextHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelInfo})),
	})
	f.svc.now = func() time.Time { return f.now }
	return f
}

func (f *setupFixture) enableProvider() {
	f.providers.rows = []*models.InstanceAuthProvider{{Slug: "off", Enabled: false}, {Slug: "sso", Enabled: true}}
}

// bootToken boots the service and returns the minted token.
func (f *setupFixture) bootToken(t *testing.T) string {
	t.Helper()
	setupURL, minted, err := f.svc.EnsureTokenAtBoot(context.Background())
	require.NoError(t, err)
	require.True(t, minted)
	const prefix = "https://vibexp.example.com/setup?token="
	require.True(t, strings.HasPrefix(setupURL, prefix), setupURL)
	return strings.TrimPrefix(setupURL, prefix)
}

func TestSetupMode_IsActive(t *testing.T) {
	ctx := context.Background()

	t.Run("no provider rows", func(t *testing.T) {
		f := newSetupFixture(t, false)
		active, err := f.svc.IsActive(ctx)
		require.NoError(t, err)
		assert.True(t, active)
	})

	t.Run("only disabled providers", func(t *testing.T) {
		f := newSetupFixture(t, false)
		f.providers.rows = []*models.InstanceAuthProvider{{Slug: "off", Enabled: false}}
		active, err := f.svc.IsActive(ctx)
		require.NoError(t, err)
		assert.True(t, active)
	})

	t.Run("an enabled provider ends it", func(t *testing.T) {
		f := newSetupFixture(t, false)
		f.enableProvider()
		active, err := f.svc.IsActive(ctx)
		require.NoError(t, err)
		assert.False(t, active)
	})

	t.Run("an outstanding token alone does not keep it active once a provider is enabled", func(t *testing.T) {
		f := newSetupFixture(t, false)
		f.bootToken(t)
		f.enableProvider()
		active, err := f.svc.IsActive(ctx)
		require.NoError(t, err)
		assert.False(t, active)
	})

	t.Run("recovery mode forces it on", func(t *testing.T) {
		f := newSetupFixture(t, true)
		f.enableProvider()
		active, err := f.svc.IsActive(ctx)
		require.NoError(t, err)
		assert.True(t, active)
	})

	t.Run("re-armed keeps it on with a provider enabled, until consumed", func(t *testing.T) {
		f := newSetupFixture(t, false)
		f.enableProvider()
		_, err := f.svc.ForceRearm(ctx)
		require.NoError(t, err)
		active, err := f.svc.IsActive(ctx)
		require.NoError(t, err)
		assert.True(t, active)

		require.NoError(t, f.svc.ConsumeOnRootLogin(ctx, &models.User{ID: "u-root", Email: setupRootEmail}))
		active, err = f.svc.IsActive(ctx)
		require.NoError(t, err)
		assert.False(t, active, "consumption clears the re-arm")
	})

	t.Run("errors are returned, never read as inactive", func(t *testing.T) {
		f := newSetupFixture(t, false)
		f.providers.err = errors.New("db down")
		_, err := f.svc.IsActive(ctx)
		require.Error(t, err)

		f = newSetupFixture(t, false)
		f.repo.getErr = errors.New("db down")
		_, err = f.svc.IsActive(ctx)
		require.Error(t, err)
	})
}

func TestSetupMode_EnsureTokenAtBoot(t *testing.T) {
	ctx := context.Background()

	t.Run("mints, logs exactly one SETUP URL line, stores only the hash", func(t *testing.T) {
		f := newSetupFixture(t, false)
		token := f.bootToken(t)

		raw, err := base64.RawURLEncoding.DecodeString(token)
		require.NoError(t, err, "the token is unpadded URL-safe base64")
		assert.GreaterOrEqual(t, len(raw), 32)

		want := sha256.Sum256([]byte(token))
		assert.Equal(t, want[:], f.repo.row.TokenHash)
		assert.Equal(t, f.now.Add(24*time.Hour), *f.repo.row.ExpiresAt)

		logs := f.logs.String()
		assert.Equal(t, 1, strings.Count(logs, "SETUP URL: https://vibexp.example.com/setup?token="+token))
		assert.Contains(t, logs, "level=WARN")
	})

	t.Run("not in setup mode: mints and logs nothing", func(t *testing.T) {
		f := newSetupFixture(t, false)
		f.enableProvider()
		setupURL, minted, err := f.svc.EnsureTokenAtBoot(ctx)
		require.NoError(t, err)
		assert.False(t, minted)
		assert.Empty(t, setupURL)
		assert.Nil(t, f.repo.row)
		assert.Empty(t, f.logs.String())
	})

	t.Run("a second boot within the lifetime keeps the first token valid", func(t *testing.T) {
		f := newSetupFixture(t, false)
		token := f.bootToken(t)
		f.logs.Reset()

		f.now = f.now.Add(time.Hour)
		setupURL, minted, err := f.svc.EnsureTokenAtBoot(ctx)
		require.NoError(t, err)
		assert.False(t, minted)
		assert.Empty(t, setupURL)
		assert.NotContains(t, f.logs.String(), "SETUP URL:")
		assert.Contains(t, f.logs.String(), "already issued")
		assert.Contains(t, f.logs.String(), "vibexp admin auth setup rearm")

		_, err = f.svc.ExchangeToken(ctx, token)
		require.NoError(t, err, "the first boot's token still exchanges")
	})

	t.Run("re-mints after expiry and after consumption", func(t *testing.T) {
		f := newSetupFixture(t, false)
		first := f.bootToken(t)
		f.now = f.now.Add(24*time.Hour + time.Second)
		second := f.bootToken(t)
		assert.NotEqual(t, first, second)

		require.NoError(t, f.svc.ConsumeOnRootLogin(ctx, &models.User{ID: "u-root", Email: setupRootEmail}))
		third := f.bootToken(t) // still no provider enabled
		assert.NotEqual(t, second, third)
	})

	t.Run("an unreadable state mints nothing and returns the error", func(t *testing.T) {
		f := newSetupFixture(t, false)
		f.providers.err = errors.New("db down")
		_, minted, err := f.svc.EnsureTokenAtBoot(ctx)
		require.Error(t, err)
		assert.False(t, minted)
		assert.Nil(t, f.repo.row)

		f = newSetupFixture(t, false)
		f.repo.writeErr = errors.New("db down")
		_, minted, err = f.svc.EnsureTokenAtBoot(ctx)
		require.Error(t, err)
		assert.False(t, minted)
		assert.NotContains(t, f.logs.String(), "SETUP URL:")
	})
}

// TestSetupMode_EnsureTokenAtBoot_RecoveryMode covers the recovery flag
// (#1237) against providers present and absent.
func TestSetupMode_EnsureTokenAtBoot_RecoveryMode(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name            string
		recovery        bool
		providerEnabled bool
		wantURL         bool
		wantReason      string
	}{
		{"flag on, provider enabled: a URL is logged", true, true, true, "AUTH_SETTINGS_RECOVERY_MODE"},
		{"flag on, no provider: a URL is logged", true, false, true, "AUTH_SETTINGS_RECOVERY_MODE"},
		{"flag off, provider enabled: nothing is logged", false, true, false, ""},
		{"flag off, no provider: first-run URL", false, false, true, "no identity provider is enabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSetupFixture(t, tc.recovery)
			if tc.providerEnabled {
				f.enableProvider()
			}
			setupURL, minted, err := f.svc.EnsureTokenAtBoot(ctx)
			require.NoError(t, err)
			assert.Equal(t, tc.wantURL, minted)
			if !tc.wantURL {
				assert.Empty(t, setupURL)
				assert.Empty(t, f.logs.String())
				return
			}
			logs := f.logs.String()
			assert.Equal(t, 1, strings.Count(logs, "SETUP URL: "+setupURL))
			assert.Contains(t, logs, "level=WARN")
			assert.Contains(t, logs, tc.wantReason)
		})
	}

	t.Run("every boot replaces the token, so each one logs a usable URL", func(t *testing.T) {
		f := newSetupFixture(t, true)
		f.enableProvider()
		first := f.bootToken(t)
		stale, err := f.svc.ExchangeToken(ctx, first)
		require.NoError(t, err)

		f.now = f.now.Add(time.Minute) // well inside the first token's lifetime
		second := f.bootToken(t)
		assert.NotEqual(t, first, second)

		_, err = f.svc.ExchangeToken(ctx, first)
		require.ErrorIs(t, err, ErrSetupTokenInvalid, "the previous boot's token is replaced")
		require.ErrorIs(t, f.svc.ValidateSession(ctx, stale), ErrSetupSessionInvalid)
		_, err = f.svc.ExchangeToken(ctx, second)
		require.NoError(t, err)
	})

	t.Run("removing the flag exits setup mode: the recovery mint does not re-arm", func(t *testing.T) {
		f := newSetupFixture(t, true)
		f.enableProvider()
		token := f.bootToken(t)
		assert.False(t, f.repo.row.Rearmed)

		f.svc.recoveryMode = false // the next boot, without the flag
		active, err := f.svc.IsActive(ctx)
		require.NoError(t, err)
		assert.False(t, active)
		_, err = f.svc.ExchangeToken(ctx, token)
		require.ErrorIs(t, err, ErrSetupNotActive)
	})

	t.Run("a failed mint logs no URL and returns the error", func(t *testing.T) {
		f := newSetupFixture(t, true)
		f.repo.writeErr = errors.New("db down")
		_, minted, err := f.svc.EnsureTokenAtBoot(ctx)
		require.ErrorContains(t, err, "recovery setup token")
		assert.False(t, minted)
		assert.NotContains(t, f.logs.String(), "SETUP URL:")
	})
}

// TestSetupMode_EnsureTokenAtBoot_RearmedReason pins the logged reason when a
// boot re-mints for a re-armed setup whose token expired.
func TestSetupMode_EnsureTokenAtBoot_RearmedReason(t *testing.T) {
	f := newSetupFixture(t, false)
	f.enableProvider()
	_, err := f.svc.ForceRearm(context.Background())
	require.NoError(t, err)

	f.now = f.now.Add(SetupTokenLifetime + time.Second)
	f.bootToken(t)
	assert.Contains(t, f.logs.String(), "vibexp admin auth setup rearm")
	assert.NotContains(t, f.logs.String(), "no identity provider is enabled")
}

func TestSetupURL(t *testing.T) {
	assert.Equal(t, "https://vibexp.example.com/setup?token=abc", SetupURL("https://vibexp.example.com", "abc"))
	assert.Equal(t, "https://vibexp.example.com/setup?token=abc", SetupURL("https://vibexp.example.com/", "abc"),
		"a trailing slash on frontend.base_url does not double up")
}

func TestSetupMode_ExchangeToken(t *testing.T) {
	ctx := context.Background()

	t.Run("a valid token yields a one-hour session at the stored generation", func(t *testing.T) {
		f := newSetupFixture(t, false)
		token := f.bootToken(t)
		sess, err := f.svc.ExchangeToken(ctx, token)
		require.NoError(t, err)
		assert.Equal(t, f.repo.row.Generation, sess.Generation)
		assert.Equal(t, f.now.Add(time.Hour), sess.ExpiresAt)
		require.NoError(t, f.svc.ValidateSession(ctx, sess))
	})

	t.Run("the session never outlives the token", func(t *testing.T) {
		f := newSetupFixture(t, false)
		token := f.bootToken(t)
		f.now = f.now.Add(24*time.Hour - 10*time.Minute)
		sess, err := f.svc.ExchangeToken(ctx, token)
		require.NoError(t, err)
		assert.Equal(t, *f.repo.row.ExpiresAt, sess.ExpiresAt)
	})

	t.Run("unknown, empty, oversized, expired and consumed tokens are one error", func(t *testing.T) {
		f := newSetupFixture(t, false)
		token := f.bootToken(t)

		for name, candidate := range map[string]string{
			"unknown":   strings.Repeat("A", len(token)),
			"empty":     "",
			"oversized": strings.Repeat("A", 257),
			"prefix":    token[:len(token)-1],
		} {
			_, err := f.svc.ExchangeToken(ctx, candidate)
			assert.ErrorIs(t, err, ErrSetupTokenInvalid, name)
		}

		f.now = f.now.Add(24 * time.Hour)
		_, err := f.svc.ExchangeToken(ctx, token)
		assert.ErrorIs(t, err, ErrSetupTokenInvalid, "expired at exactly 24h")

		f = newSetupFixture(t, false)
		token = f.bootToken(t)
		require.NoError(t, f.svc.ConsumeOnRootLogin(ctx, &models.User{ID: "u-root", Email: setupRootEmail}))
		_, err = f.svc.ExchangeToken(ctx, token)
		assert.ErrorIs(t, err, ErrSetupTokenInvalid, "consumed")
	})

	t.Run("no token was ever issued", func(t *testing.T) {
		f := newSetupFixture(t, false)
		_, err := f.svc.ExchangeToken(ctx, "anything")
		assert.ErrorIs(t, err, ErrSetupTokenInvalid)
	})

	t.Run("outside setup mode it is not-active, even for the right token", func(t *testing.T) {
		f := newSetupFixture(t, false)
		token := f.bootToken(t)
		f.enableProvider()
		_, err := f.svc.ExchangeToken(ctx, token)
		assert.ErrorIs(t, err, ErrSetupNotActive)
	})

	t.Run("a state read failure is neither", func(t *testing.T) {
		f := newSetupFixture(t, false)
		token := f.bootToken(t)
		f.repo.getErr = errors.New("db down")
		_, err := f.svc.ExchangeToken(ctx, token)
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrSetupTokenInvalid)
		assert.NotErrorIs(t, err, ErrSetupNotActive)
	})
}

func TestSetupMode_ValidateSession(t *testing.T) {
	ctx := context.Background()
	f := newSetupFixture(t, false)
	token := f.bootToken(t)
	sess, err := f.svc.ExchangeToken(ctx, token)
	require.NoError(t, err)

	assert.ErrorIs(t, f.svc.ValidateSession(ctx, nil), ErrSetupSessionInvalid)

	t.Run("stays valid after a provider is enabled, so a typo can be fixed", func(t *testing.T) {
		f.enableProvider()
		require.NoError(t, f.svc.ValidateSession(ctx, sess))
		f.providers.rows = nil
	})

	t.Run("a session from another generation is rejected", func(t *testing.T) {
		stale := &session.SetupSession{Generation: sess.Generation + 1, ExpiresAt: sess.ExpiresAt}
		assert.ErrorIs(t, f.svc.ValidateSession(ctx, stale), ErrSetupSessionInvalid)
	})

	t.Run("a state read failure is not an invalid session", func(t *testing.T) {
		f.repo.getErr = errors.New("db down")
		err := f.svc.ValidateSession(ctx, sess)
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrSetupSessionInvalid)
		f.repo.getErr = nil
	})

	t.Run("a re-arm invalidates it", func(t *testing.T) {
		g := newSetupFixture(t, false)
		s, err := g.svc.ExchangeToken(ctx, g.bootToken(t))
		require.NoError(t, err)
		_, err = g.svc.ForceRearm(ctx)
		require.NoError(t, err)
		assert.ErrorIs(t, g.svc.ValidateSession(ctx, s), ErrSetupSessionInvalid)
	})

	t.Run("expiry is exclusive at the instant", func(t *testing.T) {
		f.now = sess.ExpiresAt.Add(-time.Second)
		require.NoError(t, f.svc.ValidateSession(ctx, sess))
		f.now = sess.ExpiresAt
		assert.ErrorIs(t, f.svc.ValidateSession(ctx, sess), ErrSetupSessionInvalid)
	})

	t.Run("no setup row at all", func(t *testing.T) {
		g := newSetupFixture(t, false)
		orphan := &session.SetupSession{Generation: 1, ExpiresAt: g.now.Add(time.Hour)}
		assert.ErrorIs(t, g.svc.ValidateSession(ctx, orphan), ErrSetupSessionInvalid)
	})
}

func TestSetupMode_ConsumeOnRootLogin(t *testing.T) {
	ctx := context.Background()

	t.Run("a root admin consumes: token, sessions and setup mode all end", func(t *testing.T) {
		f := newSetupFixture(t, false)
		token := f.bootToken(t)
		sess, err := f.svc.ExchangeToken(ctx, token)
		require.NoError(t, err)
		f.enableProvider() // the provider the root admin signed in through

		require.NoError(t, f.svc.ConsumeOnRootLogin(ctx, &models.User{ID: "u-root", Email: "ROOT@example.com"}))

		assert.Equal(t, []string{"u-root"}, f.repo.consumes)
		assert.ErrorIs(t, f.svc.ValidateSession(ctx, sess), ErrSetupSessionInvalid)
		active, err := f.svc.IsActive(ctx)
		require.NoError(t, err)
		assert.False(t, active)
		assert.Contains(t, f.logs.String(), "Authentication setup completed")
	})

	t.Run("a non-root sign-in leaves setup open", func(t *testing.T) {
		f := newSetupFixture(t, false)
		token := f.bootToken(t)
		sess, err := f.svc.ExchangeToken(ctx, token)
		require.NoError(t, err)

		require.NoError(t, f.svc.ConsumeOnRootLogin(ctx, &models.User{ID: "u-1", Email: "user@example.com"}))
		require.NoError(t, f.svc.ConsumeOnRootLogin(ctx, nil))

		assert.Empty(t, f.repo.consumes)
		require.NoError(t, f.svc.ValidateSession(ctx, sess))
		_, err = f.svc.ExchangeToken(ctx, token)
		require.NoError(t, err)
	})

	t.Run("nothing outstanding: no write", func(t *testing.T) {
		f := newSetupFixture(t, false)
		f.repo.writeErr = errors.New("must not be called")
		root := &models.User{ID: "u-root", Email: setupRootEmail}
		require.NoError(t, f.svc.ConsumeOnRootLogin(ctx, root), "no row")

		f.repo.writeErr = nil
		f.bootToken(t)
		require.NoError(t, f.svc.ConsumeOnRootLogin(ctx, root))
		f.repo.writeErr = errors.New("must not be called")
		require.NoError(t, f.svc.ConsumeOnRootLogin(ctx, root), "already consumed")
		assert.Len(t, f.repo.consumes, 1)
	})

	t.Run("failures are returned", func(t *testing.T) {
		f := newSetupFixture(t, false)
		f.bootToken(t)
		root := &models.User{ID: "u-root", Email: setupRootEmail}
		f.repo.writeErr = errors.New("db down")
		require.Error(t, f.svc.ConsumeOnRootLogin(ctx, root))
		f.repo.writeErr = nil
		f.repo.getErr = errors.New("db down")
		require.Error(t, f.svc.ConsumeOnRootLogin(ctx, root))
	})
}

func TestSetupMode_ForceRearm(t *testing.T) {
	ctx := context.Background()
	f := newSetupFixture(t, false)
	old := f.bootToken(t)

	token, err := f.svc.ForceRearm(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, old, token)
	assert.True(t, f.repo.row.Rearmed)

	_, err = f.svc.ExchangeToken(ctx, old)
	assert.ErrorIs(t, err, ErrSetupTokenInvalid, "the replaced token no longer exchanges")
	_, err = f.svc.ExchangeToken(ctx, token)
	require.NoError(t, err)

	f.repo.writeErr = errors.New("db down")
	_, err = f.svc.ForceRearm(ctx)
	require.Error(t, err)
}

func TestNewSetupModeService_ReturnsTheInterface(t *testing.T) {
	svc := NewSetupModeService(SetupModeDeps{
		Setup: &memSetupRepo{}, Providers: &setupProviderRows{},
		IsRootAdmin: func(string) bool { return false },
		Logger:      slog.New(slog.DiscardHandler),
	})
	active, err := svc.IsActive(context.Background())
	require.NoError(t, err)
	assert.True(t, active)
}
