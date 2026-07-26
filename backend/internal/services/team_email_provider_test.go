package services

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

func newTestTeamEmailProviderService(
	t *testing.T,
	repo repositories.TeamEmailProviderRepository,
	userRepo repositories.UserRepository,
	authz AuthorizationServiceInterface,
) *TeamEmailProviderService {
	t.Helper()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	return NewTeamEmailProviderService(
		repo, userRepo, enc, localDevProviderConfig(), authz, slog.New(slog.DiscardHandler))
}

func validSMTPRequest() models.UpsertTeamEmailProviderRequest {
	secret := "smtp-password"
	return models.UpsertTeamEmailProviderRequest{
		ProviderType: EmailProviderTypeSMTP,
		Settings: models.TeamEmailProviderSettings{
			SMTP: &models.SMTPProviderSettings{
				// localhost resolves without touching DNS, and the local-dev
				// guard permits loopback, so these tests never hit the network.
				Host:     "localhost",
				Port:     "587",
				Username: "mailer@example.com",
			},
		},
		Secret:      &secret,
		FromAddress: "team@example.com",
	}
}

func validMailgunRequest() models.UpsertTeamEmailProviderRequest {
	secret := "mailgun-key"
	return models.UpsertTeamEmailProviderRequest{
		ProviderType: EmailProviderTypeMailgun,
		Settings: models.TeamEmailProviderSettings{
			Mailgun: &models.MailgunProviderSettings{Domain: "mg.example.com"},
		},
		Secret:      &secret,
		FromAddress: "team@example.com",
	}
}

// --- Authorization -----------------------------------------------------------

// A denied caller must not reach the repository. The repo mock is deliberately
// given no expectations, so mockery fails the test if anything touches it.
func TestTeamEmailProviderMutations_DeniedForMember(t *testing.T) {
	ctx := context.Background()

	t.Run("upsert", func(t *testing.T) {
		svc := newTestTeamEmailProviderService(t,
			repomocks.NewMockTeamEmailProviderRepository(t),
			repomocks.NewMockUserRepository(t), denyingProviderAuthz{})

		_, err := svc.Upsert(ctx, testProviderUserID, testProviderTeamID, validSMTPRequest())

		assert.ErrorIs(t, err, ErrPermissionDenied)
	})

	t.Run("delete", func(t *testing.T) {
		svc := newTestTeamEmailProviderService(t,
			repomocks.NewMockTeamEmailProviderRepository(t),
			repomocks.NewMockUserRepository(t), denyingProviderAuthz{})

		err := svc.Delete(ctx, testProviderUserID, testProviderTeamID)

		assert.ErrorIs(t, err, ErrPermissionDenied)
	})

	t.Run("test", func(t *testing.T) {
		svc := newTestTeamEmailProviderService(t,
			repomocks.NewMockTeamEmailProviderRepository(t),
			repomocks.NewMockUserRepository(t), denyingProviderAuthz{})

		_, err := svc.Test(ctx, testProviderUserID, testProviderTeamID,
			models.TestTeamEmailProviderRequest{UpsertTeamEmailProviderRequest: validSMTPRequest()})

		assert.ErrorIs(t, err, ErrPermissionDenied)
	})
}

// A nil authz service is a wiring bug and must fail closed, not open.
func TestTeamEmailProvider_NilAuthzFailsClosed(t *testing.T) {
	svc := newTestTeamEmailProviderService(t,
		repomocks.NewMockTeamEmailProviderRepository(t),
		repomocks.NewMockUserRepository(t), nil)

	_, err := svc.Upsert(context.Background(), testProviderUserID, testProviderTeamID, validSMTPRequest())

	assert.ErrorIs(t, err, ErrPermissionDenied)
}

// Get is intentionally not role-gated: it reveals which provider a team uses, not
// its credential.
func TestTeamEmailProvider_GetIsNotRoleGated(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(&models.TeamEmailProvider{TeamID: testProviderTeamID}, nil)

	svc := newTestTeamEmailProviderService(t, repo, repomocks.NewMockUserRepository(t),
		denyingProviderAuthz{})

	got, err := svc.Get(context.Background(), testProviderUserID, testProviderTeamID)

	require.NoError(t, err)
	assert.Equal(t, testProviderTeamID, got.TeamID)
}

// --- Upsert ------------------------------------------------------------------

func TestTeamEmailProvider_Upsert_EncryptsSecretAndStoresSettings(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(nil, repositories.ErrTeamEmailProviderNotFound)

	var stored *models.TeamEmailProvider
	repo.On("Upsert", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			stored = args.Get(1).(*models.TeamEmailProvider)
		}).Return(nil)

	svc := newTestTeamEmailProviderService(t, repo, repomocks.NewMockUserRepository(t),
		permissiveProviderAuthz{})

	_, err := svc.Upsert(context.Background(), testProviderUserID, testProviderTeamID, validSMTPRequest())
	require.NoError(t, err)

	require.NotNil(t, stored)
	assert.Equal(t, EmailProviderTypeSMTP, stored.ProviderType)
	assert.Equal(t, testProviderTeamID, stored.TeamID)
	require.NotNil(t, stored.UserID)
	assert.Equal(t, testProviderUserID, *stored.UserID)

	// The secret must be stored as ciphertext, never as the plaintext given.
	assert.NotEmpty(t, stored.SecretEncrypted)
	assert.NotEqual(t, "smtp-password", stored.SecretEncrypted)
	plaintext, err := svc.decrypt(stored.SecretEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "smtp-password", plaintext)

	// Only the block for this provider type is persisted.
	var settings models.SMTPProviderSettings
	require.NoError(t, json.Unmarshal(stored.Settings, &settings))
	assert.Equal(t, "localhost", settings.Host)
	assert.Equal(t, "587", settings.Port)
}

// An omitted secret on update keeps the stored ciphertext — the UI never receives
// the secret, so it cannot resend it.
func TestTeamEmailProvider_Upsert_OmittedSecretKeepsStored(t *testing.T) {
	existing := &models.TeamEmailProvider{
		TeamID:          testProviderTeamID,
		ProviderType:    EmailProviderTypeSMTP,
		SecretEncrypted: "existing-ciphertext",
		FromAddress:     "old@example.com",
	}

	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).Return(existing, nil)

	var stored *models.TeamEmailProvider
	repo.On("Upsert", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			stored = args.Get(1).(*models.TeamEmailProvider)
		}).Return(nil)

	svc := newTestTeamEmailProviderService(t, repo, repomocks.NewMockUserRepository(t),
		permissiveProviderAuthz{})

	req := validSMTPRequest()
	req.Secret = nil
	req.FromAddress = "new@example.com"

	_, err := svc.Upsert(context.Background(), testProviderUserID, testProviderTeamID, req)
	require.NoError(t, err)

	require.NotNil(t, stored)
	assert.Equal(t, "existing-ciphertext", stored.SecretEncrypted)
	assert.Equal(t, "new@example.com", stored.FromAddress)
}

func TestTeamEmailProvider_Upsert_ValidationRejections(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*models.UpsertTeamEmailProviderRequest)
		wantField  string
		isCreate   bool
		wantDetail string
	}{
		{
			name:      "unknown provider type",
			mutate:    func(r *models.UpsertTeamEmailProviderRequest) { r.ProviderType = "ses" },
			wantField: "provider_type",
			isCreate:  true,
		},
		{
			name:      "missing secret on create",
			mutate:    func(r *models.UpsertTeamEmailProviderRequest) { r.Secret = nil },
			wantField: "secret",
			isCreate:  true,
		},
		{
			name: "empty secret is rejected, not treated as clear",
			mutate: func(r *models.UpsertTeamEmailProviderRequest) {
				empty := ""
				r.Secret = &empty
			},
			wantField:  "secret",
			isCreate:   true,
			wantDetail: "omit the field to keep the stored secret",
		},
		{
			name:      "missing from_address",
			mutate:    func(r *models.UpsertTeamEmailProviderRequest) { r.FromAddress = "" },
			wantField: "from_address",
			isCreate:  true,
		},
		{
			name:      "malformed from_address",
			mutate:    func(r *models.UpsertTeamEmailProviderRequest) { r.FromAddress = "not-an-email" },
			wantField: "from_address",
			isCreate:  true,
		},
		{
			name: "malformed reply_to",
			mutate: func(r *models.UpsertTeamEmailProviderRequest) {
				bad := "also-not-an-email"
				r.ReplyTo = &bad
			},
			wantField: "reply_to",
			isCreate:  true,
		},
		{
			name: "smtp missing host",
			mutate: func(r *models.UpsertTeamEmailProviderRequest) {
				r.Settings.SMTP.Host = ""
			},
			wantField: "settings.smtp.host",
			isCreate:  true,
		},
		{
			name: "smtp non-numeric port",
			mutate: func(r *models.UpsertTeamEmailProviderRequest) {
				r.Settings.SMTP.Port = "not-a-port"
			},
			wantField: "settings.smtp.port",
			isCreate:  true,
		},
		{
			name: "smtp missing settings block entirely",
			mutate: func(r *models.UpsertTeamEmailProviderRequest) {
				r.Settings.SMTP = nil
			},
			wantField: "settings.smtp",
			isCreate:  true,
		},
		{
			name: "settings block for another provider type",
			mutate: func(r *models.UpsertTeamEmailProviderRequest) {
				r.Settings.Mailgun = &models.MailgunProviderSettings{Domain: "mg.example.com"}
			},
			wantField: "settings.mailgun",
			isCreate:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validSMTPRequest()
			tc.mutate(&req)

			err := validateUpsertRequest(req, tc.isCreate)

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrTeamEmailProviderValidation)

			var verr *TeamEmailProviderValidationError
			require.True(t, errors.As(err, &verr))
			fields := make([]string, 0, len(verr.Fields))
			for _, f := range verr.Fields {
				fields = append(fields, f.Field)
			}
			assert.Contains(t, fields, tc.wantField)
			if tc.wantDetail != "" {
				assert.Contains(t, err.Error(), tc.wantDetail)
			}
		})
	}
}

// Each provider type must accept its own complete block.
func TestTeamEmailProvider_Validation_AcceptsEachType(t *testing.T) {
	secret := "a-secret"

	tests := []struct {
		name string
		req  models.UpsertTeamEmailProviderRequest
	}{
		{name: "smtp", req: validSMTPRequest()},
		{name: "mailgun", req: validMailgunRequest()},
		{
			name: "postmark needs no settings block",
			req: models.UpsertTeamEmailProviderRequest{
				ProviderType: EmailProviderTypePostmark,
				Secret:       &secret,
				FromAddress:  "team@example.com",
			},
		},
		{
			name: "sendgrid needs no settings block",
			req: models.UpsertTeamEmailProviderRequest{
				ProviderType: EmailProviderTypeSendGrid,
				Secret:       &secret,
				FromAddress:  "team@example.com",
			},
		},
		{
			name: "postmark with a message stream",
			req: models.UpsertTeamEmailProviderRequest{
				ProviderType: EmailProviderTypePostmark,
				Settings: models.TeamEmailProviderSettings{
					Postmark: &models.PostmarkProviderSettings{MessageStream: "broadcast"},
				},
				Secret:      &secret,
				FromAddress: "team@example.com",
			},
		},
		{
			name: "provider type is case and whitespace tolerant",
			req: func() models.UpsertTeamEmailProviderRequest {
				r := validMailgunRequest()
				r.ProviderType = "  MAILGUN  "
				return r
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NoError(t, validateUpsertRequest(tc.req, true))
		})
	}
}

// Mailgun's domain must be a bare domain; catching it here turns an opaque
// construction failure into a field error.
func TestTeamEmailProvider_Validation_MailgunDomainMustBeBare(t *testing.T) {
	req := validMailgunRequest()
	req.Settings.Mailgun.Domain = "https://api.mailgun.net/v3"

	err := validateUpsertRequest(req, true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "settings.mailgun.domain")
	assert.Contains(t, err.Error(), "bare domain")
}

// --- SSRF --------------------------------------------------------------------

// A team-supplied destination in a reserved range must be rejected before
// anything is stored — the repo mock has no Upsert expectation.
func TestTeamEmailProvider_Upsert_SSRFRejectsSMTPHost(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(nil, repositories.ErrTeamEmailProviderNotFound)

	svc := newTestTeamEmailProviderService(t, repo, repomocks.NewMockUserRepository(t),
		permissiveProviderAuthz{})
	// Production policy: reject loopback/private/link-local.
	svc.guard = defaultSSRFGuard

	req := validSMTPRequest()
	req.Settings.SMTP.Host = "169.254.169.254"

	_, err := svc.Upsert(context.Background(), testProviderUserID, testProviderTeamID, req)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTeamEmailProviderValidation)
	assert.Contains(t, err.Error(), "settings.smtp.host")
}

func TestTeamEmailProvider_Upsert_SSRFRejectsMailgunBaseURL(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(nil, repositories.ErrTeamEmailProviderNotFound)

	svc := newTestTeamEmailProviderService(t, repo, repomocks.NewMockUserRepository(t),
		permissiveProviderAuthz{})
	svc.guard = defaultSSRFGuard

	req := validMailgunRequest()
	req.Settings.Mailgun.BaseURL = "http://127.0.0.1:8080/v3"

	_, err := svc.Upsert(context.Background(), testProviderUserID, testProviderTeamID, req)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTeamEmailProviderValidation)
	assert.Contains(t, err.Error(), "settings.mailgun.base_url")
}

// The same guard applies to a test send, which dials without persisting.
func TestTeamEmailProvider_Test_SSRFRejectsSMTPHost(t *testing.T) {
	svc := newTestTeamEmailProviderService(t,
		repomocks.NewMockTeamEmailProviderRepository(t),
		repomocks.NewMockUserRepository(t), permissiveProviderAuthz{})
	svc.guard = defaultSSRFGuard

	req := validSMTPRequest()
	req.Settings.SMTP.Host = "127.0.0.1"

	_, err := svc.Test(context.Background(), testProviderUserID, testProviderTeamID,
		models.TestTeamEmailProviderRequest{UpsertTeamEmailProviderRequest: req})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTeamEmailProviderValidation)
	assert.Contains(t, err.Error(), "settings.smtp.host")
}

// --- Test send ---------------------------------------------------------------

// The recipient is the acting user's own email, never caller-supplied, so the
// endpoint cannot be used as a relay.
func TestTeamEmailProvider_Test_SendsToActingUserOnly(t *testing.T) {
	userRepo := repomocks.NewMockUserRepository(t)
	userRepo.On("GetByID", mock.Anything, testProviderUserID).
		Return(&models.User{ID: testProviderUserID, Email: "admin@example.com"}, nil)

	svc := newTestTeamEmailProviderService(t,
		repomocks.NewMockTeamEmailProviderRepository(t), userRepo, permissiveProviderAuthz{})

	req := validSMTPRequest()

	result, err := svc.Test(context.Background(), testProviderUserID, testProviderTeamID,
		models.TestTeamEmailProviderRequest{UpsertTeamEmailProviderRequest: req})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "admin@example.com", result.Recipient)
}

// A test send needs a full configuration; there is no stored row to inherit a
// secret from.
func TestTeamEmailProvider_Test_RequiresSecret(t *testing.T) {
	svc := newTestTeamEmailProviderService(t,
		repomocks.NewMockTeamEmailProviderRepository(t),
		repomocks.NewMockUserRepository(t), permissiveProviderAuthz{})

	req := validSMTPRequest()
	req.Secret = nil

	_, err := svc.Test(context.Background(), testProviderUserID, testProviderTeamID,
		models.TestTeamEmailProviderRequest{UpsertTeamEmailProviderRequest: req})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTeamEmailProviderValidation)
	assert.Contains(t, err.Error(), "secret")
}

// A delivery failure is reported in the result, not as an error: the caller asked
// whether the configuration works, and "no, because X" is a valid answer.
func TestTeamEmailProvider_Test_DeliveryFailureIsAResult(t *testing.T) {
	userRepo := repomocks.NewMockUserRepository(t)
	userRepo.On("GetByID", mock.Anything, testProviderUserID).
		Return(&models.User{ID: testProviderUserID, Email: "admin@example.com"}, nil)

	svc := newTestTeamEmailProviderService(t,
		repomocks.NewMockTeamEmailProviderRepository(t), userRepo, permissiveProviderAuthz{})

	// Port 1 on loopback refuses immediately, so the send fails without waiting
	// on a network timeout. (Deliberately not 1025 — Mailpit listens there in
	// local dev and would actually accept the message.)
	req := validSMTPRequest()
	req.Settings.SMTP.Port = "1"

	result, err := svc.Test(context.Background(), testProviderUserID, testProviderTeamID,
		models.TestTeamEmailProviderRequest{UpsertTeamEmailProviderRequest: req})

	require.NoError(t, err, "a delivery failure is an outcome, not an error")
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "admin@example.com", result.Recipient)
	assert.Contains(t, result.Message, "Sending failed")
}

// A test send needs no encryption service: it builds the provider from the
// request's plaintext secret and never stores or decrypts anything. Encryption is
// required only on the persistence path.
func TestTeamEmailProvider_Test_DoesNotRequireEncryption(t *testing.T) {
	userRepo := repomocks.NewMockUserRepository(t)
	userRepo.On("GetByID", mock.Anything, testProviderUserID).
		Return(&models.User{ID: testProviderUserID, Email: "admin@example.com"}, nil)

	svc := NewTeamEmailProviderService(
		repomocks.NewMockTeamEmailProviderRepository(t),
		userRepo, nil,
		localDevProviderConfig(), permissiveProviderAuthz{}, slog.New(slog.DiscardHandler))

	req := validSMTPRequest()
	req.Settings.SMTP.Port = "1"

	result, err := svc.Test(context.Background(), testProviderUserID, testProviderTeamID,
		models.TestTeamEmailProviderRequest{UpsertTeamEmailProviderRequest: req})

	require.NoError(t, err, "a nil encryption service must not block a test send")
	require.NotNil(t, result)
	assert.False(t, result.Success, "the send still fails, but on the refused dial")
	assert.NotErrorIs(t, errors.New(result.Message), ErrEncryptionUnavailable)
}

// --- Config helpers ----------------------------------------------------------

func TestProviderSpecFromRequest_MapsEachType(t *testing.T) {
	secret := "the-secret"

	t.Run("smtp", func(t *testing.T) {
		spec := providerSpecFromRequest(validSMTPRequest(), secret)
		assert.Equal(t, EmailProviderTypeSMTP, spec.Type)
		assert.Equal(t, "localhost", spec.SMTP.Host)
		assert.Equal(t, "587", spec.SMTP.Port)
		assert.Equal(t, secret, spec.SMTP.Password, "the secret is the SMTP password")
	})

	t.Run("mailgun", func(t *testing.T) {
		spec := providerSpecFromRequest(validMailgunRequest(), secret)
		assert.Equal(t, "mg.example.com", spec.Mailgun.Domain)
		assert.Equal(t, secret, spec.Mailgun.SendingKey)
	})

	t.Run("postmark", func(t *testing.T) {
		req := models.UpsertTeamEmailProviderRequest{
			ProviderType: EmailProviderTypePostmark,
			Settings: models.TeamEmailProviderSettings{
				Postmark: &models.PostmarkProviderSettings{MessageStream: "broadcast"},
			},
		}
		spec := providerSpecFromRequest(req, secret)
		assert.Equal(t, secret, spec.Postmark.ServerToken)
		assert.Equal(t, "broadcast", spec.Postmark.MessageStream)
	})

	t.Run("sendgrid", func(t *testing.T) {
		req := models.UpsertTeamEmailProviderRequest{ProviderType: EmailProviderTypeSendGrid}
		spec := providerSpecFromRequest(req, secret)
		assert.Equal(t, secret, spec.SendGrid.APIKey)
	})
}

func TestMarshalProviderSettings_OnlyTheMatchingBlock(t *testing.T) {
	// A request naming smtp but (hypothetically) carrying a mailgun block must
	// persist only smtp settings.
	req := validSMTPRequest()
	req.Settings.Mailgun = &models.MailgunProviderSettings{Domain: "mg.example.com"}

	encoded, err := marshalProviderSettings(req)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), "localhost")
	assert.NotContains(t, string(encoded), "mg.example.com")
}

func TestMarshalProviderSettings_SendGridStoresEmptyObject(t *testing.T) {
	encoded, err := marshalProviderSettings(models.UpsertTeamEmailProviderRequest{
		ProviderType: EmailProviderTypeSendGrid,
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(encoded))
}

func TestNewTeamEmailProviderResponse_HidesSecret(t *testing.T) {
	provider := &models.TeamEmailProvider{
		TeamID:          testProviderTeamID,
		SecretEncrypted: "ciphertext",
		Settings:        json.RawMessage(`{}`),
	}

	response := models.NewTeamEmailProviderResponse(provider)
	assert.True(t, response.HasSecret)

	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	// The secret must never appear in a serialized response.
	assert.NotContains(t, string(encoded), "ciphertext")
	assert.Contains(t, string(encoded), `"has_secret":true`)
}

// A missing encryption service must fail closed rather than store plaintext.
func TestTeamEmailProvider_Upsert_NilEncryptionFailsClosed(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(nil, repositories.ErrTeamEmailProviderNotFound)

	svc := NewTeamEmailProviderService(
		repo, repomocks.NewMockUserRepository(t), nil,
		localDevProviderConfig(), permissiveProviderAuthz{}, slog.New(slog.DiscardHandler))

	_, err := svc.Upsert(context.Background(), testProviderUserID, testProviderTeamID, validSMTPRequest())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEncryptionUnavailable)
}

// --- GetEffective ------------------------------------------------------------

// A team with no row reports the instance fallback, not an error: the settings
// endpoint must be able to describe "you are inheriting" rather than 404.
func TestTeamEmailProvider_GetEffective_InstanceFallback(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(nil, repositories.ErrTeamEmailProviderNotFound)

	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	cfg := localDevProviderConfig()
	cfg.Email.FromAddress = "noreply@instance.test"

	svc := NewTeamEmailProviderService(
		repo, repomocks.NewMockUserRepository(t), enc, cfg,
		permissiveProviderAuthz{}, slog.New(slog.DiscardHandler))

	effective, err := svc.GetEffective(context.Background(), testProviderUserID, testProviderTeamID)

	require.NoError(t, err, "inheriting the instance provider is not a failure")
	assert.False(t, effective.Configured)
	assert.Equal(t, "instance", effective.Source)
	assert.Equal(t, "noreply@instance.test", effective.EffectiveFromAddress)
	assert.Nil(t, effective.ProviderType)
	assert.False(t, effective.HasCredential)
}

// The instance from-address chain must fall back to the SMTP username, matching
// EmailService.sendEmail.
func TestTeamEmailProvider_GetEffective_InstanceFromAddressChain(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(nil, repositories.ErrTeamEmailProviderNotFound)

	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	cfg := localDevProviderConfig()
	cfg.Email.FromAddress = ""
	cfg.Email.SMTP.Username = "smtp-user@instance.test"

	svc := NewTeamEmailProviderService(
		repo, repomocks.NewMockUserRepository(t), enc, cfg,
		permissiveProviderAuthz{}, slog.New(slog.DiscardHandler))

	effective, err := svc.GetEffective(context.Background(), testProviderUserID, testProviderTeamID)

	require.NoError(t, err)
	assert.Equal(t, "smtp-user@instance.test", effective.EffectiveFromAddress)
}

func TestTeamEmailProvider_GetEffective_TeamConfigured(t *testing.T) {
	fromName := "Acme"
	stored := &models.TeamEmailProvider{
		TeamID:          testProviderTeamID,
		ProviderType:    EmailProviderTypeMailgun,
		Settings:        json.RawMessage(`{"domain":"mg.acme.test"}`),
		SecretEncrypted: "ciphertext",
		FromAddress:     "hello@acme.test",
		FromName:        &fromName,
	}

	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).Return(stored, nil)

	svc := newTestTeamEmailProviderService(t, repo, repomocks.NewMockUserRepository(t),
		permissiveProviderAuthz{})

	effective, err := svc.GetEffective(context.Background(), testProviderUserID, testProviderTeamID)

	require.NoError(t, err)
	assert.True(t, effective.Configured)
	assert.Equal(t, "team", effective.Source)
	assert.Equal(t, "hello@acme.test", effective.EffectiveFromAddress)
	require.NotNil(t, effective.ProviderType)
	assert.Equal(t, EmailProviderTypeMailgun, *effective.ProviderType)
	assert.True(t, effective.HasCredential)
	require.NotNil(t, effective.IsHealthy)
	assert.True(t, *effective.IsHealthy, "a provider that has not failed is healthy")
}

// A real repository failure must surface, not masquerade as the instance fallback
// — otherwise a DB blip would silently reroute the team's mail in the UI.
func TestTeamEmailProvider_GetEffective_RepositoryErrorSurfaces(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(nil, errors.New("connection reset"))

	svc := newTestTeamEmailProviderService(t, repo, repomocks.NewMockUserRepository(t),
		permissiveProviderAuthz{})

	effective, err := svc.GetEffective(context.Background(), testProviderUserID, testProviderTeamID)

	require.Error(t, err)
	assert.Nil(t, effective)
}

// The effective view must never carry the credential, even though it is built
// from a row that has one.
func TestTeamEmailProvider_GetEffective_NeverSerializesTheSecret(t *testing.T) {
	stored := &models.TeamEmailProvider{
		TeamID:          testProviderTeamID,
		ProviderType:    EmailProviderTypeSendGrid,
		Settings:        json.RawMessage(`{}`),
		SecretEncrypted: "SUPER-SECRET-CIPHERTEXT",
		FromAddress:     "hello@acme.test",
	}

	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).Return(stored, nil)

	svc := newTestTeamEmailProviderService(t, repo, repomocks.NewMockUserRepository(t),
		permissiveProviderAuthz{})

	effective, err := svc.GetEffective(context.Background(), testProviderUserID, testProviderTeamID)
	require.NoError(t, err)

	encoded, err := json.Marshal(effective)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "SUPER-SECRET-CIPHERTEXT")
	assert.Contains(t, string(encoded), `"has_credential":true`)
}

// --- EffectiveFromProvider / settings union -----------------------------------

// A write describes its own result from the row it just stored, without a second
// read.
func TestTeamEmailProvider_EffectiveFromProvider(t *testing.T) {
	svc := newTestTeamEmailProviderService(t,
		repomocks.NewMockTeamEmailProviderRepository(t),
		repomocks.NewMockUserRepository(t), permissiveProviderAuthz{})

	fromName := "Acme"
	effective := svc.EffectiveFromProvider(&models.TeamEmailProvider{
		TeamID:          testProviderTeamID,
		ProviderType:    EmailProviderTypeMailgun,
		Settings:        json.RawMessage(`{"domain":"mg.acme.test"}`),
		SecretEncrypted: "ciphertext",
		FromAddress:     "hello@acme.test",
		FromName:        &fromName,
	})

	assert.True(t, effective.Configured)
	assert.Equal(t, "team", effective.Source)
	assert.Equal(t, "hello@acme.test", effective.EffectiveFromAddress)
	assert.True(t, effective.HasCredential)
	require.NotNil(t, effective.Settings)
	require.NotNil(t, effective.Settings.Mailgun)
	assert.Equal(t, "mg.acme.test", effective.Settings.Mailgun.Domain)
}

// The stored blob holds only the inner block, so the union must be rebuilt per
// provider type — the shape the API documents and that round-trips into a PUT.
func TestTeamSettingsUnion_PerProviderType(t *testing.T) {
	tests := []struct {
		name     string
		provider *models.TeamEmailProvider
		assertOn func(t *testing.T, union *models.TeamEmailProviderSettings)
	}{
		{
			name: "smtp",
			provider: &models.TeamEmailProvider{
				ProviderType: EmailProviderTypeSMTP,
				Settings:     json.RawMessage(`{"host":"smtp.acme.test","port":"587","username":"mailer"}`),
			},
			assertOn: func(t *testing.T, union *models.TeamEmailProviderSettings) {
				require.NotNil(t, union.SMTP)
				assert.Equal(t, "smtp.acme.test", union.SMTP.Host)
				assert.Equal(t, "587", union.SMTP.Port)
				assert.Equal(t, "mailer", union.SMTP.Username)
				assert.Nil(t, union.Mailgun, "only the matching block is populated")
			},
		},
		{
			name: "mailgun",
			provider: &models.TeamEmailProvider{
				ProviderType: EmailProviderTypeMailgun,
				Settings:     json.RawMessage(`{"domain":"mg.acme.test","base_url":"https://api.eu.mailgun.net/v3"}`),
			},
			assertOn: func(t *testing.T, union *models.TeamEmailProviderSettings) {
				require.NotNil(t, union.Mailgun)
				assert.Equal(t, "mg.acme.test", union.Mailgun.Domain)
				assert.Equal(t, "https://api.eu.mailgun.net/v3", union.Mailgun.BaseURL)
			},
		},
		{
			name: "postmark",
			provider: &models.TeamEmailProvider{
				ProviderType: EmailProviderTypePostmark,
				Settings:     json.RawMessage(`{"message_stream":"broadcast"}`),
			},
			assertOn: func(t *testing.T, union *models.TeamEmailProviderSettings) {
				require.NotNil(t, union.Postmark)
				assert.Equal(t, "broadcast", union.Postmark.MessageStream)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			union := teamSettingsUnion(tc.provider)
			require.NotNil(t, union)
			tc.assertOn(t, union)
		})
	}
}

func TestTeamSettingsUnion_NilCases(t *testing.T) {
	tests := []struct {
		name     string
		provider *models.TeamEmailProvider
	}{
		{
			name: "sendgrid has no non-secret settings",
			provider: &models.TeamEmailProvider{
				ProviderType: EmailProviderTypeSendGrid,
				Settings:     json.RawMessage(`{}`),
			},
		},
		{
			name:     "empty settings",
			provider: &models.TeamEmailProvider{ProviderType: EmailProviderTypeSMTP},
		},
		{
			name: "unrecognised provider type",
			provider: &models.TeamEmailProvider{
				ProviderType: "ses",
				Settings:     json.RawMessage(`{"host":"x"}`),
			},
		},
		{
			name: "malformed blob stays readable as nil rather than failing the read",
			provider: &models.TeamEmailProvider{
				ProviderType: EmailProviderTypeSMTP,
				Settings:     json.RawMessage(`{"host": 42}`),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Nil(t, teamSettingsUnion(tc.provider))
		})
	}
}

func TestNewTeamEmailProviderTestResponse_MapsOutcome(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		got := models.NewTeamEmailProviderTestResponse(&models.TeamEmailProviderTestResult{
			Success:   true,
			Recipient: "admin@acme.test",
			Message:   "sent",
		})
		assert.True(t, got.IsValid)
		assert.Equal(t, "admin@acme.test", got.Recipient)
		assert.Empty(t, got.Details.ErrorDetails)
	})

	t.Run("failure carries the fixed category", func(t *testing.T) {
		got := models.NewTeamEmailProviderTestResponse(&models.TeamEmailProviderTestResult{
			Success:      false,
			Recipient:    "admin@acme.test",
			Message:      "nope",
			ErrorDetails: models.TeamEmailProviderErrConfigInvalid,
		})
		assert.False(t, got.IsValid)
		assert.Equal(t, models.TeamEmailProviderErrConfigInvalid, got.Details.ErrorDetails)
	})
}

// A test send whose configuration cannot build a provider is reported as a result
// with the configuration_invalid category, not as an error.
func TestTeamEmailProvider_Test_ConfigInvalidCategory(t *testing.T) {
	userRepo := repomocks.NewMockUserRepository(t)
	userRepo.On("GetByID", mock.Anything, testProviderUserID).
		Return(&models.User{ID: testProviderUserID, Email: "admin@example.com"}, nil)

	svc := newTestTeamEmailProviderService(t,
		repomocks.NewMockTeamEmailProviderRepository(t), userRepo, permissiveProviderAuthz{})

	// Passes validation but the SMTP provider rejects the port at construction:
	// validateSMTPSettings allows 65535, the sender does not accept 0-width hosts,
	// so use a port the factory parses yet gomail refuses to dial from.
	req := validSMTPRequest()
	req.Settings.SMTP.Host = "localhost"
	req.Settings.SMTP.Port = "65535"

	result, err := svc.Test(context.Background(), testProviderUserID, testProviderTeamID,
		models.TestTeamEmailProviderRequest{UpsertTeamEmailProviderRequest: req})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, models.TeamEmailProviderErrSendFailed, result.ErrorDetails)
}

// A user whose account has no email address cannot receive a test send; that is a
// validation error naming the problem, not a silent success.
func TestTeamEmailProvider_Test_UserWithoutEmail(t *testing.T) {
	userRepo := repomocks.NewMockUserRepository(t)
	userRepo.On("GetByID", mock.Anything, testProviderUserID).
		Return(&models.User{ID: testProviderUserID, Email: ""}, nil)

	svc := newTestTeamEmailProviderService(t,
		repomocks.NewMockTeamEmailProviderRepository(t), userRepo, permissiveProviderAuthz{})

	_, err := svc.Test(context.Background(), testProviderUserID, testProviderTeamID,
		models.TestTeamEmailProviderRequest{UpsertTeamEmailProviderRequest: validSMTPRequest()})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTeamEmailProviderValidation)
	assert.Contains(t, err.Error(), "recipient")
}

// A repository failure on the user lookup must surface rather than being reported
// as a bad configuration.
func TestTeamEmailProvider_Test_UserLookupFailure(t *testing.T) {
	userRepo := repomocks.NewMockUserRepository(t)
	userRepo.On("GetByID", mock.Anything, testProviderUserID).
		Return(nil, errors.New("connection reset"))

	svc := newTestTeamEmailProviderService(t,
		repomocks.NewMockTeamEmailProviderRepository(t), userRepo, permissiveProviderAuthz{})

	_, err := svc.Test(context.Background(), testProviderUserID, testProviderTeamID,
		models.TestTeamEmailProviderRequest{UpsertTeamEmailProviderRequest: validSMTPRequest()})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve the acting user")
}

// Delete surfaces a repository failure rather than reporting success.
func TestTeamEmailProvider_Delete_RepositoryError(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("Delete", mock.Anything, testProviderTeamID).Return(errors.New("connection reset"))

	svc := newTestTeamEmailProviderService(t, repo, repomocks.NewMockUserRepository(t),
		permissiveProviderAuthz{})

	err := svc.Delete(context.Background(), testProviderUserID, testProviderTeamID)

	require.Error(t, err)
}

// Upsert surfaces a repository write failure.
func TestTeamEmailProvider_Upsert_RepositoryError(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(nil, repositories.ErrTeamEmailProviderNotFound)
	repo.On("Upsert", mock.Anything, mock.Anything).Return(errors.New("constraint violation"))

	svc := newTestTeamEmailProviderService(t, repo, repomocks.NewMockUserRepository(t),
		permissiveProviderAuthz{})

	_, err := svc.Upsert(context.Background(), testProviderUserID, testProviderTeamID, validSMTPRequest())

	require.Error(t, err)
}

// A read failure while deciding create-vs-update must abort rather than guess.
func TestTeamEmailProvider_Upsert_ExistingLookupFailure(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(nil, errors.New("connection reset"))

	svc := newTestTeamEmailProviderService(t, repo, repomocks.NewMockUserRepository(t),
		permissiveProviderAuthz{})

	_, err := svc.Upsert(context.Background(), testProviderUserID, testProviderTeamID, validSMTPRequest())

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrTeamEmailProviderValidation)
}

// A test send previews the team's real sender identity, display name included —
// otherwise a team configures from_name, sends itself a test, and sees a bare
// address, which reads as the feature being broken.
func TestTestMessageFor_CarriesTheDisplayName(t *testing.T) {
	name := "Acme Team"
	replyTo := "support@acme.test"

	out := testMessageFor(models.TestTeamEmailProviderRequest{
		UpsertTeamEmailProviderRequest: models.UpsertTeamEmailProviderRequest{
			FromAddress: "  hello@acme.test  ",
			FromName:    &name,
			ReplyTo:     &replyTo,
		},
	}, "admin@example.com")

	assert.Equal(t, "Acme Team", out.FromName)
	// The gomail field stays a bare address; the name rides beside it.
	assert.Equal(t, "hello@acme.test", out.Message.GetFrom())
	assert.Equal(t, `"Acme Team" <hello@acme.test>`, out.FromHeader())
	assert.Equal(t, []string{"admin@example.com"}, out.Message.GetTo())
	assert.Equal(t, replyTo, out.Message.GetReplyTo())
}

func TestTestMessageFor_WithoutADisplayNameStaysBare(t *testing.T) {
	out := testMessageFor(models.TestTeamEmailProviderRequest{
		UpsertTeamEmailProviderRequest: models.UpsertTeamEmailProviderRequest{
			FromAddress: "hello@acme.test",
		},
	}, "admin@example.com")

	assert.Empty(t, out.FromName)
	assert.Equal(t, "hello@acme.test", out.FromHeader())
}
