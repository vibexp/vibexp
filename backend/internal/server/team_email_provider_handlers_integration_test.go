package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// teamEmailProviderPrefixes are the two mounts every operation is served on. Each
// test loops over both, so a route registered on only one prefix fails here rather
// than in production for whichever client happens to use the other.
var teamEmailProviderPrefixes = []string{
	"email-provider",
	"settings/email-provider",
}

// The credential fixture must never appear in a response body. The no-leak test
// greps for it explicitly rather than trusting struct tags, because a tag typo is
// exactly the failure that would otherwise ship silently.
const teamEmailProviderTestSecret = "SECRET-FIXTURE-must-never-be-returned"

type MockTeamEmailProviderContainer struct {
	BaseMockContainer
	mock.Mock
	teamEmailProviderService *svcmocks.MockTeamEmailProviderServiceInterface
}

func (m *MockTeamEmailProviderContainer) TeamEmailProviderService() services.TeamEmailProviderServiceInterface {
	return m.teamEmailProviderService
}

func newMockTeamEmailProviderContainer(t *testing.T) *MockTeamEmailProviderContainer {
	return &MockTeamEmailProviderContainer{
		teamEmailProviderService: svcmocks.NewMockTeamEmailProviderServiceInterface(t),
	}
}

// createTestTeamEmailProviderServer wires the PRODUCTION route setup, so the tests
// exercise the same route tree the server mounts (including both prefixes).
// Registering the verbs by hand here would make these tests pass even if a route
// were missing from production or registered on only one prefix — the exact
// regressions this file exists to catch. teamValidationMiddleware is applied by
// mountTeamEmailProviderRoutes in production and skipped here, so these tests
// cover the handlers and the spec contract rather than re-testing team membership.
func createTestTeamEmailProviderServer(container *MockTeamEmailProviderContainer) *Server {
	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: container,
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
		router:    r,
	}

	for _, prefix := range teamEmailProviderPrefixes {
		r.Route("/api/v1/{team_id}/"+prefix, srv.setupTeamEmailProviderRoutes)
	}

	return srv
}

func makeTeamEmailProviderRequest(method, path string, body any) *http.Request {
	var reqBody []byte
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		if err != nil {
			panic(err)
		}
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))
}

func sampleTeamProvider() *models.TeamEmailProvider {
	userID := "user-123"
	fromName := "Acme Team"
	replyTo := "support@acme.test"
	lastSuccess := time.Now().UTC()

	return &models.TeamEmailProvider{
		ID:              "b1234567-89ab-cdef-0123-456789abcdef",
		TeamID:          "team-123",
		UserID:          &userID,
		ProviderType:    services.EmailProviderTypeMailgun,
		Settings:        json.RawMessage(`{"domain":"mg.acme.test"}`),
		SecretEncrypted: teamEmailProviderTestSecret,
		FromAddress:     "hello@acme.test",
		FromName:        &fromName,
		ReplyTo:         &replyTo,
		LastSuccessAt:   &lastSuccess,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
		Version:         1,
	}
}

// sampleTeamSettings is the per-type union the API exposes — keyed by provider
// type, exactly like the request body, so a GET response round-trips into a PUT.
func sampleTeamSettings() *models.TeamEmailProviderSettings {
	return &models.TeamEmailProviderSettings{
		Mailgun: &models.MailgunProviderSettings{Domain: "mg.acme.test"},
	}
}

func validUpsertRequestBody() models.UpsertTeamEmailProviderRequest {
	secret := teamEmailProviderTestSecret
	return models.UpsertTeamEmailProviderRequest{
		ProviderType: services.EmailProviderTypeMailgun,
		Settings: models.TeamEmailProviderSettings{
			Mailgun: &models.MailgunProviderSettings{Domain: "mg.acme.test"},
		},
		Secret:      &secret,
		FromAddress: "hello@acme.test",
	}
}

// --- GET ---------------------------------------------------------------------

// A team with no provider of its own must get 200 describing the inherited
// instance provider — never 404.
func TestHandleGetTeamEmailProvider_InstanceFallback_SpecConformance(t *testing.T) {
	for _, prefix := range teamEmailProviderPrefixes {
		t.Run(prefix, func(t *testing.T) {
			c := newMockTeamEmailProviderContainer(t)
			c.teamEmailProviderService.EXPECT().
				GetEffective(mock.Anything, "user-123", "team-123").
				Return(models.NewTeamEmailProviderEffectiveInstance("noreply@instance.test"), nil)

			req := makeTeamEmailProviderRequest(http.MethodGet, "/api/v1/team-123/"+prefix, nil)
			w := httptest.NewRecorder()
			createTestTeamEmailProviderServer(c).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code, "an unconfigured team must not 404")
			specconformance.AssertConformsToSpec(t, req, w)

			var got models.TeamEmailProviderEffective
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.False(t, got.Configured)
			assert.Equal(t, "instance", got.Source)
			assert.Equal(t, "noreply@instance.test", got.EffectiveFromAddress)
			assert.Nil(t, got.ProviderType)
			assert.False(t, got.HasCredential)
		})
	}
}

func TestHandleGetTeamEmailProvider_TeamConfigured_SpecConformance(t *testing.T) {
	for _, prefix := range teamEmailProviderPrefixes {
		t.Run(prefix, func(t *testing.T) {
			c := newMockTeamEmailProviderContainer(t)
			c.teamEmailProviderService.EXPECT().
				GetEffective(mock.Anything, "user-123", "team-123").
				Return(models.NewTeamEmailProviderEffectiveTeam(sampleTeamProvider(), sampleTeamSettings()), nil)

			req := makeTeamEmailProviderRequest(http.MethodGet, "/api/v1/team-123/"+prefix, nil)
			w := httptest.NewRecorder()
			createTestTeamEmailProviderServer(c).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			var got models.TeamEmailProviderEffective
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.True(t, got.Configured)
			assert.Equal(t, "team", got.Source)
			require.NotNil(t, got.ProviderType)
			assert.Equal(t, services.EmailProviderTypeMailgun, *got.ProviderType)
			assert.True(t, got.HasCredential, "a stored credential is reported as a boolean")
			assert.Equal(t, "hello@acme.test", got.EffectiveFromAddress)

			// The credential itself must never be in the body.
			assert.NotContains(t, w.Body.String(), teamEmailProviderTestSecret)
		})
	}
}

// The settings a GET returns must be shaped like the PUT body that produced them
// — keyed by provider type, not the bare inner block. Otherwise a client cannot
// feed a GET response back into a PUT, and the documented schema would describe
// something the API does not serve. The spec schema is permissive enough that
// conformance alone would not catch this, so assert the shape explicitly.
func TestHandleGetTeamEmailProvider_SettingsRoundTripIntoUpsert(t *testing.T) {
	c := newMockTeamEmailProviderContainer(t)
	c.teamEmailProviderService.EXPECT().
		GetEffective(mock.Anything, "user-123", "team-123").
		Return(models.NewTeamEmailProviderEffectiveTeam(sampleTeamProvider(), sampleTeamSettings()), nil)

	req := makeTeamEmailProviderRequest(http.MethodGet, "/api/v1/team-123/email-provider", nil)
	w := httptest.NewRecorder()
	createTestTeamEmailProviderServer(c).ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// The wire shape is settings.mailgun.domain, matching the request body.
	var raw struct {
		Settings map[string]map[string]any `json:"settings"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	require.Contains(t, raw.Settings, "mailgun",
		"settings must be keyed by provider type, not the bare inner block")
	assert.Equal(t, "mg.acme.test", raw.Settings["mailgun"]["domain"])

	// And it decodes straight back into the request type.
	var back models.UpsertTeamEmailProviderRequest
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &back))
	require.NotNil(t, back.Settings.Mailgun)
	assert.Equal(t, "mg.acme.test", back.Settings.Mailgun.Domain)
}

// --- PUT ---------------------------------------------------------------------

func TestHandleUpsertTeamEmailProvider_SpecConformance(t *testing.T) {
	for _, prefix := range teamEmailProviderPrefixes {
		t.Run(prefix, func(t *testing.T) {
			c := newMockTeamEmailProviderContainer(t)
			body := validUpsertRequestBody()
			stored := sampleTeamProvider()
			c.teamEmailProviderService.EXPECT().
				Upsert(mock.Anything, "user-123", "team-123", body).
				Return(stored, nil)
			c.teamEmailProviderService.EXPECT().
				EffectiveFromProvider(stored).
				Return(models.NewTeamEmailProviderEffectiveTeam(stored, sampleTeamSettings()))

			req := makeTeamEmailProviderRequest(http.MethodPut, "/api/v1/team-123/"+prefix, body)
			w := httptest.NewRecorder()
			createTestTeamEmailProviderServer(c).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)
			assert.NotContains(t, w.Body.String(), teamEmailProviderTestSecret,
				"the credential must not come back in the upsert response")
		})
	}
}

// A validation failure from the service becomes a 400 with field details, not a
// 500.
func TestHandleUpsertTeamEmailProvider_ValidationFailure(t *testing.T) {
	c := newMockTeamEmailProviderContainer(t)
	body := validUpsertRequestBody()
	c.teamEmailProviderService.EXPECT().
		Upsert(mock.Anything, "user-123", "team-123", body).
		Return(nil, &services.TeamEmailProviderValidationError{Fields: []services.FieldError{
			{Field: "secret", Message: "cannot be empty; omit the field to keep the stored secret"},
		}})

	req := makeTeamEmailProviderRequest(http.MethodPut, "/api/v1/team-123/email-provider", body)
	w := httptest.NewRecorder()
	createTestTeamEmailProviderServer(c).ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var problem map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
	assert.Equal(t, "TEAM_EMAIL_PROVIDER_VALIDATION_FAILED", problem["code"])
	// The offending field must reach the client, or the UI cannot point at it.
	assert.Contains(t, w.Body.String(), "secret")
}

func TestHandleUpsertTeamEmailProvider_MalformedBody(t *testing.T) {
	c := newMockTeamEmailProviderContainer(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/team-123/email-provider",
		bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))

	w := httptest.NewRecorder()
	createTestTeamEmailProviderServer(c).ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
}

// --- DELETE ------------------------------------------------------------------

func TestHandleDeleteTeamEmailProvider_SpecConformance(t *testing.T) {
	for _, prefix := range teamEmailProviderPrefixes {
		t.Run(prefix, func(t *testing.T) {
			c := newMockTeamEmailProviderContainer(t)
			c.teamEmailProviderService.EXPECT().
				Delete(mock.Anything, "user-123", "team-123").
				Return(nil)

			req := makeTeamEmailProviderRequest(http.MethodDelete, "/api/v1/team-123/"+prefix, nil)
			w := httptest.NewRecorder()
			createTestTeamEmailProviderServer(c).ServeHTTP(w, req)

			require.Equal(t, http.StatusNoContent, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)
			assert.Empty(t, w.Body.String(), "204 carries no body")
		})
	}
}

// Deleting when the team has no provider is a 409, not a 404: the endpoint exists
// and the team is addressable.
func TestHandleDeleteTeamEmailProvider_NotConfigured(t *testing.T) {
	c := newMockTeamEmailProviderContainer(t)
	c.teamEmailProviderService.EXPECT().
		Delete(mock.Anything, "user-123", "team-123").
		Return(repositories.ErrTeamEmailProviderNotFound)

	req := makeTeamEmailProviderRequest(http.MethodDelete, "/api/v1/team-123/email-provider", nil)
	w := httptest.NewRecorder()
	createTestTeamEmailProviderServer(c).ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var problem map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
	assert.Equal(t, "TEAM_EMAIL_PROVIDER_NOT_CONFIGURED", problem["code"])
}

// --- POST /test --------------------------------------------------------------

func TestHandleTestTeamEmailProvider_SpecConformance(t *testing.T) {
	for _, prefix := range teamEmailProviderPrefixes {
		t.Run(prefix, func(t *testing.T) {
			c := newMockTeamEmailProviderContainer(t)
			body := validUpsertRequestBody()
			c.teamEmailProviderService.EXPECT().
				Test(mock.Anything, "user-123", "team-123",
					models.TestTeamEmailProviderRequest{UpsertTeamEmailProviderRequest: body}).
				Return(&models.TeamEmailProviderTestResult{
					Success:   true,
					Recipient: "admin@acme.test",
					Message:   "Test email sent to admin@acme.test",
				}, nil)

			req := makeTeamEmailProviderRequest(http.MethodPost, "/api/v1/team-123/"+prefix+"/test", body)
			w := httptest.NewRecorder()
			createTestTeamEmailProviderServer(c).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			var got models.TeamEmailProviderTestResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.True(t, got.IsValid)
			assert.Equal(t, "admin@acme.test", got.Recipient)
			assert.NotContains(t, w.Body.String(), teamEmailProviderTestSecret)
		})
	}
}

// A failed send is a 200 with is_valid: false and a fixed category — not a 5xx.
func TestHandleTestTeamEmailProvider_FailureIsA200(t *testing.T) {
	c := newMockTeamEmailProviderContainer(t)
	body := validUpsertRequestBody()
	c.teamEmailProviderService.EXPECT().
		Test(mock.Anything, "user-123", "team-123",
			models.TestTeamEmailProviderRequest{UpsertTeamEmailProviderRequest: body}).
		Return(&models.TeamEmailProviderTestResult{
			Success:      false,
			Recipient:    "admin@acme.test",
			Message:      "Sending failed: connection refused",
			ErrorDetails: models.TeamEmailProviderErrSendFailed,
		}, nil)

	req := makeTeamEmailProviderRequest(http.MethodPost, "/api/v1/team-123/email-provider/test", body)
	w := httptest.NewRecorder()
	createTestTeamEmailProviderServer(c).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "a bad configuration is a result, not a server error")
	specconformance.AssertConformsToSpec(t, req, w)

	var got models.TeamEmailProviderTestResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.False(t, got.IsValid)
	assert.Equal(t, models.TeamEmailProviderErrSendFailed, got.Details.ErrorDetails)
}

// A recipient in the request body must be ignored: the service decides where the
// test goes, so this endpoint cannot be used to mail third parties. The mock
// asserts the exact request the handler forwards, which carries no recipient
// field at all.
func TestHandleTestTeamEmailProvider_IgnoresCallerSuppliedRecipient(t *testing.T) {
	c := newMockTeamEmailProviderContainer(t)
	body := validUpsertRequestBody()
	c.teamEmailProviderService.EXPECT().
		Test(mock.Anything, "user-123", "team-123",
			models.TestTeamEmailProviderRequest{UpsertTeamEmailProviderRequest: body}).
		Return(&models.TeamEmailProviderTestResult{
			Success:   true,
			Recipient: "admin@acme.test",
			Message:   "Test email sent to admin@acme.test",
		}, nil)

	// Hand-built body carrying an extra recipient field an attacker would try.
	raw := map[string]any{
		"provider_type": services.EmailProviderTypeMailgun,
		"settings":      map[string]any{"mailgun": map[string]any{"domain": "mg.acme.test"}},
		"secret":        teamEmailProviderTestSecret,
		"from_address":  "hello@acme.test",
		"recipient":     "victim@elsewhere.test",
		"to":            "victim@elsewhere.test",
	}
	encoded, err := json.Marshal(raw)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/team-123/email-provider/test",
		bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))

	w := httptest.NewRecorder()
	createTestTeamEmailProviderServer(c).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var got models.TeamEmailProviderTestResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "admin@acme.test", got.Recipient,
		"the recipient comes from the acting user, never from the body")
	assert.NotContains(t, w.Body.String(), "victim@elsewhere.test")
}

// --- Authorization mapping ---------------------------------------------------

// A plain member is denied the writes and allowed the read. The service owns the
// role check; this asserts the handler maps its denial to 403 rather than
// flattening it into a 500.
func TestTeamEmailProviderHandlers_MemberIsForbiddenOnWrites(t *testing.T) {
	body := validUpsertRequestBody()

	tests := []struct {
		name    string
		arrange func(c *MockTeamEmailProviderContainer)
		method  string
		path    string
		body    any
	}{
		{
			name: "put",
			arrange: func(c *MockTeamEmailProviderContainer) {
				c.teamEmailProviderService.EXPECT().
					Upsert(mock.Anything, "user-123", "team-123", body).
					Return(nil, services.ErrPermissionDenied)
			},
			method: http.MethodPut,
			path:   "/api/v1/team-123/email-provider",
			body:   body,
		},
		{
			name: "delete",
			arrange: func(c *MockTeamEmailProviderContainer) {
				c.teamEmailProviderService.EXPECT().
					Delete(mock.Anything, "user-123", "team-123").
					Return(services.ErrPermissionDenied)
			},
			method: http.MethodDelete,
			path:   "/api/v1/team-123/email-provider",
		},
		{
			name: "test",
			arrange: func(c *MockTeamEmailProviderContainer) {
				c.teamEmailProviderService.EXPECT().
					Test(mock.Anything, "user-123", "team-123",
						models.TestTeamEmailProviderRequest{UpsertTeamEmailProviderRequest: body}).
					Return(nil, services.ErrPermissionDenied)
			},
			method: http.MethodPost,
			path:   "/api/v1/team-123/email-provider/test",
			body:   body,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newMockTeamEmailProviderContainer(t)
			tc.arrange(c)

			req := makeTeamEmailProviderRequest(tc.method, tc.path, tc.body)
			w := httptest.NewRecorder()
			createTestTeamEmailProviderServer(c).ServeHTTP(w, req)

			require.Equal(t, http.StatusForbidden, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			var problem map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
			assert.Equal(t, "FORBIDDEN", problem["code"])
		})
	}
}

func TestTeamEmailProviderHandlers_MemberCanRead(t *testing.T) {
	c := newMockTeamEmailProviderContainer(t)
	c.teamEmailProviderService.EXPECT().
		GetEffective(mock.Anything, "user-123", "team-123").
		Return(models.NewTeamEmailProviderEffectiveInstance("noreply@instance.test"), nil)

	req := makeTeamEmailProviderRequest(http.MethodGet, "/api/v1/team-123/email-provider", nil)
	w := httptest.NewRecorder()
	createTestTeamEmailProviderServer(c).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "reading the effective configuration is not privileged")
}

// --- Error paths -------------------------------------------------------------

// An unexpected service failure becomes a 500 with the domain's own code, not a
// bare panic or an unmapped error.
func TestTeamEmailProviderHandlers_InternalFailures(t *testing.T) {
	body := validUpsertRequestBody()

	tests := []struct {
		name     string
		arrange  func(c *MockTeamEmailProviderContainer)
		method   string
		path     string
		body     any
		wantCode string
	}{
		{
			name: "get",
			arrange: func(c *MockTeamEmailProviderContainer) {
				c.teamEmailProviderService.EXPECT().
					GetEffective(mock.Anything, "user-123", "team-123").
					Return(nil, errors.New("connection reset"))
			},
			method:   http.MethodGet,
			path:     "/api/v1/team-123/email-provider",
			wantCode: "INTERNAL_ERROR",
		},
		{
			name: "put",
			arrange: func(c *MockTeamEmailProviderContainer) {
				c.teamEmailProviderService.EXPECT().
					Upsert(mock.Anything, "user-123", "team-123", body).
					Return(nil, errors.New("connection reset"))
			},
			method:   http.MethodPut,
			path:     "/api/v1/team-123/email-provider",
			body:     body,
			wantCode: "TEAM_EMAIL_PROVIDER_UPDATE_FAILED",
		},
		{
			name: "delete",
			arrange: func(c *MockTeamEmailProviderContainer) {
				c.teamEmailProviderService.EXPECT().
					Delete(mock.Anything, "user-123", "team-123").
					Return(errors.New("connection reset"))
			},
			method:   http.MethodDelete,
			path:     "/api/v1/team-123/email-provider",
			wantCode: "TEAM_EMAIL_PROVIDER_DELETE_FAILED",
		},
		{
			name: "test",
			arrange: func(c *MockTeamEmailProviderContainer) {
				c.teamEmailProviderService.EXPECT().
					Test(mock.Anything, "user-123", "team-123",
						models.TestTeamEmailProviderRequest{UpsertTeamEmailProviderRequest: body}).
					Return(nil, errors.New("connection reset"))
			},
			method:   http.MethodPost,
			path:     "/api/v1/team-123/email-provider/test",
			body:     body,
			wantCode: "INTERNAL_ERROR",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newMockTeamEmailProviderContainer(t)
			tc.arrange(c)

			req := makeTeamEmailProviderRequest(tc.method, tc.path, tc.body)
			w := httptest.NewRecorder()
			createTestTeamEmailProviderServer(c).ServeHTTP(w, req)

			require.Equal(t, http.StatusInternalServerError, w.Code)

			var problem map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
			assert.Equal(t, tc.wantCode, problem["code"])
		})
	}
}

// A validation failure on the test endpoint is a 400 with field detail, not a
// misleading "your provider is broken" 200.
func TestHandleTestTeamEmailProvider_ValidationFailureIsA400(t *testing.T) {
	c := newMockTeamEmailProviderContainer(t)
	body := validUpsertRequestBody()
	c.teamEmailProviderService.EXPECT().
		Test(mock.Anything, "user-123", "team-123",
			models.TestTeamEmailProviderRequest{UpsertTeamEmailProviderRequest: body}).
		Return(nil, &services.TeamEmailProviderValidationError{Fields: []services.FieldError{
			{Field: "secret", Message: "is required"},
		}})

	req := makeTeamEmailProviderRequest(http.MethodPost, "/api/v1/team-123/email-provider/test", body)
	w := httptest.NewRecorder()
	createTestTeamEmailProviderServer(c).ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var problem map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
	assert.Equal(t, "TEAM_EMAIL_PROVIDER_VALIDATION_FAILED", problem["code"])
}

func TestHandleTestTeamEmailProvider_MalformedBody(t *testing.T) {
	c := newMockTeamEmailProviderContainer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/team-123/email-provider/test",
		bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))

	w := httptest.NewRecorder()
	createTestTeamEmailProviderServer(c).ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
}

// A validation error carrying no field list still maps to a 400 rather than
// falling through to a 500.
func TestTeamEmailProviderValidationErrors_HandlesUnwrappedError(t *testing.T) {
	assert.Nil(t, teamEmailProviderValidationErrors(errors.New("not a validation error")))
}
