//go:build integration

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

// The recovery CLI's changes in the admin audit log (#1535), against real
// Postgres: each command, run through cobra with the production wiring, leaves
// exactly one audit_events row by the time it returns, attributed to the CLI
// and the OS user, with no secret in it. No test runs in parallel: the tables
// are global to this package's database.

const (
	itRootEmail  = "root@example.com"
	itPassphrase = "an integration passphrase that is long"
	itCiphertext = "enc:v1:integration-ciphertext-sentinel"
	itOSUser     = "operator"
	itHost       = "db-box"
)

// cliAuditRow is the part of an audit_events row these tests read.
type cliAuditRow struct {
	Operation, Source, ActorType, ActorName string
	ActorUserID, ResourceType, ResourceID   *string
	Metadata                                map[string]any
	// Raw is the whole row as JSON, searched for secrets.
	Raw string
}

func resetCLIAuditTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), `TRUNCATE TABLE users, audit_events,
		instance_settings_audit, instance_auth_providers, instance_auth_allowlist, instance_auth_setup,
		instance_auth_policy CASCADE`)
	require.NoError(t, err)
}

func cliAuditRows(t *testing.T) []cliAuditRow {
	t.Helper()
	rows, err := integrationDB.QueryContext(context.Background(), `SELECT operation, source, actor_type,
		actor_name, actor_user_id, resource_type, resource_id, metadata, row_to_json(a)::text
		FROM audit_events a ORDER BY created_at`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	var got []cliAuditRow
	for rows.Next() {
		var row cliAuditRow
		var metadata []byte
		require.NoError(t, rows.Scan(&row.Operation, &row.Source, &row.ActorType, &row.ActorName,
			&row.ActorUserID, &row.ResourceType, &row.ResourceID, &metadata, &row.Raw))
		require.NoError(t, json.Unmarshal(metadata, &row.Metadata))
		got = append(got, row)
	}
	require.NoError(t, rows.Err())
	return got
}

// assertCLIAttribution checks the attribution every CLI row carries.
func assertCLIAttribution(t *testing.T, row cliAuditRow) {
	t.Helper()
	assert.Equal(t, string(models.AuditSourceCLI), row.Source)
	assert.Equal(t, string(models.AuditActorSystem), row.ActorType)
	assert.Equal(t, cliAuditActorName, row.ActorName)
	assert.Nil(t, row.ActorUserID)
	assert.Equal(t, itOSUser, row.Metadata["os_user"])
	assert.Equal(t, itHost, row.Metadata["hostname"])
}

// onlyCLIAuditRow returns the one row, attribution checked.
func onlyCLIAuditRow(t *testing.T) cliAuditRow {
	t.Helper()
	rows := cliAuditRows(t)
	require.Len(t, rows, 1, "exactly one audit_events row")
	assertCLIAttribution(t, rows[0])
	return rows[0]
}

// runCLI runs `vibexp args...` with the production wiring over the test
// database, returning stdout, what the audit logger wrote, and the error.
func runCLI(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	pinOSIdentity(t, itOSUser, nil, itHost, nil)
	cfg := &config.Config{}
	cfg.Frontend.BaseURL = "https://vibexp.example.com"
	cfg.Auth.InstanceAdmins = config.EnvStringSlice{itRootEmail}
	var auditLog bytes.Buffer

	root := &cobra.Command{Use: "vibexp"}
	root.AddCommand(newAdminCmd(func(context.Context) (authRecovery, func(), error) {
		recovery, unregister := newAuditedAuthRecovery(integrationDB, cfg, newCLIAuditLogger(&auditLog))
		return recovery, unregister, nil
	}))
	var stdout bytes.Buffer
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), auditLog.String(), err
}

func seedProvider(t *testing.T, enabled bool) {
	t.Helper()
	ciphertext := itCiphertext
	require.NoError(t, postgres.NewInstanceAuthProviderRepository(integrationDB).Create(context.Background(),
		&models.InstanceAuthProvider{
			Type: models.InstanceAuthProviderGoogle, Slug: "google", DisplayName: "Google",
			Enabled: enabled, ClientID: "google-client", ClientSecretEncrypted: &ciphertext,
		}, nil, nil))
}

func seedRootAdmin(t *testing.T) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		`INSERT INTO users (id, email, name, email_verified_at) VALUES ($1, $2, 'Root', now())`, id, itRootEmail)
	require.NoError(t, err)
	return id
}

func TestIntegrationCLIAudit_ProviderToggle(t *testing.T) {
	resetCLIAuditTables(t)
	seedProvider(t, true)

	_, auditLog, err := runCLI(t, "", "admin", "auth", "providers", "disable", "google")

	require.NoError(t, err)
	assert.Empty(t, auditLog)
	row := onlyCLIAuditRow(t)
	assert.Equal(t, string(models.AuditOpInstanceSettingsUpdated), row.Operation)
	assert.Equal(t, models.InstanceSettingAuthProviders, row.Metadata["setting"])
	assert.NotContains(t, row.Raw, itCiphertext, "the client secret never reaches the log")

	t.Run("a second disable changes nothing and records nothing", func(t *testing.T) {
		_, _, err := runCLI(t, "", "admin", "auth", "providers", "disable", "google")
		require.NoError(t, err)
		assert.Len(t, cliAuditRows(t), 1)
	})

	t.Run("an unknown slug fails and records nothing", func(t *testing.T) {
		_, _, err := runCLI(t, "", "admin", "auth", "providers", "enable", "nope")
		require.Error(t, err)
		assert.Len(t, cliAuditRows(t), 1)
	})
}

func TestIntegrationCLIAudit_AllowlistClear(t *testing.T) {
	resetCLIAuditTables(t)
	require.NoError(t, postgres.NewInstanceAuthAllowlistRepository(integrationDB).UpsertAudited(context.Background(),
		&models.InstanceAuthAllowlist{Domains: []string{"example.com"}}, nil, nil))

	_, _, err := runCLI(t, "", "admin", "auth", "allowlist", "clear")

	require.NoError(t, err)
	row := onlyCLIAuditRow(t)
	assert.Equal(t, string(models.AuditOpInstanceSettingsUpdated), row.Operation)
	assert.Equal(t, models.InstanceSettingAuthAllowlist, row.Metadata["setting"])
}

func TestIntegrationCLIAudit_SetupRearm(t *testing.T) {
	resetCLIAuditTables(t)

	stdout, _, err := runCLI(t, "", "admin", "auth", "setup", "rearm")

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	setupURL := lines[len(lines)-1]
	require.Contains(t, setupURL, "https://vibexp.example.com")
	token := setupURL[strings.LastIndexAny(setupURL, "/=")+1:]
	require.NotEmpty(t, token)

	row := onlyCLIAuditRow(t)
	assert.Equal(t, string(models.AuditOpInstanceSettingsUpdated), row.Operation)
	assert.Equal(t, models.InstanceSettingAuthSetup, row.Metadata["setting"])
	assert.NotContains(t, row.Raw, token, "the setup token never reaches the log")
}

func TestIntegrationCLIAudit_PasswordSet(t *testing.T) {
	resetCLIAuditTables(t)
	userID := seedRootAdmin(t)

	_, _, err := runCLI(t, itPassphrase, "admin", "auth", "password", "set", "--email", itRootEmail)

	require.NoError(t, err)
	row := onlyCLIAuditRow(t)
	assert.Equal(t, string(models.AuditOpAuthLocalCredentialSet), row.Operation)
	require.NotNil(t, row.ResourceType)
	assert.Equal(t, models.AuditResourceTypeUser, *row.ResourceType)
	require.NotNil(t, row.ResourceID)
	assert.Equal(t, userID, *row.ResourceID)
	assert.NotContains(t, row.Raw, itPassphrase, "the password never reaches the log")

	t.Run("a second set is auth.password.changed", func(t *testing.T) {
		_, _, err := runCLI(t, itPassphrase+"!", "admin", "auth", "password", "set", "--email", itRootEmail)
		require.NoError(t, err)
		rows := cliAuditRows(t)
		require.Len(t, rows, 2)
		assert.Equal(t, string(models.AuditOpAuthLocalCredentialChanged), rows[1].Operation)
	})

	t.Run("a refused password fails and records nothing", func(t *testing.T) {
		_, _, err := runCLI(t, "short", "admin", "auth", "password", "set", "--email", itRootEmail)
		require.Error(t, err)
		assert.Len(t, cliAuditRows(t), 2)
	})
}

// --enable-login is two changes, so two rows: the credential, and the
// password-login policy through the settings observer.
func TestIntegrationCLIAudit_PasswordSetWithEnableLogin(t *testing.T) {
	resetCLIAuditTables(t)
	seedRootAdmin(t)

	_, _, err := runCLI(t, itPassphrase, "admin", "auth", "password", "set", "--email", itRootEmail, "--enable-login")

	require.NoError(t, err)
	rows := cliAuditRows(t)
	require.Len(t, rows, 2, "one row per change")
	ops := map[string]any{}
	for _, row := range rows {
		assertCLIAttribution(t, row)
		assert.NotContains(t, row.Raw, itPassphrase)
		ops[row.Operation] = row.Metadata["setting"]
	}
	assert.Equal(t, map[string]any{
		string(models.AuditOpAuthLocalCredentialSet):  nil,
		string(models.AuditOpInstanceSettingsUpdated): models.InstanceSettingAuthPolicy,
	}, ops)
}

// A failed audit write is reported on stderr and never fails the command.
func TestIntegrationCLIAudit_RecorderFailureKeepsTheExitCode(t *testing.T) {
	resetCLIAuditTables(t)
	seedProvider(t, true)
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, `ALTER TABLE audit_events RENAME TO audit_events_away`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, restoreErr := integrationDB.ExecContext(ctx, `ALTER TABLE audit_events_away RENAME TO audit_events`)
		require.NoError(t, restoreErr)
	})

	stdout, auditLog, err := runCLI(t, "", "admin", "auth", "providers", "disable", "google")

	require.NoError(t, err, "the change succeeded, so the command does")
	assert.Contains(t, stdout, `Identity provider "google" disabled.`)
	assert.Contains(t, auditLog, "failed to record audit event")
}
