package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/services"
	servicemocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// createBackfillTestServer builds a Server whose container exposes the given
// embedding backfill service. Auth is the middleware's responsibility, so the
// handler tests call the handler directly.
func createBackfillTestServer(backfill services.EmbeddingBackfillServiceInterface) *Server {
	return &Server{
		port:   "8080",
		apiKey: "test-api-key",
		config: &config.Config{BackofficeAdminAPIKey: "test-backoffice-key"},
		container: &MockContainerForBackoffice{
			embeddingBackfillService: backfill,
		},
		logger: setupTestLogger(),
	}
}

func TestHandleEmbeddingsBackfill_Success(t *testing.T) {
	mockSvc := servicemocks.NewMockEmbeddingBackfillServiceInterface(t)
	mockSvc.EXPECT().
		Backfill(mock.Anything, services.EmbeddingBackfillRequest{EntityTypes: []string{"prompt"}, DryRun: false}).
		Return(&services.EmbeddingBackfillResult{
			Results:        []services.EmbeddingBackfillTypeResult{{EntityType: "prompt", Total: 3, Published: 3}},
			TotalSeen:      3,
			TotalPublished: 3,
		}, nil)

	srv := createBackfillTestServer(mockSvc)
	req := httptest.NewRequest(http.MethodPost, "/bo/v1/embeddings/backfill",
		strings.NewReader(`{"entity_types":["prompt"]}`))
	rr := httptest.NewRecorder()

	srv.handleEmbeddingsBackfill(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var resp services.EmbeddingBackfillResult
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 3, resp.TotalPublished)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "prompt", resp.Results[0].EntityType)
}

func TestHandleEmbeddingsBackfill_EmptyBody_DefaultsToAll(t *testing.T) {
	mockSvc := servicemocks.NewMockEmbeddingBackfillServiceInterface(t)
	mockSvc.EXPECT().
		Backfill(mock.Anything, services.EmbeddingBackfillRequest{}).
		Return(&services.EmbeddingBackfillResult{}, nil)

	srv := createBackfillTestServer(mockSvc)
	// No body at all — handler must treat EOF as the zero-value request.
	req := httptest.NewRequest(http.MethodPost, "/bo/v1/embeddings/backfill", http.NoBody)
	rr := httptest.NewRecorder()

	srv.handleEmbeddingsBackfill(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleEmbeddingsBackfill_DryRun(t *testing.T) {
	mockSvc := servicemocks.NewMockEmbeddingBackfillServiceInterface(t)
	mockSvc.EXPECT().
		Backfill(mock.Anything, services.EmbeddingBackfillRequest{DryRun: true}).
		Return(&services.EmbeddingBackfillResult{DryRun: true}, nil)

	srv := createBackfillTestServer(mockSvc)
	req := httptest.NewRequest(http.MethodPost, "/bo/v1/embeddings/backfill",
		strings.NewReader(`{"dry_run":true}`))
	rr := httptest.NewRecorder()

	srv.handleEmbeddingsBackfill(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp services.EmbeddingBackfillResult
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.True(t, resp.DryRun)
}

func TestHandleEmbeddingsBackfill_MalformedBody_BadRequest(t *testing.T) {
	mockSvc := servicemocks.NewMockEmbeddingBackfillServiceInterface(t)
	srv := createBackfillTestServer(mockSvc)
	req := httptest.NewRequest(http.MethodPost, "/bo/v1/embeddings/backfill",
		strings.NewReader(`{"entity_types": [`))
	rr := httptest.NewRecorder()

	srv.handleEmbeddingsBackfill(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))
	mockSvc.AssertNotCalled(t, "Backfill", mock.Anything, mock.Anything)
}

func TestHandleEmbeddingsBackfill_UnknownField_BadRequest(t *testing.T) {
	mockSvc := servicemocks.NewMockEmbeddingBackfillServiceInterface(t)
	srv := createBackfillTestServer(mockSvc)
	// A typo'd field ("dryrun" instead of "dry_run") must 400 rather than silently
	// fall through to a full live backfill on this destructive endpoint.
	req := httptest.NewRequest(http.MethodPost, "/bo/v1/embeddings/backfill",
		strings.NewReader(`{"dryrun":true}`))
	rr := httptest.NewRecorder()

	srv.handleEmbeddingsBackfill(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))
	mockSvc.AssertNotCalled(t, "Backfill", mock.Anything, mock.Anything)
}

func TestHandleEmbeddingsBackfill_UnsupportedType_BadRequest(t *testing.T) {
	mockSvc := servicemocks.NewMockEmbeddingBackfillServiceInterface(t)
	mockSvc.EXPECT().
		Backfill(mock.Anything, mock.Anything).
		Return(nil, services.ErrUnsupportedBackfillEntityType)

	srv := createBackfillTestServer(mockSvc)
	req := httptest.NewRequest(http.MethodPost, "/bo/v1/embeddings/backfill",
		strings.NewReader(`{"entity_types":["widget"]}`))
	rr := httptest.NewRecorder()

	srv.handleEmbeddingsBackfill(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleEmbeddingsBackfill_ServiceError_Internal(t *testing.T) {
	mockSvc := servicemocks.NewMockEmbeddingBackfillServiceInterface(t)
	mockSvc.EXPECT().
		Backfill(mock.Anything, mock.Anything).
		Return(nil, context.DeadlineExceeded)

	srv := createBackfillTestServer(mockSvc)
	req := httptest.NewRequest(http.MethodPost, "/bo/v1/embeddings/backfill",
		strings.NewReader(`{}`))
	rr := httptest.NewRecorder()

	srv.handleEmbeddingsBackfill(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}
