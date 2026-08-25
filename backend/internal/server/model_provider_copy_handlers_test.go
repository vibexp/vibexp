package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	testCopyDestTeamID     = "660e8400-e29b-41d4-a716-446655440001"
	testCopySourceTeamID   = "770e8400-e29b-41d4-a716-446655440002"
	testCopySourceProvider = "550e8400-e29b-41d4-a716-446655440000"
	testCopyUserID         = "user-123"
	testCopyPath           = "/api/v1/" + testCopyDestTeamID + "/settings/model-providers/copy"
)

// createTestModelProviderCopyServer mounts the generated copy route the way
// production does, minus the tenancy middleware (the user id is injected on the
// request context instead).
func createTestModelProviderCopyServer(container *MockModelProviderContainer) *Server {
	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: container,
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
		router:    r,
	}
	srv.mountModelProvidersCopyHandler(r)
	return srv
}

func makeCopyRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, testCopyPath, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testCopyUserID))
}

func copyRequestBody() string {
	return fmt.Sprintf(
		`{"source_team_id":%q,"source_provider_id":%q}`,
		testCopySourceTeamID, testCopySourceProvider,
	)
}

// copiedProviderRow is what the service hands back: the DESTINATION's row,
// carrying the source's ciphertext.
func copiedProviderRow() *models.ModelProvider {
	provider := sampleModelProvider()
	teamID := testCopyDestTeamID
	provider.ID = "dst-provider-1"
	provider.TeamID = &teamID
	provider.IsDefault = false
	return provider
}

func assertCopyProblem(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	assert.Equal(t, status, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), fmt.Sprintf(`"code":%q`, code))
}

func TestCopyModelProviderFromTeam_SpecConformance(t *testing.T) {
	container := newMockModelProviderContainer(t)
	container.modelProviderService.EXPECT().
		CopyFromTeam(mock.Anything, services.CopyModelProviderParams{
			TeamID:           testCopyDestTeamID,
			SourceTeamID:     testCopySourceTeamID,
			SourceProviderID: testCopySourceProvider,
			UserID:           testCopyUserID,
		}).Return(copiedProviderRow(), nil)

	srv := createTestModelProviderCopyServer(container)
	req := makeCopyRequest(copyRequestBody())
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	body := w.Body.String()
	assert.Contains(t, body, `"has_api_key":true`)
	assert.Contains(t, body, `"is_default":false`)
	// The whole reason this endpoint exists server-side: the key never travels.
	assert.NotContains(t, body, "api_key_encrypted")
	assert.NotContains(t, body, "encrypted-key")
}

// TestCopyModelProviderFromTeam_ResponseNeverCarriesKeyMaterial is the wire-level
// guard the generated type already gives by construction — there is no field for
// a key to land in. It is asserted anyway because "no field exists" is exactly
// the property a future hand-marshaling regression would quietly remove.
func TestCopyModelProviderFromTeam_ResponseNeverCarriesKeyMaterial(t *testing.T) {
	secret := "s3cr3t-ciphertext-that-must-never-ship"
	provider := copiedProviderRow()
	provider.APIKeyEncrypted = &secret

	container := newMockModelProviderContainer(t)
	container.modelProviderService.EXPECT().
		CopyFromTeam(mock.Anything, mock.Anything).Return(provider, nil)

	srv := createTestModelProviderCopyServer(container)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, makeCopyRequest(copyRequestBody()))

	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), secret)
	// Nor its length, which would narrow a brute-force search.
	assert.NotContains(t, w.Body.String(), fmt.Sprintf("%d", len(secret)))
	assert.Contains(t, w.Body.String(), `"has_api_key":true`)
}

func TestCopyModelProviderFromTeam_NoKeyReportsHasAPIKeyFalse(t *testing.T) {
	provider := copiedProviderRow()
	provider.APIKeyEncrypted = nil

	container := newMockModelProviderContainer(t)
	container.modelProviderService.EXPECT().
		CopyFromTeam(mock.Anything, mock.Anything).Return(provider, nil)

	srv := createTestModelProviderCopyServer(container)
	req := makeCopyRequest(copyRequestBody())
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
	assert.Contains(t, w.Body.String(), `"has_api_key":false`)
}

func TestCopyModelProviderFromTeam_ForbiddenIsIdenticalForEitherTeam(t *testing.T) {
	bodies := make([]string, 0, 2)

	for _, deniedTeam := range []string{testCopyDestTeamID, testCopySourceTeamID} {
		container := newMockModelProviderContainer(t)
		container.modelProviderService.EXPECT().
			CopyFromTeam(mock.Anything, mock.Anything).
			Return(nil, errors.Join(services.ErrPermissionDenied, errors.New("denied on "+deniedTeam)))

		srv := createTestModelProviderCopyServer(container)
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, makeCopyRequest(copyRequestBody()))

		assertCopyProblem(t, w, http.StatusForbidden, "FORBIDDEN")
		assert.NotContains(t, w.Body.String(), testCopySourceTeamID,
			"a 403 must never reveal the source team")
		bodies = append(bodies, w.Body.String())
	}

	// Byte-identical apart from the per-request id/timestamp, so a caller
	// cannot tell which team refused them.
	require.Len(t, bodies, 2)
	assert.Equal(t, problemShape(bodies[0]), problemShape(bodies[1]),
		"the two denials must be indistinguishable")
}

// problemShape strips the fields that legitimately vary per request, leaving
// what a caller could use to tell two denials apart.
func problemShape(body string) map[string]any {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		return map[string]any{"unparsable": body}
	}
	// request_id and timestamp legitimately vary per request. `instance` does
	// NOT: it is the request path, identical for both calls here, so it stays
	// in the comparison rather than being excused.
	delete(decoded, "request_id")
	delete(decoded, "timestamp")
	return decoded
}

func TestCopyModelProviderFromTeam_BadSourceIsBadRequest(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"missing source", services.ErrCopySourceRequired},
		{"source is destination", services.ErrCopySourceIsDestination},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			container := newMockModelProviderContainer(t)
			container.modelProviderService.EXPECT().
				CopyFromTeam(mock.Anything, mock.Anything).Return(nil, tc.err)

			srv := createTestModelProviderCopyServer(container)
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, makeCopyRequest(copyRequestBody()))

			assertCopyProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
		})
	}
}

// TestCopyModelProviderFromTeam_ZeroUUIDsRejectedBeforeTheService pins the #829
// lesson: oapi-codegen enforces neither `required` nor additionalProperties on a
// request body, so an omitted uuid binds as uuid.Nil rather than failing. Left
// unguarded it reaches the service and answers 403 for a team that cannot exist.
// The container has NO expectations — mockery fails the test if the service is
// called at all.
func TestCopyModelProviderFromTeam_ZeroUUIDsRejectedBeforeTheService(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty object", `{}`},
		{
			"no source_team_id",
			fmt.Sprintf(`{"source_provider_id":%q}`, testCopySourceProvider),
		},
		{
			"no source_provider_id",
			fmt.Sprintf(`{"source_team_id":%q}`, testCopySourceTeamID),
		},
		{
			"zero uuids spelled out",
			`{"source_team_id":"00000000-0000-0000-0000-000000000000",` +
				`"source_provider_id":"00000000-0000-0000-0000-000000000000"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := createTestModelProviderCopyServer(newMockModelProviderContainer(t))
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, makeCopyRequest(tc.body))

			assertCopyProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
		})
	}
}

func TestCopyModelProviderFromTeam_MalformedBodyIsBadRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"not json", `{`},
		{
			"malformed source_team_id",
			fmt.Sprintf(`{"source_team_id":"not-a-uuid","source_provider_id":%q}`, testCopySourceProvider),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := createTestModelProviderCopyServer(newMockModelProviderContainer(t))
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, makeCopyRequest(tc.body))

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestCopyModelProviderFromTeam_EmptyOverrideIsAValidationError covers the
// handler-level check the service does not do, mirroring the create handler:
// an override that is SENT must be non-empty. Absent overrides are the normal
// case and must not trip it.
func TestCopyModelProviderFromTeam_EmptyOverrideIsAValidationError(t *testing.T) {
	for _, field := range []string{"name", "provider_type", "model"} {
		t.Run(field, func(t *testing.T) {
			body := fmt.Sprintf(
				`{"source_team_id":%q,"source_provider_id":%q,%q:""}`,
				testCopySourceTeamID, testCopySourceProvider, field,
			)

			srv := createTestModelProviderCopyServer(newMockModelProviderContainer(t))
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, makeCopyRequest(body))

			assertCopyProblem(t, w, http.StatusBadRequest, "MODEL_PROVIDER_VALIDATION_FAILED")
			assert.Contains(t, w.Body.String(), field)
		})
	}
}

func TestCopyModelProviderFromTeam_OverridesReachTheService(t *testing.T) {
	container := newMockModelProviderContainer(t)
	container.modelProviderService.EXPECT().
		CopyFromTeam(mock.Anything, mock.MatchedBy(func(p services.CopyModelProviderParams) bool {
			return p.Name != nil && *p.Name == "Staging" &&
				p.Model != nil && *p.Model == "gpt-4o" &&
				p.BaseURL != nil && *p.BaseURL == "https://example.test/v1" &&
				p.Configuration != nil
		})).Return(copiedProviderRow(), nil)

	body := fmt.Sprintf(
		`{"source_team_id":%q,"source_provider_id":%q,"name":"Staging","model":"gpt-4o",`+
			`"base_url":"https://example.test/v1","configuration":{"temperature":0.1}}`,
		testCopySourceTeamID, testCopySourceProvider,
	)

	srv := createTestModelProviderCopyServer(container)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, makeCopyRequest(body))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestCopyModelProviderFromTeam_UnknownSourceProviderIsNotFound(t *testing.T) {
	container := newMockModelProviderContainer(t)
	container.modelProviderService.EXPECT().
		CopyFromTeam(mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("%w: %s", services.ErrModelProviderNotFound, testCopySourceProvider))

	srv := createTestModelProviderCopyServer(container)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, makeCopyRequest(copyRequestBody()))

	assertCopyProblem(t, w, http.StatusNotFound, "MODEL_PROVIDER_NOT_FOUND")
	assert.NotContains(t, w.Body.String(), testCopySourceProvider,
		"the 404 detail must not echo an id from the source team")
}

func TestCopyModelProviderFromTeam_NameCollisionIsConflict(t *testing.T) {
	container := newMockModelProviderContainer(t)
	container.modelProviderService.EXPECT().
		CopyFromTeam(mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("%w: Taken", services.ErrModelProviderAlreadyExists))

	srv := createTestModelProviderCopyServer(container)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, makeCopyRequest(copyRequestBody()))

	assertCopyProblem(t, w, http.StatusConflict, "MODEL_PROVIDER_ALREADY_EXISTS")
}

func TestCopyModelProviderFromTeam_InternalErrorDoesNotLeakDetail(t *testing.T) {
	container := newMockModelProviderContainer(t)
	container.modelProviderService.EXPECT().
		CopyFromTeam(mock.Anything, mock.Anything).
		Return(nil, errors.New("audit storage down at 10.0.0.5:5432"))

	srv := createTestModelProviderCopyServer(container)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, makeCopyRequest(copyRequestBody()))

	assertCopyProblem(t, w, http.StatusInternalServerError, "INTERNAL_ERROR")
	assert.NotContains(t, w.Body.String(), "10.0.0.5")
}
