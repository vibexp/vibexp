//go:build integration

package postgres

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/darkrockmountain/gomail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/external/implementations"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// The #1190 upgrade bridge against real Postgres: the config.yaml email:
// section is imported into instance_email_provider exactly once, and mail then
// goes out through the imported row.

const legacyIntegrationSentinel = "legacy-integration-sentinel"

func resetLegacyEmailImportTables(t *testing.T) {
	t.Helper()
	// Truncating users cascades to instance_email_provider (updated_by FK).
	resetInstanceSettingsAuditTables(t)
	resetInstanceEmailProvider(t)
	t.Cleanup(func() { resetInstanceSettingsAuditTables(t) })
}

func legacyImportEncryption(t *testing.T) services.EncryptionServiceInterface {
	t.Helper()
	enc, err := services.NewEncryptionService(strings.Repeat("k", 32))
	require.NoError(t, err)
	return enc
}

func legacyImportDeps(enc services.EncryptionServiceInterface) services.LegacyEmailImportDeps {
	return services.LegacyEmailImportDeps{
		Repo:   NewInstanceEmailProviderRepository(integrationDB),
		Audit:  NewInstanceSettingsAuditRepository(integrationDB),
		Enc:    enc,
		Logger: slog.New(slog.DiscardHandler),
	}
}

func legacySMTPBlock(host, port string) config.LegacyEmailConfig {
	return config.LegacyEmailConfig{
		Provider:    "smtp",
		FromAddress: "noreply@legacy.test",
		SMTP: config.SMTPConfig{
			Host: host, Port: port, Username: "relay", Password: legacyIntegrationSentinel,
		},
	}
}

func countEmailProviderImportAudits(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT count(*) FROM instance_settings_audit WHERE setting = $1 AND action = $2",
		models.InstanceSettingEmailProvider, models.InstanceSettingsAuditActionImport).Scan(&n))
	return n
}

// fakeSMTPServer is a minimal SMTP sink on loopback: it accepts AUTH PLAIN and
// one message, and records the credential and envelope it was given.
type fakeSMTPServer struct {
	addr string

	mu   sync.Mutex
	auth string
	rcpt []string
	data string
}

func startFakeSMTPServer(t *testing.T) *fakeSMTPServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, listener.Close()) })

	server := &fakeSMTPServer{addr: listener.Addr().String()}
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go server.serve(conn)
		}
	}()
	return server
}

func (s *fakeSMTPServer) serve(conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil {
			return
		}
	}()
	reader := bufio.NewReader(conn)
	reply := func(line string) bool {
		_, err := conn.Write([]byte(line + "\r\n"))
		return err == nil
	}
	if !reply("220 fake ESMTP") {
		return
	}
	inData := false
	var data strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if inData {
			if line == "." {
				inData = false
				s.record(func() { s.data = data.String() })
				reply("250 OK")
				continue
			}
			data.WriteString(line + "\n")
			continue
		}
		command := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(command, "EHLO"):
			reply("250-fake")
			reply("250 AUTH PLAIN")
		case strings.HasPrefix(command, "AUTH PLAIN"):
			s.record(func() { s.auth = strings.TrimSpace(line[len("AUTH PLAIN"):]) })
			reply("235 Authenticated")
		case strings.HasPrefix(command, "RCPT TO:"):
			s.record(func() { s.rcpt = append(s.rcpt, line[len("RCPT TO:"):]) })
			reply("250 OK")
		case command == "DATA":
			inData = true
			reply("354 Go ahead")
		case command == "QUIT":
			reply("221 Bye")
			return
		default:
			reply("250 OK")
		}
	}
}

func (s *fakeSMTPServer) record(update func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update()
}

func TestIntegrationLegacyEmailImport_UpgradePathSendsThroughImportedRow(t *testing.T) {
	resetLegacyEmailImportTables(t)
	ctx := context.Background()
	enc := legacyImportEncryption(t)
	smtpServer := startFakeSMTPServer(t)
	host, port, err := net.SplitHostPort(smtpServer.addr)
	require.NoError(t, err)

	result := services.ImportLegacyEmailConfig(ctx, legacyImportDeps(enc), legacySMTPBlock(host, port), nil)
	require.Equal(t, services.LegacyEmailImportResult{Imported: true}, result)

	// Exactly one row, with the secret encrypted and no actor.
	require.Equal(t, 1, countInstanceEmailProviderRows(t))
	row, err := NewInstanceEmailProviderRepository(integrationDB).Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, "smtp", row.ProviderType)
	assert.JSONEq(t, `{"host":"`+host+`","port":"`+port+`","username":"relay"}`, string(row.Settings))
	assert.Equal(t, "noreply@legacy.test", row.FromAddress)
	assert.Nil(t, row.UpdatedBy)
	require.NotNil(t, row.SecretEncrypted)
	assert.NotEqual(t, legacyIntegrationSentinel, *row.SecretEncrypted)
	decrypted, err := enc.Decrypt(*row.SecretEncrypted)
	require.NoError(t, err)
	assert.Equal(t, legacyIntegrationSentinel, decrypted)

	// Exactly one import audit entry, redacted.
	entries, _, err := NewInstanceSettingsAuditRepository(integrationDB).
		List(ctx, models.InstanceSettingEmailProvider, 10, nil)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, models.InstanceSettingsAuditActionImport, entries[0].Action)
	assert.Nil(t, entries[0].ActorUserID)
	assert.Nil(t, entries[0].Before)
	assert.NotContains(t, string(entries[0].After), legacyIntegrationSentinel)
	var after map[string]any
	require.NoError(t, json.Unmarshal(entries[0].After, &after))
	assert.Equal(t, "noreply@legacy.test", after["from_address"])

	// The instance sender now resolves to that row, and mail goes out through it.
	resolver := services.NewEmailSenderResolver(
		NewTeamEmailProviderRepository(integrationDB), NewInstanceEmailProviderRepository(integrationDB),
		enc, slog.New(slog.DiscardHandler))
	sender, err := resolver.Resolve(ctx, "")
	require.NoError(t, err)
	require.IsType(t, &implementations.SMTPEmailProvider{}, sender.Provider, "must not be the discard stub")
	assert.Equal(t, services.EmailSenderSourceInstance, sender.Source)
	assert.Equal(t, "noreply@legacy.test", sender.FromAddress)

	message := gomail.NewFullEmailMessage(sender.FromAddress, []string{"user@legacy.test"},
		"Welcome", nil, nil, "", "hello", "<p>hello</p>", nil)
	require.NoError(t, sender.Provider.SendEmail(ctx, &external.OutgoingMessage{Message: message}))

	smtpServer.mu.Lock()
	defer smtpServer.mu.Unlock()
	assert.Equal(t, []string{"<user@legacy.test>"}, smtpServer.rcpt)
	assert.Contains(t, smtpServer.data, "Subject: Welcome")
	assert.NotEmpty(t, smtpServer.auth, "the imported credential authenticated the send")
}

// barrierGetRepo holds every replica after its Get until all replicas have read,
// so each one sees "no row" and the database alone decides who inserts.
type barrierGetRepo struct {
	repositories.InstanceEmailProviderRepository
	readers *sync.WaitGroup
}

func (r barrierGetRepo) Get(ctx context.Context) (*models.InstanceEmailProvider, error) {
	row, err := r.InstanceEmailProviderRepository.Get(ctx)
	r.readers.Done()
	r.readers.Wait()
	return row, err
}

func TestIntegrationLegacyEmailImport_ConcurrentBootsStoreOneRow(t *testing.T) {
	resetLegacyEmailImportTables(t)
	enc := legacyImportEncryption(t)
	legacy := legacySMTPBlock("mail.legacy.test", "2525")

	const replicas = 2
	results := make([]services.LegacyEmailImportResult, replicas)
	var readers, wg sync.WaitGroup
	readers.Add(replicas)
	for i := range replicas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deps := legacyImportDeps(enc)
			deps.Repo = barrierGetRepo{InstanceEmailProviderRepository: deps.Repo, readers: &readers}
			results[i] = services.ImportLegacyEmailConfig(context.Background(), deps, legacy, nil)
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, countInstanceEmailProviderRows(t))
	assert.Equal(t, 1, countEmailProviderImportAudits(t))
	imported := 0
	for _, result := range results {
		assert.False(t, result.Unconfigured)
		if result.Imported {
			imported++
		}
	}
	assert.Equal(t, 1, imported, "exactly one replica imports")
}

func TestIntegrationLegacyEmailImport_ExistingRowIsUntouched(t *testing.T) {
	resetLegacyEmailImportTables(t)
	ctx := context.Background()
	repo := NewInstanceEmailProviderRepository(integrationDB)

	existing := integrationInstanceEmailProvider(t)
	require.NoError(t, repo.Upsert(ctx, existing))
	before, err := repo.Get(ctx)
	require.NoError(t, err)

	result := services.ImportLegacyEmailConfig(ctx, legacyImportDeps(legacyImportEncryption(t)),
		legacySMTPBlock("mail.legacy.test", "2525"), nil)

	assert.Equal(t, services.LegacyEmailImportResult{Ignored: true}, result)
	after, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, before, after, "the stored row must be byte-identical, version and updated_at included")
	assert.Equal(t, 0, countEmailProviderImportAudits(t))
}
