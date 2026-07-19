package server

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// universalAttachmentsURL is the universal collection endpoint.
const universalAttachmentsURL = "/api/v1/" + testAttachmentTeamID + "/attachments"

// mockOwnerAuthorizer returns an ArtifactService mock whose by-id access check
// (the owner_type="artifact" authorizer) succeeds for the sample artifact.
func mockOwnerAuthorizer(t *testing.T) *servicesmocks.MockArtifactServiceInterface {
	t.Helper()
	m := servicesmocks.NewMockArtifactServiceInterface(t)
	m.On("GetArtifactByIDInTeam", testAttachmentUser, testAttachmentTeamID, testAttachmentArtifact).
		Return(sampleArtifact(), nil).Maybe()
	return m
}

// universalUploadBody builds a multipart body with the given owner fields + a file.
func universalUploadBody(t *testing.T, ownerType, ownerID string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if ownerType != "" {
		assert.NoError(t, writer.WriteField("owner_type", ownerType))
	}
	if ownerID != "" {
		assert.NoError(t, writer.WriteField("owner_id", ownerID))
	}
	part, err := writer.CreateFormFile("file", "notes.txt")
	assert.NoError(t, err)
	_, err = part.Write([]byte("hello world!"))
	assert.NoError(t, err)
	assert.NoError(t, writer.Close())
	return body, writer.FormDataContentType()
}

func newUniversalReq(t *testing.T, method, url string, body io.Reader, withID bool) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, url, body)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testAttachmentUser))
	params := map[string]string{"team_id": testAttachmentTeamID}
	if withID {
		params["id"] = testAttachmentID
	}
	return addURLParams(req, params)
}

func TestHandleUploadAttachment_Success(t *testing.T) {
	mockAttachment := servicesmocks.NewMockAttachmentServiceInterface(t)
	mockAttachment.On("Upload", mock.Anything, mock.AnythingOfType("services.UploadAttachmentParams")).
		Return(sampleAttachment(), nil)

	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   mockOwnerAuthorizer(t),
		AttachmentServiceMock: mockAttachment,
	})

	body, contentType := universalUploadBody(t, ownerTypeArtifact, testAttachmentArtifact)
	req := newUniversalReq(t, http.MethodPost, universalAttachmentsURL, body, false)
	req.Header.Set("Content-Type", contentType)

	rr := httptest.NewRecorder()
	srv.handleUploadAttachment(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestHandleUploadAttachment_RelativePath verifies the universal upload endpoint
// (used by blueprints/prompts/memories) threads the relative_path form field to
// the service (#338).
func TestHandleUploadAttachment_RelativePath(t *testing.T) {
	att := sampleAttachment()
	att.RelativePath = "scripts/helper.txt"
	mockAttachment := servicesmocks.NewMockAttachmentServiceInterface(t)
	mockAttachment.On("Upload", mock.Anything, mock.MatchedBy(func(p services.UploadAttachmentParams) bool {
		return p.RelativePath == "scripts/helper.txt"
	})).Return(att, nil)

	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   mockOwnerAuthorizer(t),
		AttachmentServiceMock: mockAttachment,
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	assert.NoError(t, writer.WriteField("owner_type", ownerTypeArtifact))
	assert.NoError(t, writer.WriteField("owner_id", testAttachmentArtifact))
	assert.NoError(t, writer.WriteField("relative_path", "scripts/helper.txt"))
	part, err := writer.CreateFormFile("file", "helper.txt")
	assert.NoError(t, err)
	_, err = part.Write([]byte("hi"))
	assert.NoError(t, err)
	assert.NoError(t, writer.Close())

	req := newUniversalReq(t, http.MethodPost, universalAttachmentsURL, body, false)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	srv.handleUploadAttachment(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	assert.Contains(t, rr.Body.String(), `"relative_path":"scripts/helper.txt"`)
	mockAttachment.AssertExpectations(t)
}

func TestHandleUploadAttachment_UnknownOwnerType(t *testing.T) {
	// Attachment service must never be touched for an unsupported owner_type.
	mockAttachment := servicesmocks.NewMockAttachmentServiceInterface(t)

	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   servicesmocks.NewMockArtifactServiceInterface(t),
		AttachmentServiceMock: mockAttachment,
	})

	body, contentType := universalUploadBody(t, "memory", testAttachmentArtifact)
	req := newUniversalReq(t, http.MethodPost, universalAttachmentsURL, body, false)
	req.Header.Set("Content-Type", contentType)

	rr := httptest.NewRecorder()
	srv.handleUploadAttachment(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	mockAttachment.AssertNotCalled(t, "Upload", mock.Anything, mock.Anything)
}

func TestHandleUploadAttachment_AccessDenied(t *testing.T) {
	artifactMock := servicesmocks.NewMockArtifactServiceInterface(t)
	artifactMock.On("GetArtifactByIDInTeam", testAttachmentUser, testAttachmentTeamID, testAttachmentArtifact).
		Return(nil, repositories.ErrArtifactNotFound)
	mockAttachment := servicesmocks.NewMockAttachmentServiceInterface(t)

	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   artifactMock,
		AttachmentServiceMock: mockAttachment,
	})

	body, contentType := universalUploadBody(t, ownerTypeArtifact, testAttachmentArtifact)
	req := newUniversalReq(t, http.MethodPost, universalAttachmentsURL, body, false)
	req.Header.Set("Content-Type", contentType)

	rr := httptest.NewRecorder()
	srv.handleUploadAttachment(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	mockAttachment.AssertNotCalled(t, "Upload", mock.Anything, mock.Anything)
}

func TestHandleUploadAttachment_MissingOwnerFields(t *testing.T) {
	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   servicesmocks.NewMockArtifactServiceInterface(t),
		AttachmentServiceMock: servicesmocks.NewMockAttachmentServiceInterface(t),
	})

	body, contentType := universalUploadBody(t, "", "")
	req := newUniversalReq(t, http.MethodPost, universalAttachmentsURL, body, false)
	req.Header.Set("Content-Type", contentType)

	rr := httptest.NewRecorder()
	srv.handleUploadAttachment(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleUploadAttachment_InvalidOwnerID(t *testing.T) {
	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   servicesmocks.NewMockArtifactServiceInterface(t),
		AttachmentServiceMock: servicesmocks.NewMockAttachmentServiceInterface(t),
	})

	body, contentType := universalUploadBody(t, ownerTypeArtifact, "not-a-uuid")
	req := newUniversalReq(t, http.MethodPost, universalAttachmentsURL, body, false)
	req.Header.Set("Content-Type", contentType)

	rr := httptest.NewRecorder()
	srv.handleUploadAttachment(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleListAttachments_Success(t *testing.T) {
	mockAttachment := servicesmocks.NewMockAttachmentServiceInterface(t)
	mockAttachment.On("List", mock.Anything, ownerTypeArtifact, testAttachmentArtifact).
		Return(&models.AttachmentListResponse{
			Attachments:    []models.Attachment{*sampleAttachment()},
			TotalCount:     1,
			TotalSizeBytes: 12,
		}, nil)

	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   mockOwnerAuthorizer(t),
		AttachmentServiceMock: mockAttachment,
	})

	url := universalAttachmentsURL + "?owner_type=" + ownerTypeArtifact + "&owner_id=" + testAttachmentArtifact
	req := newUniversalReq(t, http.MethodGet, url, nil, false)

	rr := httptest.NewRecorder()
	srv.handleListAttachments(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestHandleListAttachments_MissingParams(t *testing.T) {
	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   servicesmocks.NewMockArtifactServiceInterface(t),
		AttachmentServiceMock: servicesmocks.NewMockAttachmentServiceInterface(t),
	})

	req := newUniversalReq(t, http.MethodGet, universalAttachmentsURL, nil, false)

	rr := httptest.NewRecorder()
	srv.handleListAttachments(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleDownloadAttachment_Success(t *testing.T) {
	att := sampleAttachment()
	mockAttachment := servicesmocks.NewMockAttachmentServiceInterface(t)
	mockAttachment.On("GetByIDInTeam", mock.Anything, testAttachmentTeamID, testAttachmentID).
		Return(att, nil)
	mockAttachment.On("Download", mock.Anything, att).
		Return(io.NopCloser(bytes.NewReader([]byte("hello world!"))), nil)

	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   mockOwnerAuthorizer(t),
		AttachmentServiceMock: mockAttachment,
	})

	url := universalAttachmentsURL + "/" + testAttachmentID
	req := newUniversalReq(t, http.MethodGet, url, nil, true)

	rr := httptest.NewRecorder()
	srv.handleDownloadAttachment(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/octet-stream", rr.Header().Get("Content-Type"))
	assert.Contains(t, rr.Header().Get("Content-Disposition"), "attachment;")
	assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestHandleDownloadAttachment_NotFound(t *testing.T) {
	mockAttachment := servicesmocks.NewMockAttachmentServiceInterface(t)
	mockAttachment.On("GetByIDInTeam", mock.Anything, testAttachmentTeamID, testAttachmentID).
		Return(nil, repositories.ErrAttachmentNotFound)

	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   servicesmocks.NewMockArtifactServiceInterface(t),
		AttachmentServiceMock: mockAttachment,
	})

	url := universalAttachmentsURL + "/" + testAttachmentID
	req := newUniversalReq(t, http.MethodGet, url, nil, true)

	rr := httptest.NewRecorder()
	srv.handleDownloadAttachment(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandleDeleteAttachment_Success(t *testing.T) {
	att := sampleAttachment()
	mockAttachment := servicesmocks.NewMockAttachmentServiceInterface(t)
	mockAttachment.On("GetByIDInTeam", mock.Anything, testAttachmentTeamID, testAttachmentID).
		Return(att, nil)
	mockAttachment.On("Delete", mock.Anything, ownerTypeArtifact, testAttachmentArtifact, testAttachmentID).
		Return(nil)

	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   mockOwnerAuthorizer(t),
		AttachmentServiceMock: mockAttachment,
	})

	url := universalAttachmentsURL + "/" + testAttachmentID
	req := newUniversalReq(t, http.MethodDelete, url, nil, true)

	rr := httptest.NewRecorder()
	srv.handleDeleteAttachment(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestHandleDeleteAttachment_OwnerAccessDenied proves a caller who can reach a
// valid attachment id but cannot access its owner is refused (and the row is not
// deleted) — the cross-owner access guard.
func TestHandleDeleteAttachment_OwnerAccessDenied(t *testing.T) {
	att := sampleAttachment()
	mockAttachment := servicesmocks.NewMockAttachmentServiceInterface(t)
	mockAttachment.On("GetByIDInTeam", mock.Anything, testAttachmentTeamID, testAttachmentID).
		Return(att, nil)
	artifactMock := servicesmocks.NewMockArtifactServiceInterface(t)
	artifactMock.On("GetArtifactByIDInTeam", testAttachmentUser, testAttachmentTeamID, testAttachmentArtifact).
		Return(nil, repositories.ErrArtifactNotFound)

	srv := newAttachmentTestServer(&MockAttachmentContainer{
		ArtifactServiceMock:   artifactMock,
		AttachmentServiceMock: mockAttachment,
	})

	url := universalAttachmentsURL + "/" + testAttachmentID
	req := newUniversalReq(t, http.MethodDelete, url, nil, true)

	rr := httptest.NewRecorder()
	srv.handleDeleteAttachment(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	mockAttachment.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
