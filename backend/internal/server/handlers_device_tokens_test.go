package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// deviceTokenContainerStub implements container.Container for device-token handler tests.
type deviceTokenContainerStub struct {
	BaseMockContainer
	tokenRepo *repomocks.MockDeviceTokenRepository
}

func (s *deviceTokenContainerStub) DeviceTokenRepository() repositories.DeviceTokenRepository {
	return s.tokenRepo
}

func buildDeviceTokenServer(t *testing.T, repo *repomocks.MockDeviceTokenRepository) *Server {
	t.Helper()

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	r := chi.NewRouter()
	ctr := &deviceTokenContainerStub{tokenRepo: repo}

	srv := &Server{
		port:      "8080",
		container: ctr,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	r.Post("/api/v1/device-tokens", srv.handleRegisterDeviceToken)
	r.Delete("/api/v1/device-tokens", srv.handleDeleteDeviceToken)

	return srv
}

// makeDeviceTokenReq creates an authenticated device-token request for the default test user.
func makeDeviceTokenReq(t *testing.T, method, body string) *http.Request {
	t.Helper()

	const testUserID = "user-1"

	var buf *bytes.Buffer
	if body != "" {
		buf = bytes.NewBufferString(body)
	} else {
		buf = &bytes.Buffer{}
	}

	req, err := http.NewRequest(method, "/api/v1/device-tokens", buf)
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")

	ctx := context.WithValue(req.Context(), contextKeyUserID, testUserID)
	req = req.WithContext(ctx)

	return req
}

// TestRegisterDeviceToken_Unauthorized verifies the route requires authentication.
func TestRegisterDeviceToken_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest(http.MethodPost, "/api/v1/device-tokens", bytes.NewBufferString(`{}`))
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestDeleteDeviceToken_Unauthorized verifies the route requires authentication.
func TestDeleteDeviceToken_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest(http.MethodDelete, "/api/v1/device-tokens", bytes.NewBufferString(`{}`))
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleRegisterDeviceToken_InvalidJSON(t *testing.T) {
	repo := new(repomocks.MockDeviceTokenRepository)
	srv := buildDeviceTokenServer(t, repo)

	req := makeDeviceTokenReq(t, http.MethodPost, `{bad json}`)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleRegisterDeviceToken_MissingToken(t *testing.T) {
	repo := new(repomocks.MockDeviceTokenRepository)
	srv := buildDeviceTokenServer(t, repo)

	req := makeDeviceTokenReq(t, http.MethodPost, `{"platform":"web"}`)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleRegisterDeviceToken_MissingPlatform(t *testing.T) {
	repo := new(repomocks.MockDeviceTokenRepository)
	srv := buildDeviceTokenServer(t, repo)

	req := makeDeviceTokenReq(t, http.MethodPost, `{"token":"fcm-abc"}`)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleRegisterDeviceToken_InvalidPlatform(t *testing.T) {
	repo := new(repomocks.MockDeviceTokenRepository)
	srv := buildDeviceTokenServer(t, repo)

	req := makeDeviceTokenReq(t, http.MethodPost, `{"token":"fcm-abc","platform":"android"}`)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleRegisterDeviceToken_TokenTooLong(t *testing.T) {
	repo := new(repomocks.MockDeviceTokenRepository)
	srv := buildDeviceTokenServer(t, repo)

	longToken := strings.Repeat("x", 4097)
	body, err := json.Marshal(map[string]string{"token": longToken, "platform": "web"})
	require.NoError(t, err)
	req := makeDeviceTokenReq(t, http.MethodPost, string(body))
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleRegisterDeviceToken_UpsertError(t *testing.T) {
	repo := new(repomocks.MockDeviceTokenRepository)
	repo.On("Upsert", mock.Anything, mock.MatchedBy(func(dt *models.DeviceToken) bool {
		return dt.UserID == "user-1" && dt.Token == "fcm-abc" && dt.Platform == "web"
	})).Return(errors.New("db error"))

	srv := buildDeviceTokenServer(t, repo)

	req := makeDeviceTokenReq(t, http.MethodPost, `{"token":"fcm-abc","platform":"web"}`)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	repo.AssertExpectations(t)
}

func TestHandleRegisterDeviceToken_TokenBelongsToAnotherUser(t *testing.T) {
	repo := new(repomocks.MockDeviceTokenRepository)
	repo.On("Upsert", mock.Anything, mock.MatchedBy(func(dt *models.DeviceToken) bool {
		return dt.UserID == "user-1" && dt.Token == "shared-token" && dt.Platform == "web"
	})).Return(repositories.ErrDeviceTokenConflict)

	srv := buildDeviceTokenServer(t, repo)

	req := makeDeviceTokenReq(t, http.MethodPost, `{"token":"shared-token","platform":"web"}`)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Code)
	repo.AssertExpectations(t)
}

func TestHandleRegisterDeviceToken_Success(t *testing.T) {
	repo := new(repomocks.MockDeviceTokenRepository)
	repo.On("Upsert", mock.Anything, mock.MatchedBy(func(dt *models.DeviceToken) bool {
		return dt.UserID == "user-1" && dt.Token == "fcm-abc" &&
			dt.Platform == "web" && dt.UserAgent == "Mozilla"
	})).Return(nil)

	srv := buildDeviceTokenServer(t, repo)

	body := `{"token":"fcm-abc","platform":"web","user_agent":"Mozilla"}`
	req := makeDeviceTokenReq(t, http.MethodPost, body)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	repo.AssertExpectations(t)
}

func TestHandleRegisterDeviceToken_MissingUserContext(t *testing.T) {
	repo := new(repomocks.MockDeviceTokenRepository)
	srv := buildDeviceTokenServer(t, repo)

	// Build a request without injecting contextKeyUserID
	req, err := http.NewRequest(http.MethodPost, "/api/v1/device-tokens",
		bytes.NewBufferString(`{"token":"fcm-abc","platform":"web"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleDeleteDeviceToken_InvalidJSON(t *testing.T) {
	repo := new(repomocks.MockDeviceTokenRepository)
	srv := buildDeviceTokenServer(t, repo)

	req := makeDeviceTokenReq(t, http.MethodDelete, `{bad}`)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleDeleteDeviceToken_MissingToken(t *testing.T) {
	repo := new(repomocks.MockDeviceTokenRepository)
	srv := buildDeviceTokenServer(t, repo)

	req := makeDeviceTokenReq(t, http.MethodDelete, `{}`)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleDeleteDeviceToken_DeleteError(t *testing.T) {
	repo := new(repomocks.MockDeviceTokenRepository)
	repo.On("Delete", mock.Anything, "fcm-abc", "user-1").Return(errors.New("db error"))

	srv := buildDeviceTokenServer(t, repo)

	payload, err := json.Marshal(map[string]string{"token": "fcm-abc"})
	require.NoError(t, err)
	req := makeDeviceTokenReq(t, http.MethodDelete, string(payload))
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	repo.AssertExpectations(t)
}

func TestHandleDeleteDeviceToken_Success(t *testing.T) {
	repo := new(repomocks.MockDeviceTokenRepository)
	repo.On("Delete", mock.Anything, "fcm-abc", "user-1").Return(nil)

	srv := buildDeviceTokenServer(t, repo)

	payload, err := json.Marshal(map[string]string{"token": "fcm-abc"})
	require.NoError(t, err)
	req := makeDeviceTokenReq(t, http.MethodDelete, string(payload))
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	repo.AssertExpectations(t)
}
