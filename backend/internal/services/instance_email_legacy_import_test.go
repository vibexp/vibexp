package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// legacyImportSecret is the legacy block's plaintext credential; no stored
// snapshot may contain it.
const legacyImportSecret = "legacy-import-sentinel-5c1e"

// dockerDefaultsLegacyEmail is what the baked config.docker.yaml resolves to on
// an install that never configured mail.
func dockerDefaultsLegacyEmail() config.LegacyEmailConfig {
	return config.LegacyEmailConfig{
		Provider:         EmailProviderTypeSMTP,
		PrivacyPolicyURL: config.DefaultLegacyPrivacyPolicyURL,
		SMTP:             config.SMTPConfig{Host: config.DefaultLegacySMTPHost, Port: config.DefaultLegacySMTPPort},
		Postmark:         config.PostmarkConfig{MessageStream: "outbound"},
	}
}

// populatedLegacySMTP is an operator's real SMTP block.
func populatedLegacySMTP() config.LegacyEmailConfig {
	c := dockerDefaultsLegacyEmail()
	c.FromAddress = "noreply@legacy.test"
	c.SMTP = config.SMTPConfig{Host: "mail.legacy.test", Port: "2525", Username: "relay", Password: legacyImportSecret}
	return c
}

type legacyImportFixture struct {
	repo   *repomocks.MockInstanceEmailProviderRepository
	audit  *repomocks.MockInstanceSettingsAuditRepository
	enc    EncryptionServiceInterface
	logs   *logtest.Recorder
	logger *slog.Logger
}

func newLegacyImportFixture(t *testing.T) *legacyImportFixture {
	t.Helper()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	logger, logs := logtest.New()
	return &legacyImportFixture{
		repo:   repomocks.NewMockInstanceEmailProviderRepository(t),
		audit:  repomocks.NewMockInstanceSettingsAuditRepository(t),
		enc:    enc,
		logs:   logs,
		logger: logger,
	}
}

func (f *legacyImportFixture) deps() LegacyEmailImportDeps {
	return LegacyEmailImportDeps{Repo: f.repo, Audit: f.audit, Enc: f.enc, Logger: f.logger}
}

func (f *legacyImportFixture) run(legacy config.LegacyEmailConfig, admins ...string) LegacyEmailImportResult {
	return ImportLegacyEmailConfig(context.Background(), f.deps(), legacy, admins)
}

func (f *legacyImportFixture) noRow() {
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound).Once()
}

// captureInsert records the row InsertIfAbsent receives.
func (f *legacyImportFixture) captureInsert(inserted bool, err error) *models.InstanceEmailProvider {
	captured := &models.InstanceEmailProvider{}
	f.repo.On("InsertIfAbsent", mock.Anything, mock.AnythingOfType("*models.InstanceEmailProvider")).
		Run(func(args mock.Arguments) {
			*captured = *args.Get(1).(*models.InstanceEmailProvider)
		}).Return(inserted, err).Once()
	return captured
}

func (f *legacyImportFixture) captureAppend(err error) *models.InstanceSettingsAuditEntry {
	captured := &models.InstanceSettingsAuditEntry{}
	f.audit.On("Append", mock.Anything, mock.AnythingOfType("*models.InstanceSettingsAuditEntry")).
		Run(func(args mock.Arguments) {
			*captured = *args.Get(1).(*models.InstanceSettingsAuditEntry)
		}).Return(err).Once()
	return captured
}

// entriesAt returns the messages logged at level.
func (f *legacyImportFixture) entriesAt(level slog.Level) []*logtest.Entry {
	var out []*logtest.Entry
	for _, entry := range f.logs.AllEntries() {
		if entry.Level == level {
			out = append(out, entry)
		}
	}
	return out
}

func (f *legacyImportFixture) messagesAt(level slog.Level) []string {
	var out []string
	for _, entry := range f.entriesAt(level) {
		out = append(out, entry.Message)
	}
	return out
}

func TestLegacyEmailPopulated(t *testing.T) {
	withSMTP := func(host, port, password string) config.LegacyEmailConfig {
		c := dockerDefaultsLegacyEmail()
		c.SMTP = config.SMTPConfig{Host: host, Port: port, Password: password}
		return c
	}
	withProvider := func(provider string, mutate func(*config.LegacyEmailConfig)) config.LegacyEmailConfig {
		c := dockerDefaultsLegacyEmail()
		c.Provider = provider
		if mutate != nil {
			mutate(&c)
		}
		return c
	}

	tests := []struct {
		name   string
		legacy config.LegacyEmailConfig
		want   bool
	}{
		{"docker defaults", dockerDefaultsLegacyEmail(), false},
		{"zero value", config.LegacyEmailConfig{}, false},
		{"default destination with a from address only", func() config.LegacyEmailConfig {
			c := dockerDefaultsLegacyEmail()
			c.FromAddress = "noreply@legacy.test"
			return c
		}(), false},
		{"default destination with a password", withSMTP(config.DefaultLegacySMTPHost, config.DefaultLegacySMTPPort, "app-password"), true},
		{"dev mailpit", withSMTP("localhost", "1025", "dev"), true},
		{"dev mailpit without a password", withSMTP("localhost", "1025", ""), true},
		{"relay without a password", withSMTP("relay.internal", "25", ""), true},
		{"default host on another port", withSMTP(config.DefaultLegacySMTPHost, "465", ""), true},
		{"host without port", withSMTP("relay.internal", "", "pw"), false},
		{"port without host", withSMTP("", "25", "pw"), false},
		{"empty provider is smtp", withProvider("", func(c *config.LegacyEmailConfig) {
			c.SMTP = config.SMTPConfig{Host: "relay.internal", Port: "25"}
		}), true},
		{"mailgun with domain and key", withProvider("mailgun", func(c *config.LegacyEmailConfig) {
			c.Mailgun = config.MailgunConfig{Domain: "mg.legacy.test", SendingKey: "key"}
		}), true},
		{"mailgun without key", withProvider("mailgun", func(c *config.LegacyEmailConfig) {
			c.Mailgun = config.MailgunConfig{Domain: "mg.legacy.test"}
		}), false},
		{"mailgun without domain", withProvider("mailgun", func(c *config.LegacyEmailConfig) {
			c.Mailgun = config.MailgunConfig{SendingKey: "key"}
		}), false},
		{"postmark with token", withProvider("postmark", func(c *config.LegacyEmailConfig) {
			c.Postmark.ServerToken = "token"
		}), true},
		{"postmark without token", withProvider("postmark", nil), false},
		{"sendgrid with key", withProvider("SendGrid", func(c *config.LegacyEmailConfig) {
			c.SendGrid.APIKey = "key"
		}), true},
		{"sendgrid without key", withProvider("sendgrid", nil), false},
		{"default destination with a blank password", withSMTP(config.DefaultLegacySMTPHost, config.DefaultLegacySMTPPort, "  "), false},
		{"mailgun with a blank key", withProvider("mailgun", func(c *config.LegacyEmailConfig) {
			c.Mailgun = config.MailgunConfig{Domain: "mg.legacy.test", SendingKey: " "}
		}), false},
		{"postmark with a blank token", withProvider("postmark", func(c *config.LegacyEmailConfig) {
			c.Postmark.ServerToken = "\t"
		}), false},
		{"sendgrid with a blank key", withProvider("sendgrid", func(c *config.LegacyEmailConfig) {
			c.SendGrid.APIKey = " "
		}), false},
		{"unknown provider", withProvider("carrier-pigeon", nil), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, LegacyEmailPopulated(tt.legacy))
		})
	}
}

// loadLegacyEmail loads a real config file through the real loader and returns
// its email: section, exactly as the boot path sees it.
func loadLegacyEmail(t *testing.T, path string) config.LegacyEmailConfig {
	t.Helper()
	cfg, err := config.Load(path)
	require.NoError(t, err)
	return cfg.LegacyEmail
}

// The baked image config with no mail env set, and a config that inherits the
// email: defaults entirely, are not real configurations and must not be
// imported. The dev example (Mailpit) is, per epic decision 9.
func TestLegacyEmailPopulated_ShippedConfigs(t *testing.T) {
	encryptionKey := strings.Repeat("k", 32)

	t.Run("baked config.docker.yaml with no mail env", func(t *testing.T) {
		t.Setenv("ENCRYPTION_KEY", encryptionKey)
		t.Setenv("DB_PASSWORD", "local_password")
		// Empty is unset for ${VAR:-default}, so a developer shell exporting
		// SMTP_PASSWORD (backend/.env) cannot leak into this case.
		for _, name := range []string{
			"EMAIL_PROVIDER", "EMAIL_FROM_ADDRESS", "SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD",
			"MAILGUN_BASE_URL", "MAILGUN_DOMAIN", "MAILGUN_SENDING_KEY", "POSTMARK_SERVER_TOKEN", "SENDGRID_API_KEY",
		} {
			t.Setenv(name, "")
		}
		legacy := loadLegacyEmail(t, "../../config.docker.yaml")
		assert.False(t, LegacyEmailPopulated(legacy), "docker defaults must not count as a configuration")
	})

	t.Run("defaults only", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte("security:\n  encryption_key: \""+encryptionKey+"\"\n"), 0o600))
		assert.False(t, LegacyEmailPopulated(loadLegacyEmail(t, path)))
	})

	t.Run("dev config.example.yaml", func(t *testing.T) {
		t.Setenv("ENCRYPTION_KEY", encryptionKey)
		t.Setenv("SMTP_PASSWORD", "dev")
		legacy := loadLegacyEmail(t, "../../config.example.yaml")
		assert.True(t, LegacyEmailPopulated(legacy), "the dev Mailpit block is imported (epic decision 9)")
	})
}

// A default-only block makes no repository write and no audit call; the one
// read decides the unconfigured-mail report.
func TestImportLegacyEmailConfig_DefaultsAreNotImported(t *testing.T) {
	f := newLegacyImportFixture(t)
	f.noRow()

	result := f.run(dockerDefaultsLegacyEmail(), "admin@legacy.test")

	assert.Equal(t, LegacyEmailImportResult{Unconfigured: true}, result)
	f.repo.AssertNotCalled(t, "InsertIfAbsent", mock.Anything, mock.Anything)
	f.audit.AssertNotCalled(t, "Append", mock.Anything, mock.Anything)
	assert.Empty(t, f.messagesAt(slog.LevelWarn), "no deprecation warning for a block the operator never set")
}

func TestImportLegacyEmailConfig_ImportsPopulatedSMTPBlock(t *testing.T) {
	f := newLegacyImportFixture(t)
	f.noRow()
	row := f.captureInsert(true, nil)
	entry := f.captureAppend(nil)

	legacy := populatedLegacySMTP()
	legacy.ContactRecipientAddress = "ops@legacy.test"
	result := f.run(legacy)

	assert.Equal(t, LegacyEmailImportResult{Imported: true}, result)

	assert.Equal(t, EmailProviderTypeSMTP, row.ProviderType)
	assert.JSONEq(t, `{"host":"mail.legacy.test","port":"2525","username":"relay"}`, string(row.Settings))
	assert.Equal(t, "noreply@legacy.test", row.FromAddress)
	assert.Equal(t, "ops@legacy.test", derefOrEmpty(row.ContactRecipientAddress))
	assert.Nil(t, row.PrivacyPolicyURL, "the defaults() placeholder was never chosen by the operator")
	assert.Nil(t, row.UpdatedBy)
	require.NotNil(t, row.SecretEncrypted)
	assert.NotEqual(t, legacyImportSecret, *row.SecretEncrypted)
	decrypted, err := f.enc.Decrypt(*row.SecretEncrypted)
	require.NoError(t, err)
	assert.Equal(t, legacyImportSecret, decrypted)

	assert.Equal(t, models.InstanceSettingEmailProvider, entry.Setting)
	assert.Equal(t, models.InstanceSettingsAuditActionImport, entry.Action)
	assert.Nil(t, entry.ActorUserID)
	assert.Nil(t, entry.Before)
	var after map[string]any
	require.NoError(t, json.Unmarshal(entry.After, &after))
	assert.Equal(t, models.InstanceSettingsAuditSecretChanged, after["secret"])
	assert.Equal(t, true, after["has_credential"])
	encoded, err := json.Marshal(entry)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), legacyImportSecret)

	assert.Equal(t, []string{
		"The config.yaml email: section is deprecated and will be removed in the next minor release; " +
			"configure instance mail under Admin → Settings → Email",
	}, f.messagesAt(slog.LevelWarn))
	assert.Equal(t, legacyEmailSection, f.entriesAt(slog.LevelWarn)[0].Data["section"])
}

// An unauthenticated relay has no secret: nothing is encrypted and the audit
// entry carries no secret marker.
func TestImportLegacyEmailConfig_ImportsRelayWithoutSecret(t *testing.T) {
	f := newLegacyImportFixture(t)
	f.enc = nil // never needed without a secret
	f.noRow()
	row := f.captureInsert(true, nil)
	entry := f.captureAppend(nil)

	legacy := populatedLegacySMTP()
	legacy.SMTP.Password = ""
	legacy.PrivacyPolicyURL = "https://legacy.test/privacy"

	assert.True(t, f.run(legacy).Imported)
	assert.Nil(t, row.SecretEncrypted)
	assert.Equal(t, "https://legacy.test/privacy", derefOrEmpty(row.PrivacyPolicyURL))

	var after map[string]any
	require.NoError(t, json.Unmarshal(entry.After, &after))
	assert.NotContains(t, after, "secret")
	assert.Equal(t, false, after["has_credential"])
}

func TestImportLegacyEmailConfig_LostRaceWritesNoAudit(t *testing.T) {
	f := newLegacyImportFixture(t)
	f.noRow()
	f.captureInsert(false, nil)

	result := f.run(populatedLegacySMTP())

	assert.Equal(t, LegacyEmailImportResult{}, result, "a row exists: neither imported here nor unconfigured")
	f.audit.AssertNotCalled(t, "Append", mock.Anything, mock.Anything)
	assert.Contains(t, f.messagesAt(slog.LevelInfo),
		"The instance email provider was stored concurrently; the config.yaml email: section was not imported")
}

func TestImportLegacyEmailConfig_ExistingRowWins(t *testing.T) {
	f := newLegacyImportFixture(t)
	f.repo.On("Get", mock.Anything).Return(&models.InstanceEmailProvider{ProviderType: "smtp"}, nil).Once()

	result := f.run(populatedLegacySMTP())

	assert.Equal(t, LegacyEmailImportResult{Ignored: true}, result)
	f.repo.AssertNotCalled(t, "InsertIfAbsent", mock.Anything, mock.Anything)
	f.audit.AssertNotCalled(t, "Append", mock.Anything, mock.Anything)
	warnings := f.messagesAt(slog.LevelWarn)
	require.Len(t, warnings, 2)
	assert.Equal(t, "The config.yaml email: section is ignored: the instance email provider stored in the "+
		"database is authoritative", warnings[1])
}

func TestImportLegacyEmailConfig_ExistingRowAndDefaultsIsSilent(t *testing.T) {
	f := newLegacyImportFixture(t)
	f.repo.On("Get", mock.Anything).Return(&models.InstanceEmailProvider{ProviderType: "smtp"}, nil).Once()

	assert.Equal(t, LegacyEmailImportResult{}, f.run(dockerDefaultsLegacyEmail()))
	assert.Empty(t, f.logs.AllEntries())
}

func TestImportLegacyEmailConfig_ReadErrorSkipsEverything(t *testing.T) {
	f := newLegacyImportFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, errors.New("connection refused")).Once()

	assert.Equal(t, LegacyEmailImportResult{}, f.run(populatedLegacySMTP()))
	f.repo.AssertNotCalled(t, "InsertIfAbsent", mock.Anything, mock.Anything)
	assert.Equal(t, []string{
		"Failed to read the instance email provider; skipping the config.yaml email: import",
	}, f.messagesAt(slog.LevelError))
	assert.Len(t, f.messagesAt(slog.LevelWarn), 1, "only the deprecation warning: configured state is unknown")
}

// Every import failure logs an ERROR, imports nothing and never panics; the
// instance is then reported as unconfigured.
func TestImportLegacyEmailConfig_FailuresNeverBreakBoot(t *testing.T) {
	encryptErr := errors.New("bad key")
	tests := []struct {
		name      string
		legacy    func() config.LegacyEmailConfig
		setup     func(f *legacyImportFixture)
		wantError string
		wantAttr  string
		wantField string
	}{
		{
			name: "no usable from address",
			legacy: func() config.LegacyEmailConfig {
				c := populatedLegacySMTP()
				c.FromAddress = ""
				return c
			},
			wantError: "The config.yaml email: section is invalid; nothing was imported",
			wantAttr:  "fields",
			wantField: "from_address: is required",
		},
		{
			name: "unknown provider",
			legacy: func() config.LegacyEmailConfig {
				c := populatedLegacySMTP()
				c.Provider = "carrier-pigeon"
				return c
			},
			wantError: "The config.yaml email: section is invalid; nothing was imported",
			wantAttr:  "fields",
			wantField: "provider_type: must be one of smtp, mailgun, postmark, sendgrid",
		},
		{
			name:      "encryption fails",
			legacy:    populatedLegacySMTP,
			setup:     func(f *legacyImportFixture) { f.enc = failingEncryption{err: encryptErr} },
			wantError: "Failed to import the config.yaml email: section",
			wantAttr:  "error",
		},
		{
			name:      "encryption unavailable",
			legacy:    populatedLegacySMTP,
			setup:     func(f *legacyImportFixture) { f.enc = nil },
			wantError: "Failed to import the config.yaml email: section",
			wantAttr:  "error",
		},
		{
			name:   "insert fails",
			legacy: populatedLegacySMTP,
			setup: func(f *legacyImportFixture) {
				f.captureInsert(false, errors.New("insert failed"))
			},
			wantError: "Failed to import the config.yaml email: section",
			wantAttr:  "error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newLegacyImportFixture(t)
			f.noRow()
			if tt.setup != nil {
				tt.setup(f)
			}

			var result LegacyEmailImportResult
			require.NotPanics(t, func() { result = f.run(tt.legacy()) })

			assert.Equal(t, LegacyEmailImportResult{Unconfigured: true}, result)
			f.audit.AssertNotCalled(t, "Append", mock.Anything, mock.Anything)
			errorsLogged := f.entriesAt(slog.LevelError)
			require.Len(t, errorsLogged, 1)
			assert.Equal(t, tt.wantError, errorsLogged[0].Message)
			require.Contains(t, errorsLogged[0].Data, tt.wantAttr)
			if tt.wantField != "" {
				assert.Contains(t, errorsLogged[0].Data[tt.wantAttr], tt.wantField)
			}
			assert.NotContains(t, fmt.Sprint(errorsLogged[0].Data), legacyImportSecret)
		})
	}
}

// The row is stored and serving mail; an audit failure is logged, not fatal.
func TestImportLegacyEmailConfig_AuditFailureIsLogged(t *testing.T) {
	f := newLegacyImportFixture(t)
	f.noRow()
	f.captureInsert(true, nil)
	f.captureAppend(errors.New("audit down"))

	result := f.run(populatedLegacySMTP())

	assert.Equal(t, LegacyEmailImportResult{Imported: true}, result)
	assert.Equal(t, []string{"Imported the config.yaml email: section but failed to audit it"},
		f.messagesAt(slog.LevelError))
}

func TestImportLegacyEmailConfig_UnconfiguredWarnings(t *testing.T) {
	t.Run("no instance admins", func(t *testing.T) {
		f := newLegacyImportFixture(t)
		f.noRow()

		f.run(dockerDefaultsLegacyEmail())

		warnings := f.messagesAt(slog.LevelWarn)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "Instance email is not configured and auth.instance_admins is empty, "+
			"so nobody can configure it")
		assert.Empty(t, f.messagesAt(slog.LevelInfo))
	})

	t.Run("only blank instance admins", func(t *testing.T) {
		f := newLegacyImportFixture(t)
		f.noRow()

		f.run(dockerDefaultsLegacyEmail(), "", " ")

		warnings := f.messagesAt(slog.LevelWarn)
		require.Len(t, warnings, 1, "blank entries are not admins (IsRootAdmin ignores them)")
		assert.Contains(t, warnings[0], "nobody can configure it")
	})

	t.Run("instance admins present", func(t *testing.T) {
		f := newLegacyImportFixture(t)
		f.noRow()

		f.run(dockerDefaultsLegacyEmail(), "admin@legacy.test")

		assert.Empty(t, f.messagesAt(slog.LevelWarn))
		assert.Equal(t, []string{
			"Instance email is not configured; an instance admin can configure it under Admin → Settings → Email",
		}, f.messagesAt(slog.LevelInfo))
	})

	t.Run("failed import is reported as unconfigured", func(t *testing.T) {
		f := newLegacyImportFixture(t)
		f.noRow()
		legacy := populatedLegacySMTP()
		legacy.FromAddress = ""

		f.run(legacy)

		warnings := f.messagesAt(slog.LevelWarn)
		require.Len(t, warnings, 2, "deprecation + unconfigured")
		assert.Contains(t, warnings[1], "Instance email is not configured")
	})
}

func TestLegacyEmailRequest_Mapping(t *testing.T) {
	discard := slog.New(slog.DiscardHandler)

	t.Run("from address falls back to an email-shaped smtp username", func(t *testing.T) {
		c := populatedLegacySMTP()
		c.FromAddress = ""
		c.SMTP.Username = "sender@legacy.test"
		assert.Equal(t, "sender@legacy.test", legacyEmailRequest(c, discard).FromAddress)
	})

	t.Run("a non-email smtp username is not a from address", func(t *testing.T) {
		c := populatedLegacySMTP()
		c.FromAddress = ""
		assert.Empty(t, legacyEmailRequest(c, discard).FromAddress)
	})

	t.Run("the secret is taken verbatim", func(t *testing.T) {
		c := populatedLegacySMTP()
		c.SMTP.Password = " spaced secret "
		req := legacyEmailRequest(c, discard)
		require.NotNil(t, req.Secret)
		assert.Equal(t, " spaced secret ", *req.Secret)
	})

	t.Run("mailgun", func(t *testing.T) {
		c := dockerDefaultsLegacyEmail()
		c.Provider, c.FromAddress = "mailgun", "noreply@legacy.test"
		c.Mailgun = config.MailgunConfig{Domain: "mg.legacy.test", BaseURL: "https://api.eu.mailgun.net", SendingKey: "mg-key"}
		req := legacyEmailRequest(c, discard)
		assert.Equal(t, EmailProviderTypeMailgun, req.ProviderType)
		assert.Equal(t, &models.MailgunProviderSettings{Domain: "mg.legacy.test", BaseURL: "https://api.eu.mailgun.net"},
			req.Settings.Mailgun)
		assert.Nil(t, req.Settings.SMTP)
		assert.Equal(t, "mg-key", *req.Secret)
		require.NoError(t, validateInstanceUpsertRequest(req, false))
	})

	t.Run("postmark", func(t *testing.T) {
		c := dockerDefaultsLegacyEmail()
		c.Provider, c.FromAddress = "postmark", "noreply@legacy.test"
		c.Postmark.ServerToken = "pm-token"
		req := legacyEmailRequest(c, discard)
		assert.Equal(t, &models.PostmarkProviderSettings{MessageStream: "outbound"}, req.Settings.Postmark)
		assert.Equal(t, "pm-token", *req.Secret)
		require.NoError(t, validateInstanceUpsertRequest(req, false))
	})

	t.Run("sendgrid", func(t *testing.T) {
		c := dockerDefaultsLegacyEmail()
		c.Provider, c.FromAddress = "sendgrid", "noreply@legacy.test"
		c.SendGrid.APIKey = "sg-key"
		req := legacyEmailRequest(c, discard)
		assert.Equal(t, models.TeamEmailProviderSettings{}, req.Settings)
		assert.Equal(t, "sg-key", *req.Secret)
		require.NoError(t, validateInstanceUpsertRequest(req, false))
	})

	t.Run("credentials of unselected providers are dropped and logged", func(t *testing.T) {
		logger, logs := logtest.New()
		c := populatedLegacySMTP()
		c.Mailgun.SendingKey = "mg-key"
		c.SendGrid.APIKey = "sg-key"

		req := legacyEmailRequest(c, logger)

		assert.Equal(t, legacyImportSecret, *req.Secret)
		assert.Nil(t, req.Settings.Mailgun)
		entry := logs.LastEntry()
		require.NotNil(t, entry)
		assert.Equal(t, slog.LevelInfo, entry.Level)
		assert.Equal(t, []string{EmailProviderTypeMailgun, EmailProviderTypeSendGrid}, entry.Data["dropped"])
		assert.NotContains(t, fmt.Sprint(entry.Data), "mg-key")
	})
}
