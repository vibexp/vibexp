package server

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	testAttachmentTeamID    = "550e8400-e29b-41d4-a716-446655440000"
	testAttachmentProjectID = "660e8400-e29b-41d4-a716-446655440000"
	testAttachmentSlug      = "test-artifact"
	testAttachmentArtifact  = "770e8400-e29b-41d4-a716-446655440000"
	testAttachmentID        = "880e8400-e29b-41d4-a716-446655440000"
	testAttachmentUser      = "user-123"
)

// MockAttachmentContainer exposes mocked artifact + attachment services.
type MockAttachmentContainer struct {
	BaseMockContainer
	ArtifactServiceMock   services.ArtifactServiceInterface
	AttachmentServiceMock services.AttachmentServiceInterface
}

func (m *MockAttachmentContainer) ArtifactService() services.ArtifactServiceInterface {
	return m.ArtifactServiceMock
}

func (m *MockAttachmentContainer) TypeService() services.TypeServiceInterface { return nil }

func (m *MockAttachmentContainer) AttachmentService() services.AttachmentServiceInterface {
	return m.AttachmentServiceMock
}

func newAttachmentTestServer(c *MockAttachmentContainer) *Server {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = c
	// Rebuild the attachment owner-authorizer registry against the mock container
	// so universal-endpoint tests exercise the mocked ArtifactService rather than
	// the real (nil-db) one wired during New.
	srv.attachmentAuthorizers = setupAttachmentAuthorizers(c)
	return srv
}

func sampleArtifact() *models.Artifact {
	return &models.Artifact{
		ID:        testAttachmentArtifact,
		ProjectID: testAttachmentProjectID,
		Slug:      testAttachmentSlug,
		TeamID:    testAttachmentTeamID,
		UserID:    testAttachmentUser,
		Title:     "Test Artifact",
	}
}

func sampleAttachment() *models.Attachment {
	return &models.Attachment{
		ID:          testAttachmentID,
		TeamID:      testAttachmentTeamID,
		UserID:      testAttachmentUser,
		OwnerType:   ownerTypeArtifact,
		OwnerID:     testAttachmentArtifact,
		FileName:    "notes.txt",
		ContentType: "text/plain",
		SizeBytes:   12,
		CreatedAt:   time.Now().UTC(),
	}
}

// attachmentURLParams adds the chi URL params the attachment handlers read.
func attachmentURLParams(req *http.Request, withID bool) *http.Request {
	params := map[string]string{
		"team_id":    testAttachmentTeamID,
		"project_id": testAttachmentProjectID,
		"slug":       testAttachmentSlug,
	}
	if withID {
		params["id"] = testAttachmentID
	}
	return addURLParams(req, params)
}

func mockArtifactResolver(t *testing.T) *servicesmocks.MockArtifactServiceInterface {
	t.Helper()
	m := servicesmocks.NewMockArtifactServiceInterface(t)
	m.On("GetArtifactByProjectIDAndSlugInTeam",
		testAttachmentUser, testAttachmentTeamID, testAttachmentProjectID, testAttachmentSlug,
	).Return(sampleArtifact(), nil)
	return m
}

func TestHandleUploadArtifactAttachment_Success(t *testing.T) {
	mockAttachment := servicesmocks.NewMockAttachmentServiceInterface(t)
	mockAttachment.On("Upload", mock.Anything, mock.AnythingOfType("services.UploadAttachmentParams")).
		Return(sampleAttachment(), nil)

	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   mockArtifactResolver(t),
		AttachmentServiceMock: mockAttachment,
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "notes.txt")
	assert.NoError(t, err)
	_, err = part.Write([]byte("hello world!"))
	assert.NoError(t, err)
	assert.NoError(t, writer.Close())

	url := "/api/v1/" + testAttachmentTeamID + "/artifacts/" + testAttachmentProjectID +
		"/" + testAttachmentSlug + "/attachments"
	req := httptest.NewRequest(http.MethodPost, url, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testAttachmentUser))
	req = attachmentURLParams(req, false)

	rr := httptest.NewRecorder()
	srv.handleUploadArtifactAttachment(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestHandleListArtifactAttachments_Success(t *testing.T) {
	mockAttachment := servicesmocks.NewMockAttachmentServiceInterface(t)
	mockAttachment.On("List", mock.Anything, ownerTypeArtifact, testAttachmentArtifact).
		Return(&models.AttachmentListResponse{
			Attachments:    []models.Attachment{*sampleAttachment()},
			TotalCount:     1,
			TotalSizeBytes: 12,
		}, nil)

	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   mockArtifactResolver(t),
		AttachmentServiceMock: mockAttachment,
	})

	url := "/api/v1/" + testAttachmentTeamID + "/artifacts/" + testAttachmentProjectID +
		"/" + testAttachmentSlug + "/attachments"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testAttachmentUser))
	req = attachmentURLParams(req, false)

	rr := httptest.NewRecorder()
	srv.handleListArtifactAttachments(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestHandleDownloadArtifactAttachment_Success(t *testing.T) {
	mockAttachment := servicesmocks.NewMockAttachmentServiceInterface(t)
	att := sampleAttachment()
	mockAttachment.On("Get", mock.Anything, ownerTypeArtifact, testAttachmentArtifact, testAttachmentID).
		Return(att, nil)
	mockAttachment.On("Download", mock.Anything, att).
		Return(io.NopCloser(bytes.NewReader([]byte("hello world!"))), nil)

	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   mockArtifactResolver(t),
		AttachmentServiceMock: mockAttachment,
	})

	url := "/api/v1/" + testAttachmentTeamID + "/artifacts/" + testAttachmentProjectID +
		"/" + testAttachmentSlug + "/attachments/" + testAttachmentID
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testAttachmentUser))
	req = attachmentURLParams(req, true)

	rr := httptest.NewRecorder()
	srv.handleDownloadArtifactAttachment(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/octet-stream", rr.Header().Get("Content-Type"))
	assert.Contains(t, rr.Header().Get("Content-Disposition"), "attachment;")
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestHandleDeleteArtifactAttachment_Success(t *testing.T) {
	mockAttachment := servicesmocks.NewMockAttachmentServiceInterface(t)
	mockAttachment.On("Delete", mock.Anything, ownerTypeArtifact, testAttachmentArtifact, testAttachmentID).
		Return(nil)

	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   mockArtifactResolver(t),
		AttachmentServiceMock: mockAttachment,
	})

	url := "/api/v1/" + testAttachmentTeamID + "/artifacts/" + testAttachmentProjectID +
		"/" + testAttachmentSlug + "/attachments/" + testAttachmentID
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testAttachmentUser))
	req = attachmentURLParams(req, true)

	rr := httptest.NewRecorder()
	srv.handleDeleteArtifactAttachment(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestHandleUploadArtifactAttachment_MissingFile(t *testing.T) {
	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   mockArtifactResolver(t),
		AttachmentServiceMock: servicesmocks.NewMockAttachmentServiceInterface(t),
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	assert.NoError(t, writer.Close())

	url := "/api/v1/" + testAttachmentTeamID + "/artifacts/" + testAttachmentProjectID +
		"/" + testAttachmentSlug + "/attachments"
	req := httptest.NewRequest(http.MethodPost, url, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testAttachmentUser))
	req = attachmentURLParams(req, false)

	rr := httptest.NewRecorder()
	srv.handleUploadArtifactAttachment(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
