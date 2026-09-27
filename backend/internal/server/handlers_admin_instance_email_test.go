package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	instanceEmailActingAdmin = "22222222-2222-4222-8222-222222222222"
	instanceEmailAdminEmail  = "operator@instance.test"
	// instanceEmailSecretSentinel is the stored credential's plaintext. It must
	// never appear in any response body.
	instanceEmailSecretSentinel = "instance-secret-sentinel-value"
	instanceEmailSettingsPath   = "/api/v1/admin/settings/email"
)

// adminInstanceEmailSettingsKeys is the complete key set of a configured SMTP
// settings response. Anything else appearing is a leak.
var adminInstanceEmailSettingsKeys = []string{
	"configured", "provider_type", "has_credential", "is_healthy",
	"settings", "settings.smtp", "settings.smtp.host", "settings.smtp.port", "settings.smtp.username",
	"from_address", "from_name", "reply_to", "contact_recipient_address", "privacy_policy_url",
	"last_success_at", "last_error", "last_error_at", "updated_at", "updated_by",
}

// instanceEmailFixture wires the REAL InstanceEmailProviderService over mocked
// repositories and a real encryption service, so a stored credential is
// genuinely present server-side while the responses are asserted.
type instanceEmailFixture struct {
	router   http.Handler
	repo     *repomocks.MockInstanceEmailProviderRepository
	audit    *repomocks.MockInstanceSettingsAuditRepository
	userRepo *repomocks.MockUserRepository
	enc      services.EncryptionServiceInterface
}

func newInstanceEmailFixture(t *testing.T) *instanceEmailFixture {
	t.Helper()
	enc, err := services.NewEncryptionService(strings.Repeat("k", 32))
	require.NoError(t, err)
	f := &instanceEmailFixture{
		repo:     repomocks.NewMockInstanceEmailProviderRepository(t),
		audit:    repomocks.NewMockInstanceSettingsAuditRepository(t),
		userRepo: repomocks.NewMockUserRepository(t),
		enc:      enc,
	}
	svc := services.NewInstanceEmailProviderService(f.repo, f.audit, f.userRepo, enc, slog.New(slog.DiscardHandler))
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
		instanceEmailService: svc,
		instanceAuditRepo:    f.audit,
		userRepo:             f.userRepo,
	})
	f.router = mountAdminStrictRouter(srv)
	return f
}

// newInstanceEmailMockRouter mounts the handlers over a mocked service, for the
// error-mapping cases.
func newInstanceEmailMockRouter(t *testing.T) (http.Handler, *servicesmocks.MockInstanceEmailProviderServiceInterface) {
	t.Helper()
	svc := servicesmocks.NewMockInstanceEmailProviderServiceInterface(t)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{instanceEmailService: svc})
	return mountAdminStrictRouter(srv), svc
}

// storedRow is an SMTP row whose credential is the encrypted sentinel.
func (f *instanceEmailFixture) storedRow(t *testing.T) *models.InstanceEmailProvider {
	t.Helper()
	ciphertext, err := f.enc.Encrypt(instanceEmailSecretSentinel)
	require.NoError(t, err)
	lastErr := "smtp: connection refused"
	updatedBy := instanceEmailActingAdmin
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	return &models.InstanceEmailProvider{
		ProviderType:            "smtp",
		Settings:                json.RawMessage(`{"host":"127.0.0.1","port":"1","username":"relay"}`),
		SecretEncrypted:         &ciphertext,
		FromAddress:             "noreply@instance.test",
		FromName:                strPtr("VibeXP"),
		ReplyTo:                 strPtr("support@instance.test"),
		ContactRecipientAddress: strPtr("hello@instance.test"),
		PrivacyPolicyURL:        strPtr("https://instance.test/privacy"),
		LastSuccessAt:           &now,
		LastError:               &lastErr,
		LastErrorAt:             &now,
		UpdatedAt:               now,
		UpdatedBy:               &updatedBy,
	}
}

func instanceEmailRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req = httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
	}
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, instanceEmailActingAdmin))
}

func serveInstanceEmail(t *testing.T, router http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

// smtpUpsertBody is a full SMTP upsert body. The secret is built from a Go
// value rather than a JSON literal so the fixture does not read as a leaked key.
func smtpUpsertBody(secret *string) map[string]any {
	body := map[string]any{
		"provider_type": "smtp",
		"settings": map[string]any{
			"smtp": map[string]any{"host": "127.0.0.1", "port": "1", "username": "relay"},
		},
		"from_address":              "noreply@instance.test",
		"from_name":                 "VibeXP",
		"contact_recipient_address": "hello@instance.test",
		"privacy_policy_url":        "https://instance.test/privacy",
	}
	if secret != nil {
		body["secret"] = *secret
	}
	return body
}

// --- GET ----------------------------------------------------------------------

func TestGetAdminInstanceEmailSettings_Unconfigured(t *testing.T) {
	f := newInstanceEmailFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)

	req := instanceEmailRequest(t, http.MethodGet, instanceEmailSettingsPath, nil)
	rr := serveInstanceEmail(t, f.router, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.JSONEq(t,
		`{"configured":false,"provider_type":null,"has_credential":false,"is_healthy":null}`,
		rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestGetAdminInstanceEmailSettings_ConfiguredNeverLeaksSecret(t *testing.T) {
	f := newInstanceEmailFixture(t)
	row := f.storedRow(t)
	f.repo.On("Get", mock.Anything).Return(row, nil)

	req := instanceEmailRequest(t, http.MethodGet, instanceEmailSettingsPath, nil)
	rr := serveInstanceEmail(t, f.router, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
	assertBodyExcludes(t, rr.Body.Bytes(), instanceEmailSecretSentinel, *row.SecretEncrypted)
	assertJSONKeyPaths(t, rr.Body.Bytes(), adminInstanceEmailSettingsKeys...)

	var got map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, true, got["configured"])
	assert.Equal(t, "smtp", got["provider_type"])
	assert.Equal(t, true, got["has_credential"])
	assert.Equal(t, "noreply@instance.test", got["from_address"])
	assert.Equal(t, instanceEmailActingAdmin, got["updated_by"])
}

func TestGetAdminInstanceEmailSettings_RepositoryFailureIs500(t *testing.T) {
	f := newInstanceEmailFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, errors.New("db down"))

	req := instanceEmailRequest(t, http.MethodGet, instanceEmailSettingsPath, nil)
	rr := serveInstanceEmail(t, f.router, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.NotContains(t, rr.Body.String(), "db down")
	specconformance.AssertConformsToSpec(t, req, rr)
}

// --- PUT ----------------------------------------------------------------------

func TestUpsertAdminInstanceEmailSettings_CreatesAndNeverEchoesSecret(t *testing.T) {
	f := newInstanceEmailFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)
	var stored *models.InstanceEmailProvider
	f.repo.On("Upsert", mock.Anything, mock.AnythingOfType("*models.InstanceEmailProvider")).
		Run(func(args mock.Arguments) { stored = args.Get(1).(*models.InstanceEmailProvider) }).
		Return(nil)
	var entry *models.InstanceSettingsAuditEntry
	f.audit.On("Append", mock.Anything, mock.AnythingOfType("*models.InstanceSettingsAuditEntry")).
		Run(func(args mock.Arguments) { entry = args.Get(1).(*models.InstanceSettingsAuditEntry) }).
		Return(nil)

	secret := instanceEmailSecretSentinel
	req := instanceEmailRequest(t, http.MethodPut, instanceEmailSettingsPath, smtpUpsertBody(&secret))
	rr := serveInstanceEmail(t, f.router, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
	assertBodyExcludes(t, rr.Body.Bytes(), instanceEmailSecretSentinel)

	require.NotNil(t, stored)
	require.NotNil(t, stored.SecretEncrypted)
	decrypted, err := f.enc.Decrypt(*stored.SecretEncrypted)
	require.NoError(t, err)
	assert.Equal(t, instanceEmailSecretSentinel, decrypted, "the secret is stored, encrypted")
	assertBodyExcludes(t, rr.Body.Bytes(), *stored.SecretEncrypted)
	assert.Equal(t, "hello@instance.test", *stored.ContactRecipientAddress)
	assert.Equal(t, "https://instance.test/privacy", *stored.PrivacyPolicyURL)
	require.NotNil(t, stored.UpdatedBy)
	assert.Equal(t, instanceEmailActingAdmin, *stored.UpdatedBy, "the actor is the calling admin")

	require.NotNil(t, entry)
	require.NotNil(t, entry.ActorUserID)
	assert.Equal(t, instanceEmailActingAdmin, *entry.ActorUserID)
}

// TestUpsertAdminInstanceEmailSettings_OmittedSecretKeepsStored: the handler
// passes a nil secret through, so the stored ciphertext is kept as is.
func TestUpsertAdminInstanceEmailSettings_OmittedSecretKeepsStored(t *testing.T) {
	router, svc := newInstanceEmailMockRouter(t)
	svc.On("Upsert", mock.Anything, instanceEmailActingAdmin,
		mock.MatchedBy(func(req models.UpsertInstanceEmailProviderRequest) bool {
			return req.Secret == nil && req.ProviderType == "smtp" &&
				req.Settings.SMTP != nil && req.Settings.SMTP.Username == "relay"
		})).
		Return(models.NewInstanceEmailProviderEffective(&models.InstanceEmailProvider{
			ProviderType: "smtp", FromAddress: "noreply@instance.test",
		}, nil), nil)

	req := instanceEmailRequest(t, http.MethodPut, instanceEmailSettingsPath, smtpUpsertBody(nil))
	rr := serveInstanceEmail(t, router, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestUpsertAdminInstanceEmailSettings_CredentialFreeSMTPRelay: an SMTP create
// with no secret configures an unauthenticated relay (#1208) — 200, nothing
// stored, and the response reports has_credential false.
func TestUpsertAdminInstanceEmailSettings_CredentialFreeSMTPRelay(t *testing.T) {
	f := newInstanceEmailFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)
	var stored *models.InstanceEmailProvider
	f.repo.On("Upsert", mock.Anything, mock.AnythingOfType("*models.InstanceEmailProvider")).
		Run(func(args mock.Arguments) { stored = args.Get(1).(*models.InstanceEmailProvider) }).
		Return(nil)
	f.audit.On("Append", mock.Anything, mock.AnythingOfType("*models.InstanceSettingsAuditEntry")).Return(nil)

	req := instanceEmailRequest(t, http.MethodPut, instanceEmailSettingsPath, smtpUpsertBody(nil))
	rr := serveInstanceEmail(t, f.router, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
	var got map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, true, got["configured"])
	assert.Equal(t, false, got["has_credential"])
	require.NotNil(t, stored)
	assert.Nil(t, stored.SecretEncrypted)
}

func TestUpsertAdminInstanceEmailSettings_EmptySecretIs400(t *testing.T) {
	f := newInstanceEmailFixture(t)
	f.repo.On("Get", mock.Anything).Return(f.storedRow(t), nil)

	empty := ""
	req := instanceEmailRequest(t, http.MethodPut, instanceEmailSettingsPath, smtpUpsertBody(&empty))
	rr := serveInstanceEmail(t, f.router, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "INSTANCE_EMAIL_PROVIDER_VALIDATION_FAILED")
	assert.Contains(t, rr.Body.String(), `"field":"secret"`)
	specconformance.AssertConformsToSpec(t, req, rr)
	f.repo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
}

func TestUpsertAdminInstanceEmailSettings_UnknownFieldIs400(t *testing.T) {
	router, _ := newInstanceEmailMockRouter(t) // no expectation: never reached

	body := smtpUpsertBody(nil)
	body["password_hint"] = "x"
	req := instanceEmailRequest(t, http.MethodPut, instanceEmailSettingsPath, body)
	rr := serveInstanceEmail(t, router, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "password_hint")
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestUpsertAdminInstanceEmailSettings_ServiceFailureIs500(t *testing.T) {
	router, svc := newInstanceEmailMockRouter(t)
	svc.On("Upsert", mock.Anything, instanceEmailActingAdmin, mock.Anything).
		Return(nil, errors.New("audit append failed"))

	req := instanceEmailRequest(t, http.MethodPut, instanceEmailSettingsPath, smtpUpsertBody(nil))
	rr := serveInstanceEmail(t, router, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// --- DELETE -------------------------------------------------------------------

func TestDeleteAdminInstanceEmailSettings_Removes(t *testing.T) {
	f := newInstanceEmailFixture(t)
	f.repo.On("Get", mock.Anything).Return(f.storedRow(t), nil)
	f.repo.On("Delete", mock.Anything).Return(nil).Once()
	f.audit.On("Append", mock.Anything, mock.MatchedBy(func(e *models.InstanceSettingsAuditEntry) bool {
		return e.Action == models.InstanceSettingsAuditActionDelete &&
			e.ActorUserID != nil && *e.ActorUserID == instanceEmailActingAdmin
	})).Return(nil).Once()

	req := instanceEmailRequest(t, http.MethodDelete, instanceEmailSettingsPath, nil)
	rr := serveInstanceEmail(t, f.router, req)

	require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	assert.Empty(t, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestDeleteAdminInstanceEmailSettings_NothingStoredIs409(t *testing.T) {
	f := newInstanceEmailFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)

	req := instanceEmailRequest(t, http.MethodDelete, instanceEmailSettingsPath, nil)
	rr := serveInstanceEmail(t, f.router, req)

	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "INSTANCE_EMAIL_PROVIDER_NOT_CONFIGURED")
	specconformance.AssertConformsToSpec(t, req, rr)
	f.audit.AssertNotCalled(t, "Append", mock.Anything, mock.Anything)
}

// --- test send ----------------------------------------------------------------

// TestTestAdminInstanceEmailSettings_StoredConfigFailedSendIs200 tests the
// stored configuration (empty body): the SMTP relay at 127.0.0.1:1 refuses, so
// the answer is a 200 with is_valid false, sent to the acting admin — and the
// decrypted stored secret appears nowhere in the body.
func TestTestAdminInstanceEmailSettings_StoredConfigFailedSendIs200(t *testing.T) {
	f := newInstanceEmailFixture(t)
	f.repo.On("Get", mock.Anything).Return(f.storedRow(t), nil)
	f.userRepo.On("GetByID", mock.Anything, instanceEmailActingAdmin).
		Return(&models.User{ID: instanceEmailActingAdmin, Email: instanceEmailAdminEmail}, nil)

	req := instanceEmailRequest(t, http.MethodPost, instanceEmailSettingsPath+"/test", nil)
	rr := serveInstanceEmail(t, f.router, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
	assertBodyExcludes(t, rr.Body.Bytes(), instanceEmailSecretSentinel)

	var got map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, false, got["is_valid"])
	assert.Equal(t, instanceEmailAdminEmail, got["recipient"])
	assert.Equal(t, map[string]any{"error_details": "send_failed"}, got["details"])
}

// TestTestAdminInstanceEmailSettings_EmptyObjectTestsStored: `{}` is the same
// as no body — the service receives no candidate configuration.
func TestTestAdminInstanceEmailSettings_EmptyObjectTestsStored(t *testing.T) {
	router, svc := newInstanceEmailMockRouter(t)
	svc.On("Test", mock.Anything, instanceEmailActingAdmin,
		models.TestInstanceEmailProviderRequest{}).
		Return(&models.TeamEmailProviderTestResult{
			Success: true, Recipient: instanceEmailAdminEmail, Message: "Test email sent",
		}, nil)

	req := instanceEmailRequest(t, http.MethodPost, instanceEmailSettingsPath+"/test", map[string]any{})
	rr := serveInstanceEmail(t, router, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.JSONEq(t,
		`{"is_valid":true,"message":"Test email sent","recipient":"operator@instance.test","details":{}}`,
		rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestTestAdminInstanceEmailSettings_CandidateConfigWithoutSecret passes the
// body through as the candidate, with a nil secret and no recipient override.
func TestTestAdminInstanceEmailSettings_CandidateConfigWithoutSecret(t *testing.T) {
	router, svc := newInstanceEmailMockRouter(t)
	svc.On("Test", mock.Anything, instanceEmailActingAdmin,
		mock.MatchedBy(func(req models.TestInstanceEmailProviderRequest) bool {
			return req.Recipient == nil && req.Config != nil && req.Config.Secret == nil &&
				req.Config.ProviderType == "smtp" && req.Config.FromAddress == "noreply@instance.test"
		})).
		Return(&models.TeamEmailProviderTestResult{
			Success: false, Recipient: instanceEmailAdminEmail, Message: "Sending failed",
			ErrorDetails: models.TeamEmailProviderErrSendFailed,
		}, nil)

	req := instanceEmailRequest(t, http.MethodPost, instanceEmailSettingsPath+"/test", smtpUpsertBody(nil))
	rr := serveInstanceEmail(t, router, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestTestAdminInstanceEmailSettings_RecipientIsRejected: the recipient is
// always the acting admin, so a body naming one is refused, not honoured.
func TestTestAdminInstanceEmailSettings_RecipientIsRejected(t *testing.T) {
	router, _ := newInstanceEmailMockRouter(t) // no expectation: never reached

	req := instanceEmailRequest(t, http.MethodPost, instanceEmailSettingsPath+"/test",
		map[string]any{"recipient": "attacker@example.com"})
	rr := serveInstanceEmail(t, router, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "recipient")
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestTestAdminInstanceEmailSettings_NothingStoredIs400(t *testing.T) {
	f := newInstanceEmailFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)
	f.userRepo.On("GetByID", mock.Anything, instanceEmailActingAdmin).
		Return(&models.User{ID: instanceEmailActingAdmin, Email: instanceEmailAdminEmail}, nil)

	req := instanceEmailRequest(t, http.MethodPost, instanceEmailSettingsPath+"/test", nil)
	rr := serveInstanceEmail(t, f.router, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "INSTANCE_EMAIL_PROVIDER_VALIDATION_FAILED")
	specconformance.AssertConformsToSpec(t, req, rr)
}

// --- audit --------------------------------------------------------------------

func TestListAdminInstanceEmailSettingsAudit_FirstPage(t *testing.T) {
	f := newInstanceEmailFixture(t)
	entryID := uuid.NewString()
	nextID := uuid.NewString()
	createdAt := time.Date(2026, 9, 27, 9, 0, 0, 123456000, time.UTC)
	actor := instanceEmailActingAdmin
	// A snapshot carrying keys outside the allowlist, including a plaintext
	// secret under a non-marker value: the read filter must drop all of it.
	after, err := json.Marshal(map[string]any{
		"provider_type":    "smtp",
		"has_credential":   true,
		"secret":           instanceEmailSecretSentinel,
		"secret_encrypted": instanceEmailSecretSentinel,
		"password":         instanceEmailSecretSentinel,
	})
	require.NoError(t, err)
	before, err := json.Marshal(map[string]any{"provider_type": "smtp", "secret": "unchanged"})
	require.NoError(t, err)

	f.audit.On("List", mock.Anything, models.InstanceSettingEmailProvider, adminInstanceAuditDefaultLimit,
		(*models.InstanceSettingsAuditCursor)(nil)).
		Return([]*models.InstanceSettingsAuditEntry{{
			ID: entryID, Setting: models.InstanceSettingEmailProvider,
			Action: models.InstanceSettingsAuditActionUpsert, ActorUserID: &actor,
			Before: before, After: after, CreatedAt: createdAt,
		}, {
			ID: uuid.NewString(), Setting: models.InstanceSettingEmailProvider,
			Action: models.InstanceSettingsAuditActionImport, CreatedAt: createdAt.Add(-time.Hour),
		}}, &models.InstanceSettingsAuditCursor{CreatedAt: createdAt, ID: nextID}, nil)
	f.userRepo.On("GetNamesByIDs", mock.Anything, []string{actor}).
		Return(map[string]string{actor: "Ada Admin"}, nil)

	req := instanceEmailRequest(t, http.MethodGet, instanceEmailSettingsPath+"/audit", nil)
	rr := serveInstanceEmail(t, f.router, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
	assertBodyExcludes(t, rr.Body.Bytes(), instanceEmailSecretSentinel)
	assertJSONKeyPaths(t, rr.Body.Bytes(),
		"entries", "entries[].id", "entries[].setting", "entries[].action", "entries[].actor_user_id",
		"entries[].actor_name", "entries[].before", "entries[].after", "entries[].created_at",
		"entries[].before.provider_type", "entries[].before.secret",
		"entries[].after.provider_type", "entries[].after.has_credential",
		"next_cursor")

	var page struct {
		Entries []struct {
			ActorName *string        `json:"actor_name"`
			Action    string         `json:"action"`
			Before    map[string]any `json:"before"`
			After     map[string]any `json:"after"`
		} `json:"entries"`
		NextCursor *string `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
	require.Len(t, page.Entries, 2)
	require.NotNil(t, page.Entries[0].ActorName)
	assert.Equal(t, "Ada Admin", *page.Entries[0].ActorName)
	assert.Equal(t, "unchanged", page.Entries[0].Before["secret"], "a redaction marker is kept")
	assert.Nil(t, page.Entries[1].ActorName, "an import has no actor")
	assert.Nil(t, page.Entries[1].Before)
	assert.Nil(t, page.Entries[1].After)

	require.NotNil(t, page.NextCursor)
	decoded, err := decodeAdminInstanceAuditCursor(*page.NextCursor)
	require.NoError(t, err)
	assert.Equal(t, nextID, decoded.ID)
	assert.True(t, createdAt.Equal(decoded.CreatedAt))
}

// TestListAdminInstanceEmailSettingsAudit_CursorAndLimitPassThrough: a cursor
// from a previous page round-trips to the repository, and the last page reads
// `entries: []` with a null cursor.
func TestListAdminInstanceEmailSettingsAudit_CursorAndLimitPassThrough(t *testing.T) {
	f := newInstanceEmailFixture(t)
	position := models.InstanceSettingsAuditCursor{
		CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 1, time.UTC), ID: uuid.NewString(),
	}
	cursor, err := encodeAdminInstanceAuditCursor(position)
	require.NoError(t, err)
	f.audit.On("List", mock.Anything, models.InstanceSettingEmailProvider, 5,
		mock.MatchedBy(func(c *models.InstanceSettingsAuditCursor) bool {
			return c != nil && c.ID == position.ID && c.CreatedAt.Equal(position.CreatedAt)
		})).
		Return(nil, (*models.InstanceSettingsAuditCursor)(nil), nil)

	req := instanceEmailRequest(t, http.MethodGet,
		instanceEmailSettingsPath+"/audit?limit=5&cursor="+cursor, nil)
	rr := serveInstanceEmail(t, f.router, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.JSONEq(t, `{"entries":[],"next_cursor":null}`, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestListAdminInstanceEmailSettingsAudit_BadParamsAre400(t *testing.T) {
	badCursor, err := json.Marshal(map[string]string{"t": "not-a-time", "id": uuid.NewString()})
	require.NoError(t, err)
	for name, query := range map[string]string{
		"limit zero":          "?limit=0",
		"limit above max":     "?limit=101",
		"cursor not base64":   "?cursor=%25%25%25",
		"cursor not json":     "?cursor=bm90LWpzb24",
		"cursor bad time":     "?cursor=" + encodeBase64URL(badCursor),
		"cursor bad entry id": "?cursor=" + encodeBase64URL([]byte(`{"t":"2026-09-01T00:00:00Z","id":"x"}`)),
	} {
		t.Run(name, func(t *testing.T) {
			f := newInstanceEmailFixture(t) // no List expectation: never reached

			req := instanceEmailRequest(t, http.MethodGet, instanceEmailSettingsPath+"/audit"+query, nil)
			rr := serveInstanceEmail(t, f.router, req)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

func TestListAdminInstanceEmailSettingsAudit_RepositoryFailureIs500(t *testing.T) {
	f := newInstanceEmailFixture(t)
	f.audit.On("List", mock.Anything, models.InstanceSettingEmailProvider, adminInstanceAuditDefaultLimit,
		(*models.InstanceSettingsAuditCursor)(nil)).
		Return(nil, (*models.InstanceSettingsAuditCursor)(nil), errors.New("db down"))

	req := instanceEmailRequest(t, http.MethodGet, instanceEmailSettingsPath+"/audit", nil)
	rr := serveInstanceEmail(t, f.router, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestListAdminInstanceEmailSettingsAudit_NameLookupFailureDegrades: an actor
// name that cannot be resolved is null, not a failed read.
func TestListAdminInstanceEmailSettingsAudit_NameLookupFailureDegrades(t *testing.T) {
	f := newInstanceEmailFixture(t)
	actor := instanceEmailActingAdmin
	f.audit.On("List", mock.Anything, models.InstanceSettingEmailProvider, adminInstanceAuditDefaultLimit,
		(*models.InstanceSettingsAuditCursor)(nil)).
		Return([]*models.InstanceSettingsAuditEntry{{
			ID: uuid.NewString(), Setting: models.InstanceSettingEmailProvider,
			Action: models.InstanceSettingsAuditActionDelete, ActorUserID: &actor, CreatedAt: time.Now(),
		}}, (*models.InstanceSettingsAuditCursor)(nil), nil)
	f.userRepo.On("GetNamesByIDs", mock.Anything, []string{actor}).Return(nil, errors.New("db down"))

	req := instanceEmailRequest(t, http.MethodGet, instanceEmailSettingsPath+"/audit", nil)
	rr := serveInstanceEmail(t, f.router, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), `"actor_name":null`)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func encodeBase64URL(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}
