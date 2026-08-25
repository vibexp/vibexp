package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	testEmbedCopyDestTeamID     = "660e8400-e29b-41d4-a716-446655440001"
	testEmbedCopySourceTeamID   = "770e8400-e29b-41d4-a716-446655440002"
	testEmbedCopySourceProvider = "550e8400-e29b-41d4-a716-446655440000"
	testEmbedCopyUserID         = "user-123"
	testEmbedCopyPath           = "/api/v1/" + testEmbedCopyDestTeamID +
		"/settings/embedding-providers/copy"
)

func makeEmbedCopyRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, testEmbedCopyPath, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testEmbedCopyUserID))
}

func embedCopyRequestBody() string {
	return fmt.Sprintf(
		`{"source_team_id":%q,"source_provider_id":%q}`,
		testEmbedCopySourceTeamID, testEmbedCopySourceProvider,
	)
}

// copiedEmbeddingProviderRow is what the service hands back: the DESTINATION's
// row, carrying the source's ciphertext and always non-default.
func copiedEmbeddingProviderRow() *models.EmbeddingProvider {
	teamID := testEmbedCopyDestTeamID
	baseURL := "https://api.openai.com/v1"
	queryPrefix := "query: "
	documentPrefix := "passage: "
	secret := "encrypted-key"
	now := time.Now()

	return &models.EmbeddingProvider{
		ID:              "dst-provider-1",
		UserID:          testEmbedCopyUserID,
		TeamID:          &teamID,
		Name:            "mxbai-embed-large",
		ProviderType:    "openai_compatible",
		Model:           "mxbai-embed-large",
		ChunkSize:       512,
		ChunkOverlap:    64,
		Concurrency:     4,
		QueryPrefix:     &queryPrefix,
		DocumentPrefix:  &documentPrefix,
		IsDefault:       false,
		BaseURL:         &baseURL,
		APIKeyEncrypted: &secret,
		Configuration:   "{}",
		CreatedAt:       now,
		UpdatedAt:       now,
		Version:         1,
	}
}

func embedCopyResult(activation services.CopyEmbeddingProviderActivation) *services.CopyEmbeddingProviderResult {
	return &services.CopyEmbeddingProviderResult{
		Provider:   copiedEmbeddingProviderRow(),
		Activation: activation,
	}
}

func assertEmbedCopyProblem(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	assert.Equal(t, status, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var problem map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
	assert.Equal(t, code, problem["code"])
}

// TestCopyEmbeddingProviderFromTeam_Success is the happy path, and the response
// body is validated against openapi.yaml so the ledger entry for this operation
// is earned rather than asserted by hand.
func TestCopyEmbeddingProviderFromTeam_Success(t *testing.T) {
	container := newMockEmbeddingProviderContainer(t)
	container.embeddingProviderService.EXPECT().
		CopyFromTeam(mock.Anything, mock.MatchedBy(func(p services.CopyEmbeddingProviderParams) bool {
			return p.TeamID == testEmbedCopyDestTeamID &&
				p.SourceTeamID == testEmbedCopySourceTeamID &&
				p.SourceProviderID == testEmbedCopySourceProvider &&
				p.UserID == testEmbedCopyUserID
		})).
		Return(embedCopyResult(services.CopyEmbeddingProviderActivation{}), nil)

	srv := createTestEmbeddingProviderServer(container)
	w := httptest.NewRecorder()
	req := makeEmbedCopyRequest(embedCopyRequestBody())
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

// TestCopyEmbeddingProviderFromTeam_CarriesSourceTuningToTheResponse pins that
// chunk sizing, concurrency and both instruction prefixes survive the copy onto
// the wire — the tuning that makes the copy behave like the provider it came
// from rather than like a freshly created one on create-path defaults.
func TestCopyEmbeddingProviderFromTeam_CarriesSourceTuningToTheResponse(t *testing.T) {
	container := newMockEmbeddingProviderContainer(t)
	container.embeddingProviderService.EXPECT().
		CopyFromTeam(mock.Anything, mock.Anything).
		Return(embedCopyResult(services.CopyEmbeddingProviderActivation{}), nil)

	srv := createTestEmbeddingProviderServer(container)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, makeEmbedCopyRequest(embedCopyRequestBody()))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var payload struct {
		Provider struct {
			ChunkSize      int     `json:"chunk_size"`
			ChunkOverlap   int     `json:"chunk_overlap"`
			Concurrency    int     `json:"concurrency"`
			QueryPrefix    *string `json:"query_prefix"`
			DocumentPrefix *string `json:"document_prefix"`
		} `json:"provider"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))

	assert.Equal(t, 512, payload.Provider.ChunkSize)
	assert.Equal(t, 64, payload.Provider.ChunkOverlap)
	assert.Equal(t, 4, payload.Provider.Concurrency)
	require.NotNil(t, payload.Provider.QueryPrefix)
	assert.Equal(t, "query: ", *payload.Provider.QueryPrefix)
	require.NotNil(t, payload.Provider.DocumentPrefix)
	assert.Equal(t, "passage: ", *payload.Provider.DocumentPrefix)
}

// TestCopyEmbeddingProviderFromTeam_ReportsSilentActivation is THE test for the
// trap this endpoint exists to expose: the destination team already has
// providers but NO default, so the copy — written is_default:false, as every
// copy is — silently becomes the provider every future embedding is generated
// with, displacing a different model with no error anywhere in the system.
//
// A response that reported only the provider row would show `is_default:false`
// and look inert.
func TestCopyEmbeddingProviderFromTeam_ReportsSilentActivation(t *testing.T) {
	displaced := "text-embedding-3-small"

	container := newMockEmbeddingProviderContainer(t)
	container.embeddingProviderService.EXPECT().
		CopyFromTeam(mock.Anything, mock.Anything).
		Return(embedCopyResult(services.CopyEmbeddingProviderActivation{
			BecomesActive:              true,
			DisplacedModel:             &displaced,
			DisplacedEmbeddedResources: 412,
			ModelChanged:               true,
		}), nil)

	srv := createTestEmbeddingProviderServer(container)
	w := httptest.NewRecorder()
	req := makeEmbedCopyRequest(embedCopyRequestBody())
	srv.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	var payload struct {
		Provider struct {
			IsDefault bool `json:"is_default"`
		} `json:"provider"`
		Activation struct {
			BecomesActive              bool    `json:"becomes_active"`
			DisplacedModel             *string `json:"displaced_model"`
			DisplacedEmbeddedResources int64   `json:"displaced_embedded_resources"`
			ReprocessEnqueued          bool    `json:"reprocess_enqueued"`
			EmbeddingsWiped            bool    `json:"embeddings_wiped"`
		} `json:"activation"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))

	assert.False(t, payload.Provider.IsDefault, "a copy is always written non-default")
	assert.True(t, payload.Activation.BecomesActive, "…which does NOT make it inert")
	require.NotNil(t, payload.Activation.DisplacedModel)
	assert.Equal(t, displaced, *payload.Activation.DisplacedModel)
	assert.Equal(t, int64(412), payload.Activation.DisplacedEmbeddedResources)
	// reprocess was not requested, so nothing was enqueued and nothing wiped.
	assert.False(t, payload.Activation.ReprocessEnqueued)
	assert.False(t, payload.Activation.EmbeddingsWiped)
}

// TestCopyEmbeddingProviderFromTeam_NoActivationReportsNulls covers the other
// side: the destination has a default of its own, so the copy changes nothing
// about search. displaced_model must be present-and-null (a required nullable
// field), not absent.
func TestCopyEmbeddingProviderFromTeam_NoActivationReportsNulls(t *testing.T) {
	container := newMockEmbeddingProviderContainer(t)
	container.embeddingProviderService.EXPECT().
		CopyFromTeam(mock.Anything, mock.Anything).
		Return(embedCopyResult(services.CopyEmbeddingProviderActivation{BecomesActive: false}), nil)

	srv := createTestEmbeddingProviderServer(container)
	w := httptest.NewRecorder()
	req := makeEmbedCopyRequest(embedCopyRequestBody())
	srv.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Contains(t, w.Body.String(), `"displaced_model":null`,
		"a required nullable field must be present as null, never omitted")
	assert.Contains(t, w.Body.String(), `"becomes_active":false`)
	assert.Contains(t, w.Body.String(), `"displaced_embedded_resources":0`)
}

// TestCopyEmbeddingProviderFromTeam_ReprocessWipesOnlyWhenTheModelMoved is the
// guard on the one destructive act in this epic. Deleting a team's vectors is
// warranted when the copy took over with a DIFFERENT model (the stored vectors
// are no longer comparable to new queries) and never otherwise.
func TestCopyEmbeddingProviderFromTeam_ReprocessWipesOnlyWhenTheModelMoved(t *testing.T) {
	displaced := "text-embedding-3-small"
	sameModel := copiedEmbeddingProviderRow().Model

	tests := []struct {
		name       string
		activation services.CopyEmbeddingProviderActivation
		reprocess  bool
		wantQueued bool
		wantWiped  bool
	}{
		{
			name: "active with a different model wipes",
			activation: services.CopyEmbeddingProviderActivation{
				BecomesActive: true, DisplacedModel: &displaced, ModelChanged: true,
			},
			reprocess: true, wantQueued: true, wantWiped: true,
		},
		{
			name: "active with the same model fills gaps only",
			activation: services.CopyEmbeddingProviderActivation{
				BecomesActive: true, DisplacedModel: &sameModel, ModelChanged: false,
			},
			reprocess: true, wantQueued: true, wantWiped: false,
		},
		{
			name:       "not active fills gaps only",
			activation: services.CopyEmbeddingProviderActivation{BecomesActive: false},
			reprocess:  true, wantQueued: true, wantWiped: false,
		},
		{
			name: "reprocess omitted enqueues nothing at all",
			activation: services.CopyEmbeddingProviderActivation{
				BecomesActive: true, DisplacedModel: &displaced, ModelChanged: true,
			},
			reprocess: false, wantQueued: false, wantWiped: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			container := newMockEmbeddingProviderContainer(t)
			container.embeddingProviderService.EXPECT().
				CopyFromTeam(mock.Anything, mock.Anything).
				Return(embedCopyResult(tc.activation), nil)

			// The wipe is observable as exactly one repository call. Declaring it
			// only for the case that should wipe means mockery fails the test if a
			// case that must not wipe reaches it.
			wiped := make(chan struct{}, 1)
			if tc.wantWiped {
				container.embeddingRepository.EXPECT().
					DeleteByTeam(mock.Anything, testEmbedCopyDestTeamID).
					RunAndReturn(func(context.Context, string) (int64, error) {
						wiped <- struct{}{}
						return 7, nil
					})
			}
			// The regeneration itself runs on a goroutine the handler does not
			// join, so it is allowed rather than required.
			container.embeddingBackfillService.EXPECT().
				Backfill(mock.Anything, mock.Anything).
				Return(&services.EmbeddingBackfillResult{}, nil).Maybe()

			body := embedCopyRequestBody()
			if tc.reprocess {
				body = strings.TrimSuffix(body, "}") + `,"reprocess":true}`
			}

			srv := createTestEmbeddingProviderServer(container)
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, makeEmbedCopyRequest(body))

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(),
				fmt.Sprintf(`"reprocess_enqueued":%t`, tc.wantQueued))
			assert.Contains(t, w.Body.String(),
				fmt.Sprintf(`"embeddings_wiped":%t`, tc.wantWiped))

			if tc.wantWiped {
				select {
				case <-wiped:
				case <-time.After(2 * time.Second):
					t.Fatal("expected the team's embeddings to be wiped before re-embedding")
				}
			}
		})
	}
}

// TestCopyEmbeddingProviderFromTeam_RejectsMissingSourceIdentifiers pins the
// #829 finding: a uuid-typed required body field binds as uuid.Nil when omitted,
// so `{}` would otherwise reach the service and answer 403 for a team that
// cannot exist, where the spec documents 400.
func TestCopyEmbeddingProviderFromTeam_RejectsMissingSourceIdentifiers(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty object", `{}`},
		{"source_provider_id only", fmt.Sprintf(`{"source_provider_id":%q}`, testEmbedCopySourceProvider)},
		{"source_team_id only", fmt.Sprintf(`{"source_team_id":%q}`, testEmbedCopySourceTeamID)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// No CopyFromTeam expectation: the request must never reach the service.
			container := newMockEmbeddingProviderContainer(t)
			srv := createTestEmbeddingProviderServer(container)

			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, makeEmbedCopyRequest(tc.body))

			assertEmbedCopyProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
		})
	}
}

// TestCopyEmbeddingProviderFromTeam_EnforcesThePrefixCap pins the 256-RUNE cap
// on the copy path. The cap is counted in characters, so a 256-rune multi-byte
// prefix is accepted although it is far longer than 256 bytes, and 257 runes is
// rejected — the same rule the create and update paths enforce, through the same
// helper.
func TestCopyEmbeddingProviderFromTeam_EnforcesThePrefixCap(t *testing.T) {
	tests := []struct {
		name       string
		field      string
		prefix     string
		wantStatus int
	}{
		{"query_prefix at the cap", "query_prefix", strings.Repeat("a", 256), http.StatusOK},
		{"query_prefix over the cap", "query_prefix", strings.Repeat("a", 257), http.StatusBadRequest},
		{"document_prefix over the cap", "document_prefix", strings.Repeat("a", 257), http.StatusBadRequest},
		// 256 multi-byte runes is 768 bytes: counting bytes would reject it.
		{"multi-byte at the cap", "document_prefix", strings.Repeat("日", 256), http.StatusOK},
		{"multi-byte over the cap", "document_prefix", strings.Repeat("日", 257), http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			container := newMockEmbeddingProviderContainer(t)
			if tc.wantStatus == http.StatusOK {
				container.embeddingProviderService.EXPECT().
					CopyFromTeam(mock.Anything, mock.Anything).
					Return(embedCopyResult(services.CopyEmbeddingProviderActivation{}), nil)
			}

			body := fmt.Sprintf(
				`{"source_team_id":%q,"source_provider_id":%q,%q:%q}`,
				testEmbedCopySourceTeamID, testEmbedCopySourceProvider, tc.field, tc.prefix,
			)

			srv := createTestEmbeddingProviderServer(container)
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, makeEmbedCopyRequest(body))

			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			if tc.wantStatus == http.StatusBadRequest {
				assertEmbedCopyProblem(t, w, http.StatusBadRequest, "PROVIDER_VALIDATION_FAILED")
			}
		})
	}
}

// TestCopyEmbeddingProviderFromTeam_RejectsEmptyOverrides mirrors the create
// path: an override that is SENT must be non-empty. An absent one is the normal
// case and means "copy the source's value".
func TestCopyEmbeddingProviderFromTeam_RejectsEmptyOverrides(t *testing.T) {
	for _, field := range []string{"name", "provider_type", "model"} {
		t.Run(field, func(t *testing.T) {
			container := newMockEmbeddingProviderContainer(t)
			srv := createTestEmbeddingProviderServer(container)

			body := fmt.Sprintf(
				`{"source_team_id":%q,"source_provider_id":%q,%q:""}`,
				testEmbedCopySourceTeamID, testEmbedCopySourceProvider, field,
			)

			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, makeEmbedCopyRequest(body))

			assertEmbedCopyProblem(t, w, http.StatusBadRequest, "PROVIDER_VALIDATION_FAILED")
		})
	}
}

// TestCopyEmbeddingProviderFromTeam_ServiceErrorsMapToTheirDocumentedStatus
// pins the error mapping, including the one message a denial ever returns —
// naming the team would tell a caller entitled to neither whether the source
// team exists (#829).
func TestCopyEmbeddingProviderFromTeam_ServiceErrorsMapToTheirDocumentedStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"denied", fmt.Errorf("%w: nope", services.ErrPermissionDenied), http.StatusForbidden, "FORBIDDEN"},
		{"source missing", services.ErrCopySourceRequired, http.StatusBadRequest, "BAD_REQUEST"},
		{"source is destination", services.ErrCopySourceIsDestination, http.StatusBadRequest, "BAD_REQUEST"},
		{
			"provider not found",
			fmt.Errorf("%w: x", services.ErrProviderNotFound),
			http.StatusNotFound, "PROVIDER_NOT_FOUND",
		},
		{
			"name taken",
			fmt.Errorf("%w: x", services.ErrProviderAlreadyExists),
			http.StatusConflict, "PROVIDER_ALREADY_EXISTS",
		},
		{"anything else", errors.New("boom"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			container := newMockEmbeddingProviderContainer(t)
			container.embeddingProviderService.EXPECT().
				CopyFromTeam(mock.Anything, mock.Anything).Return(nil, tc.err)

			srv := createTestEmbeddingProviderServer(container)
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, makeEmbedCopyRequest(embedCopyRequestBody()))

			assertEmbedCopyProblem(t, w, tc.wantStatus, tc.wantCode)
		})
	}
}

// TestCopyEmbeddingProviderFromTeam_ForbiddenNeverNamesATeam is the leak guard
// on the shared 403: whichever team the caller was refused on, the body says
// the same thing and mentions neither id.
func TestCopyEmbeddingProviderFromTeam_ForbiddenNeverNamesATeam(t *testing.T) {
	container := newMockEmbeddingProviderContainer(t)
	container.embeddingProviderService.EXPECT().
		CopyFromTeam(mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("%w: refused on %s", services.ErrPermissionDenied, testEmbedCopySourceTeamID))

	srv := createTestEmbeddingProviderServer(container)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, makeEmbedCopyRequest(embedCopyRequestBody()))

	assertEmbedCopyProblem(t, w, http.StatusForbidden, "FORBIDDEN")
	assert.NotContains(t, w.Body.String(), testEmbedCopySourceTeamID,
		"the source team id must never appear: it would confirm the team exists")
}
