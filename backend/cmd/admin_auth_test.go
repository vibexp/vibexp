package cmd

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// fakeAuthRecovery records what the commands ask for and replays set results.
type fakeAuthRecovery struct {
	providers []*models.InstanceAuthProvider
	toggle    services.ProviderToggle
	cleared   bool
	setupURL  string
	err       error

	toggles  []string
	clears   int
	rearms   int
	released int
	deadline bool
}

func (f *fakeAuthRecovery) ListProviders(ctx context.Context) ([]*models.InstanceAuthProvider, error) {
	_, f.deadline = ctx.Deadline()
	return f.providers, f.err
}

func (f *fakeAuthRecovery) SetProviderEnabled(
	_ context.Context, slug string, enabled bool,
) (services.ProviderToggle, error) {
	f.toggles = append(f.toggles, slug+"="+yesNo(enabled))
	return f.toggle, f.err
}

func (f *fakeAuthRecovery) ClearAllowlist(context.Context) (bool, error) {
	f.clears++
	return f.cleared, f.err
}

func (f *fakeAuthRecovery) RearmSetup(context.Context) (string, error) {
	f.rearms++
	return f.setupURL, f.err
}

// runAdmin executes `vibexp <args>` against fake and returns stdout, stderr and
// the command error.
func runAdmin(t *testing.T, fake *fakeAuthRecovery, openErr error, args ...string) (string, string, error) {
	t.Helper()
	root := &cobra.Command{Use: "vibexp"}
	root.AddCommand(newAdminCmd(func(context.Context) (authRecovery, func(), error) {
		if openErr != nil {
			return nil, nil, openErr
		}
		return fake, func() { fake.released++ }, nil
	}))
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func TestAdminAuth_RegisteredOnRoot(t *testing.T) {
	for _, path := range [][]string{
		{"admin", "auth", "providers", "list"},
		{"admin", "auth", "providers", "disable"},
		{"admin", "auth", "providers", "enable"},
		{"admin", "auth", "allowlist", "clear"},
		{"admin", "auth", "setup", "rearm"},
	} {
		found, _, err := rootCmd.Find(path)
		require.NoError(t, err, path)
		assert.Equal(t, path[len(path)-1], found.Name(), path)
		assert.NotNil(t, found.Flag("config"), "subcommands inherit --config")
	}
}

func TestAdminAuth_ProvidersList(t *testing.T) {
	ciphertext := "enc:v1:do-not-print"
	fake := &fakeAuthRecovery{providers: []*models.InstanceAuthProvider{
		{Slug: "google", Type: models.InstanceAuthProviderGoogle, DisplayName: "Google", Enabled: true,
			ClientID: "client-id", ClientSecretEncrypted: &ciphertext},
		{Slug: "corp-sso", Type: models.InstanceAuthProviderOIDC, DisplayName: "Corp SSO"},
	}}

	out, _, err := runAdmin(t, fake, nil, "admin", "auth", "providers", "list")
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, []string{"SLUG", "TYPE", "ENABLED", "DISPLAY", "NAME", "CLIENT", "SECRET"}, strings.Fields(lines[0]))
	assert.Equal(t, []string{"google", "google", "yes", "Google", "stored"}, strings.Fields(lines[1]))
	assert.Equal(t, []string{"corp-sso", "oidc", "no", "Corp", "SSO", "none"}, strings.Fields(lines[2]))
	assert.NotContains(t, out, ciphertext, "the secret's ciphertext is never printed")
	assert.True(t, fake.deadline, "the command runs under a bounded context")
	assert.Equal(t, 1, fake.released, "what the opener opened is released")

	out, _, err = runAdmin(t, &fakeAuthRecovery{}, nil, "admin", "auth", "providers", "list")
	require.NoError(t, err)
	assert.Equal(t, "No identity providers are stored.\n", out)
}

func TestAdminAuth_ProviderToggle(t *testing.T) {
	t.Run("disable", func(t *testing.T) {
		fake := &fakeAuthRecovery{toggle: services.ProviderToggle{Changed: true}}
		out, _, err := runAdmin(t, fake, nil, "admin", "auth", "providers", "disable", "corp-sso")
		require.NoError(t, err)
		assert.Equal(t, "Identity provider \"corp-sso\" disabled.\n", out)
		assert.Equal(t, []string{"corp-sso=no"}, fake.toggles)
	})

	t.Run("disabling the last provider hints at setup mode", func(t *testing.T) {
		fake := &fakeAuthRecovery{toggle: services.ProviderToggle{Changed: true, NoneEnabled: true}}
		out, _, err := runAdmin(t, fake, nil, "admin", "auth", "providers", "disable", "corp-sso")
		require.NoError(t, err)
		assert.Contains(t, out, "Identity provider \"corp-sso\" disabled.\n")
		assert.Contains(t, out, "the instance is in setup mode")
		assert.Contains(t, out, "vibexp admin auth setup rearm")
	})

	t.Run("enable", func(t *testing.T) {
		fake := &fakeAuthRecovery{toggle: services.ProviderToggle{Changed: true}}
		out, _, err := runAdmin(t, fake, nil, "admin", "auth", "providers", "enable", "corp-sso")
		require.NoError(t, err)
		assert.Equal(t, "Identity provider \"corp-sso\" enabled.\n", out)
		assert.Equal(t, []string{"corp-sso=yes"}, fake.toggles)
	})

	t.Run("already in the requested state exits 0 with no change", func(t *testing.T) {
		out, _, err := runAdmin(t, &fakeAuthRecovery{}, nil, "admin", "auth", "providers", "disable", "corp-sso")
		require.NoError(t, err)
		assert.Equal(t, "Identity provider \"corp-sso\" is already disabled; no change.\n", out)

		out, _, err = runAdmin(t, &fakeAuthRecovery{}, nil, "admin", "auth", "providers", "enable", "corp-sso")
		require.NoError(t, err)
		assert.Equal(t, "Identity provider \"corp-sso\" is already enabled; no change.\n", out)
	})

	t.Run("an unknown slug fails with a clear message", func(t *testing.T) {
		fake := &fakeAuthRecovery{err: repositories.ErrInstanceAuthProviderNotFound}
		out, stderr, err := runAdmin(t, fake, nil, "admin", "auth", "providers", "disable", "nope")
		require.ErrorContains(t, err, `no identity provider has the slug "nope"`)
		require.ErrorContains(t, err, "vibexp admin auth providers list")
		assert.Empty(t, out)
		assert.Empty(t, stderr, "the failure is reported once, by Execute, without the usage text")
	})

	t.Run("the slug is required", func(t *testing.T) {
		fake := &fakeAuthRecovery{}
		_, _, err := runAdmin(t, fake, nil, "admin", "auth", "providers", "disable")
		require.Error(t, err)
		assert.Empty(t, fake.toggles)
	})
}

func TestAdminAuth_AllowlistClear(t *testing.T) {
	fake := &fakeAuthRecovery{cleared: true}
	out, _, err := runAdmin(t, fake, nil, "admin", "auth", "allowlist", "clear")
	require.NoError(t, err)
	assert.Contains(t, out, "Access allowlist cleared")
	assert.Equal(t, 1, fake.clears)

	out, _, err = runAdmin(t, &fakeAuthRecovery{}, nil, "admin", "auth", "allowlist", "clear")
	require.NoError(t, err)
	assert.Equal(t, "No access allowlist is stored; no change.\n", out)
}

func TestAdminAuth_SetupRearm(t *testing.T) {
	const setupURL = "https://vibexp.example.com/setup?token=abc"
	fake := &fakeAuthRecovery{setupURL: setupURL}
	out, _, err := runAdmin(t, fake, nil, "admin", "auth", "setup", "rearm")
	require.NoError(t, err)
	assert.Equal(t, 1, fake.rearms)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	assert.Equal(t, setupURL, lines[len(lines)-1], "the URL is the last line, alone, so it is easy to copy")
	assert.Contains(t, out, "within 24 hours")
}

func TestAdminAuth_Failures(t *testing.T) {
	boom := errors.New("db down")

	for _, args := range [][]string{
		{"admin", "auth", "providers", "list"},
		{"admin", "auth", "providers", "enable", "x"},
		{"admin", "auth", "allowlist", "clear"},
		{"admin", "auth", "setup", "rearm"},
	} {
		fake := &fakeAuthRecovery{err: boom}
		out, _, err := runAdmin(t, fake, nil, args...)
		require.ErrorIs(t, err, boom, args)
		assert.Empty(t, out, args)
		assert.Equal(t, 1, fake.released, args)

		fake = &fakeAuthRecovery{}
		_, _, err = runAdmin(t, fake, boom, args...)
		require.ErrorIs(t, err, boom, "an unreachable database fails the command: %v", args)
		assert.Zero(t, fake.released+fake.clears+fake.rearms+len(fake.toggles), args)
	}
}

func TestAdminAuth_GroupCommands(t *testing.T) {
	out, _, err := runAdmin(t, &fakeAuthRecovery{}, nil, "admin", "auth")
	require.NoError(t, err)
	assert.Contains(t, out, "never run database migrations")
	assert.Contains(t, out, "providers")

	_, _, err = runAdmin(t, &fakeAuthRecovery{}, nil, "admin", "auth", "providers", "delete")
	require.Error(t, err, "an unknown subcommand is an error, not a help page")
}

func TestRequireAuthSettingsTables(t *testing.T) {
	const query = `SELECT to_regclass($1) IS NOT NULL`
	newDB := func(t *testing.T) (*database.DB, sqlmock.Sqlmock) {
		t.Helper()
		sqlDB, mock, err := sqlmock.NewWithDSN("admin-auth?case=" + t.Name())
		require.NoError(t, err)
		t.Cleanup(func() {
			mock.ExpectClose()
			require.NoError(t, sqlDB.Close())
			require.NoError(t, mock.ExpectationsWereMet())
		})
		return &database.DB{DB: sqlDB}, mock
	}
	exists := func(v bool) *sqlmock.Rows { return sqlmock.NewRows([]string{"exists"}).AddRow(v) }

	t.Run("every table present", func(t *testing.T) {
		db, mock := newDB(t)
		for _, table := range authSettingsTables {
			mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(table).WillReturnRows(exists(true))
		}
		require.NoError(t, requireAuthSettingsTables(context.Background(), db))
	})

	t.Run("a missing table refuses to act and points at a server boot", func(t *testing.T) {
		db, mock := newDB(t)
		mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(authSettingsTables[0]).WillReturnRows(exists(true))
		mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(authSettingsTables[1]).WillReturnRows(exists(false))
		err := requireAuthSettingsTables(context.Background(), db)
		require.ErrorContains(t, err, authSettingsTables[1])
		require.ErrorContains(t, err, "start the server once to run the migrations")
	})

	t.Run("a failed check is an error", func(t *testing.T) {
		db, mock := newDB(t)
		mock.ExpectQuery(regexp.QuoteMeta(query)).WillReturnError(errors.New("db down"))
		require.ErrorContains(t, requireAuthSettingsTables(context.Background(), db), "failed to check the database schema")
	})
}

func TestOpenAuthRecovery_ConfigAndConnectionFailures(t *testing.T) {
	previous := configPath
	t.Cleanup(func() { configPath = previous })

	configPath = filepath.Join(t.TempDir(), "missing.yaml")
	_, _, err := openAuthRecovery(context.Background())
	require.ErrorContains(t, err, "failed to load configuration")

	// A config that loads but names a database nothing listens on: the command
	// must fail rather than hang or act.
	t.Setenv("ENCRYPTION_KEY", "change_me_to_a_32_byte_secret_ok")
	t.Setenv("DB_PASSWORD", "local_password")
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("DB_PORT", "1")
	raw, err := os.ReadFile(filepath.Join("..", "config.docker.yaml"))
	require.NoError(t, err)
	configPath = filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, raw, 0o600))
	_, _, err = openAuthRecovery(context.Background())
	require.ErrorContains(t, err, "failed to connect to the database")
}

func TestNewAuthRecoveryService_BuildsOverTheDatabase(t *testing.T) {
	sqlDB, mock, err := sqlmock.NewWithDSN("admin-auth?case=" + t.Name())
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, sqlDB.Close())
	})
	db := &database.DB{DB: sqlDB}

	cfg := &config.Config{}
	cfg.Frontend.BaseURL = "https://vibexp.example.com"
	svc := newAuthRecoveryService(db, cfg)

	mock.ExpectQuery("SELECT .* FROM instance_auth_providers").WillReturnRows(sqlmock.NewRows(nil))
	providers, err := svc.ListProviders(context.Background())
	require.NoError(t, err)
	assert.Empty(t, providers)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWarnRecoveryMode(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.Config{}

	warnRecoveryMode(cfg, slog.New(slog.NewTextHandler(&buf, nil)))
	assert.Empty(t, buf.String(), "nothing is logged without the flag")

	cfg.Auth.RecoveryMode = true
	warnRecoveryMode(cfg, slog.New(slog.NewTextHandler(&buf, nil)))
	assert.Contains(t, buf.String(), "level=WARN")
	assert.Contains(t, buf.String(), "AUTH_SETTINGS_RECOVERY_MODE")
	assert.Contains(t, buf.String(), "auth.recovery_mode")
}
