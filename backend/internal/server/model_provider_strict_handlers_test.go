package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// renderJSONMap marshals v and decodes it back as a generic map, so two values
// of different Go types can be compared by the bytes they actually put on the
// wire. Comparing rendered maps is what makes these tests mutation-resistant:
// a field-by-field assertion list only ever checks the fields someone
// remembered to list.
func renderJSONMap(t *testing.T, v any) map[string]interface{} {
	t.Helper()

	raw, err := json.Marshal(v)
	require.NoError(t, err)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

// Guards for the #837 conversion that specconformance.AssertConformsToSpec
// cannot provide. The validator checks a body against the schema; it does not
// notice two same-typed fields swapped in the converter, a field left at its
// zero value, or a required array serialized as `null` — all three are
// schema-valid. These tests close that gap.

// fullyPopulatedModelProvider is the fixture the converter tests need: every
// optional field set, and a team_id that actually parses, so no field can be
// dropped without a test noticing. The _SpecConformance fixtures deliberately
// leave team_id nil, which is exactly the shape that hides a lost field.
func fullyPopulatedModelProvider() models.ModelProviderResponse {
	provider := *sampleModelProvider()
	teamID := testModelProviderTeamID
	provider.TeamID = &teamID
	provider.Configuration = `{"temperature":0.7}`
	provider.Version = 7
	return models.ModelProviderResponse{ModelProvider: provider, HasAPIKey: true}
}

// TestToGenModelProviderResponse_RendersTheSameWireBody compares the rendered
// JSON of the domain read model against the generated type's, rather than
// listing fields — an assertion list can only check what someone remembered,
// and the conversion's whole review criterion is a byte-identical body.
//
// Mutation-checked: nulling any single field in toGenModelProviderResponse
// fails this test.
func TestToGenModelProviderResponse_RendersTheSameWireBody(t *testing.T) {
	src := fullyPopulatedModelProvider()

	want := renderJSONMap(t, src)
	got := renderJSONMap(t, toGenModelProviderResponse(src))

	assert.Equal(t, want, got)
}

// TestToGenModelProviderResponse_NeverCarriesKeyMaterial pins the property the
// generated type buys us: the encrypted key has no field to land in.
func TestToGenModelProviderResponse_NeverCarriesKeyMaterial(t *testing.T) {
	src := fullyPopulatedModelProvider()
	secret := "super-secret-ciphertext"
	src.APIKeyEncrypted = &secret

	body, err := json.Marshal(toGenModelProviderResponse(src))
	require.NoError(t, err)

	assert.NotContains(t, string(body), secret)
	// The JSON KEY, not the substring — "has_api_key" legitimately contains it.
	assert.NotContains(t, string(body), `"api_key"`)
	assert.NotContains(t, string(body), `"api_key_encrypted"`)
	assert.Contains(t, string(body), `"has_api_key":true`)
}

// TestToGenModelProviderResponse_DropsAnUnparseableTeamID documents the one
// deliberate divergence: team_id is a uuid in the spec but an opaque *string on
// the row, so a value that cannot parse is omitted rather than emitted invalid.
func TestToGenModelProviderResponse_DropsAnUnparseableTeamID(t *testing.T) {
	src := fullyPopulatedModelProvider()
	notAUUID := "team-123"
	src.TeamID = &notAUUID

	assert.Nil(t, toGenModelProviderResponse(src).TeamId)
}

// TestToGenValidateModelProviderResponse_OmitsZeroDetailFields pins the probe
// payload: `details` is always present (omitempty does nothing on a struct, so
// the hand-marshaled path always emitted it) while each field inside it is
// omitted when zero.
func TestToGenValidateModelProviderResponse_OmitsZeroDetailFields(t *testing.T) {
	src := &models.ValidateModelProviderResponse{
		IsValid: false,
		Message: "Authentication failed",
		Details: models.ValidateModelProviderDetails{StatusCode: 401},
	}

	want := renderJSONMap(t, src)
	got := renderJSONMap(t, toGenValidateModelProviderResponse(src))

	assert.Equal(t, want, got)
	// Spelled out, because the map comparison would also pass if BOTH sides
	// dropped the object: details is present, its zero members are not.
	details, ok := got["details"].(map[string]interface{})
	require.True(t, ok, "details must be an object, got %#v", got["details"])
	assert.Equal(t, float64(401), details["status_code"])
	assert.NotContains(t, details, "response_time_ms")
	assert.NotContains(t, details, "error_details")
}

// TestHandleListModelProviders_EmptyListIsArrayNotNull pins the #125 rule at
// the wire. AssertConformsToSpec passes on `null` here, so only a literal check
// of the body bites — and the generated response type cannot use the
// models.JSONArray[T] shim that protects hand-marshaled paths.
func TestHandleListModelProviders_EmptyListIsArrayNotNull(t *testing.T) {
	for _, prefix := range []string{"model-providers", "settings/model-providers"} {
		t.Run(prefix, func(t *testing.T) {
			mockContainer := newMockModelProviderContainer(t)
			// A nil slice from the service is the case that would marshal `null`.
			mockContainer.modelProviderService.
				On("GetModelProvidersByTeamID", mock.Anything, testModelProviderTeamID).
				Return(([]models.ModelProviderResponse)(nil), nil)

			srv := createTestModelProviderServer(mockContainer)
			req := makeAuthenticatedModelProviderRequest(
				"GET", "/api/v1/"+testModelProviderTeamID+"/"+prefix, nil, "user-123",
			)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			assert.JSONEq(t, "[]", w.Body.String())
			assert.NotContains(t, w.Body.String(), "null")

			mockContainer.modelProviderService.AssertExpectations(t)
		})
	}
}

// TestModelProviderRoutes_MalformedTeamIDIsBadRequest pins what the generated
// binder added: team_id is `format: uuid`, so a non-uuid is now rejected before
// the handler with this domain's RFC 9457 body rather than reaching the service.
func TestModelProviderRoutes_MalformedTeamIDIsBadRequest(t *testing.T) {
	mockContainer := newMockModelProviderContainer(t)
	srv := createTestModelProviderServer(mockContainer)

	req := makeAuthenticatedModelProviderRequest(
		"GET", "/api/v1/not-a-uuid/model-providers", nil, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "team_id must be a valid UUID")
}

// TestHandleModelProvider_MalformedBodyIsBadRequest pins that the binder's
// decode failure keeps the message the hand-written decoder used, so a client
// matching on it is unaffected by the conversion.
func TestHandleModelProvider_MalformedBodyIsBadRequest(t *testing.T) {
	for _, tc := range []struct{ name, method, path string }{
		{"create", http.MethodPost, "/model-providers"},
		{"update", http.MethodPut, "/model-providers/provider-1"},
		{"validate", http.MethodPost, "/model-providers/validate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockContainer := newMockModelProviderContainer(t)
			srv := createTestModelProviderServer(mockContainer)

			req := httptest.NewRequest(
				tc.method,
				"/api/v1/"+testModelProviderTeamID+tc.path,
				bytes.NewReader([]byte(`{"name":`)),
			)
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), "BAD_REQUEST")
			// The pre-conversion message, not the generated decoder's wording.
			assert.Contains(t, w.Body.String(), msgInvalidBodyWellFormedJSON)
		})
	}
}
