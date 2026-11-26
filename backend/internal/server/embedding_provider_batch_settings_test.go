package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// --- per-provider batch size and request timeout (#1665) ---

const batchSettingsTeamPath = "/api/v1/11111111-2222-4333-8444-555555555555"

// batchSettingsOutOfRange is every out-of-range value of the two settings, each
// paired with the field the 400 must name.
var batchSettingsOutOfRange = []struct {
	field string
	value int
}{
	{"batch_size", 0},
	{"batch_size", 33},
	{"request_timeout_seconds", 4},
	{"request_timeout_seconds", 301},
}

// assertNamesField asserts a PROVIDER_VALIDATION_FAILED 400 whose validation
// errors name exactly the given field.
func assertNamesField(t *testing.T, w *httptest.ResponseRecorder, field string) {
	t.Helper()
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var problem struct {
		Code             string `json:"code"`
		ValidationErrors []struct {
			Field string `json:"field"`
			Code  string `json:"code"`
		} `json:"validation_errors"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
	assert.Equal(t, "PROVIDER_VALIDATION_FAILED", problem.Code)
	require.Len(t, problem.ValidationErrors, 1, w.Body.String())
	assert.Equal(t, field, problem.ValidationErrors[0].Field)
	assert.Equal(t, "INVALID_RANGE", problem.ValidationErrors[0].Code)
}

func TestCreateEmbeddingProvider_RejectsOutOfRangeBatchSettings(t *testing.T) {
	for _, tc := range batchSettingsOutOfRange {
		t.Run(fmt.Sprintf("%s=%d", tc.field, tc.value), func(t *testing.T) {
			srv := createTestEmbeddingProviderServer(newMockEmbeddingProviderContainer(t))
			body := map[string]any{"name": "P", "provider_type": "openai_compatible", "model": "m", tc.field: tc.value}
			req := makeAuthenticatedEmbeddingProviderRequest(
				http.MethodPost, batchSettingsTeamPath+"/embedding-providers", body, "user-123",
			)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			assertNamesField(t, w, tc.field)
			specconformance.AssertConformsToSpec(t, req, w)
		})
	}
}

func TestUpdateEmbeddingProvider_RejectsOutOfRangeBatchSettings(t *testing.T) {
	for _, tc := range batchSettingsOutOfRange {
		t.Run(fmt.Sprintf("%s=%d", tc.field, tc.value), func(t *testing.T) {
			srv := createTestEmbeddingProviderServer(newMockEmbeddingProviderContainer(t))
			req := makeAuthenticatedEmbeddingProviderRequest(
				http.MethodPut, batchSettingsTeamPath+"/embedding-providers/provider-1",
				map[string]any{tc.field: tc.value}, "user-123",
			)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			assertNamesField(t, w, tc.field)
			specconformance.AssertConformsToSpec(t, req, w)
		})
	}
}

func TestValidateEmbeddingProvider_RejectsOutOfRangeBatchSettings(t *testing.T) {
	for _, tc := range batchSettingsOutOfRange {
		t.Run(fmt.Sprintf("%s=%d", tc.field, tc.value), func(t *testing.T) {
			srv := createTestEmbeddingProviderServer(newMockEmbeddingProviderContainer(t))
			body := map[string]any{
				"provider_type": "openai_compatible", "model": "m", "base_url": "https://api.example.com/v1",
				tc.field: tc.value,
			}
			req := makeAuthenticatedEmbeddingProviderRequest(
				http.MethodPost, batchSettingsTeamPath+"/embedding-providers/validate", body, "user-123",
			)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			assertNamesField(t, w, tc.field)
		})
	}
}

func TestCopyEmbeddingProviderFromTeam_RejectsOutOfRangeBatchSettings(t *testing.T) {
	for _, tc := range batchSettingsOutOfRange {
		t.Run(fmt.Sprintf("%s=%d", tc.field, tc.value), func(t *testing.T) {
			srv := createTestEmbeddingProviderServer(newMockEmbeddingProviderContainer(t))
			body := fmt.Sprintf(`{"source_team_id":%q,"source_provider_id":%q,%q:%d}`,
				testEmbedCopySourceTeamID, testEmbedCopySourceProvider, tc.field, tc.value)
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, makeEmbedCopyRequest(body))
			assertNamesField(t, w, tc.field)
		})
	}
}

// The bounds are accepted inclusively and reach the service unchanged.
func TestCreateEmbeddingProvider_PassesBatchSettingsThrough(t *testing.T) {
	for _, tc := range []struct{ batch, timeout int }{{1, 5}, {32, 300}} {
		t.Run(fmt.Sprintf("%d/%d", tc.batch, tc.timeout), func(t *testing.T) {
			container := newMockEmbeddingProviderContainer(t)
			var got models.CreateEmbeddingProviderRequest
			container.embeddingProviderService.On(
				"CreateEmbeddingProvider", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
			).Run(func(args mock.Arguments) {
				got = args.Get(3).(models.CreateEmbeddingProviderRequest)
			}).Return(func() *models.EmbeddingProvider {
				p := specConformanceProvider()
				p.BatchSize, p.RequestTimeoutSeconds = tc.batch, tc.timeout
				return p
			}(), nil)

			expectTeamReembed(t, container)
			srv := createTestEmbeddingProviderServer(container)
			body := map[string]any{
				"name": "P", "provider_type": "openai_compatible", "model": "m",
				"batch_size": tc.batch, "request_timeout_seconds": tc.timeout,
			}
			req := makeAuthenticatedEmbeddingProviderRequest(
				http.MethodPost, batchSettingsTeamPath+"/embedding-providers", body, "user-123",
			)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.NotNil(t, got.BatchSize)
			require.NotNil(t, got.RequestTimeoutSeconds)
			assert.Equal(t, tc.batch, *got.BatchSize)
			assert.Equal(t, tc.timeout, *got.RequestTimeoutSeconds)
			var resp map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.InDelta(t, tc.batch, resp["batch_size"], 0)
			assert.InDelta(t, tc.timeout, resp["request_timeout_seconds"], 0)
			specconformance.AssertConformsToSpec(t, req, w)
		})
	}
}

// The spec's minimum/maximum on every request schema agree with the bounds the
// handlers enforce, so a client reading the spec and the server cannot disagree.
func TestBatchSettingsSpecBoundsMatchModels(t *testing.T) {
	raw, err := os.ReadFile("../../schemas/embedding-providers.yaml")
	require.NoError(t, err)
	var schemas map[string]struct {
		Properties map[string]struct {
			Minimum *int `yaml:"minimum"`
			Maximum *int `yaml:"maximum"`
		} `yaml:"properties"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &schemas))

	want := map[string][2]int{
		"batch_size":              {models.EmbeddingBatchSizeMin, models.EmbeddingBatchSizeMax},
		"request_timeout_seconds": {models.EmbeddingRequestTimeoutSecondsMin, models.EmbeddingRequestTimeoutSecondsMax},
	}
	for _, schema := range []string{
		"EmbeddingProvider", "CreateEmbeddingProviderRequest", "UpdateEmbeddingProviderRequest",
		"ValidateEmbeddingProviderRequest", "CopyEmbeddingProviderRequest",
	} {
		for field, bounds := range want {
			prop, ok := schemas[schema].Properties[field]
			require.True(t, ok, "%s.%s is missing from the spec", schema, field)
			require.NotNil(t, prop.Minimum, "%s.%s has no minimum", schema, field)
			require.NotNil(t, prop.Maximum, "%s.%s has no maximum", schema, field)
			assert.Equal(t, bounds, [2]int{*prop.Minimum, *prop.Maximum}, "%s.%s", schema, field)
		}
	}
}
