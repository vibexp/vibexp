package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

// attachmentMCPMocks bundles the service mocks an attachment MCP tool test drives.
type attachmentMCPMocks struct {
	attachment *mocks.MockAttachmentServiceInterface
	artifact   *mocks.MockArtifactServiceInterface
}

// newAttachmentMCPTestServer builds a *Server whose container exposes mocked
// team, artifact, and attachment services, and rebuilds the owner-authorizer
// registry against those mocks so the universal authorizer runs the mocked
// ArtifactService. The member user belongs to memberTeam() (testTeamUUID).
func newAttachmentMCPTestServer(t *testing.T) (*Server, attachmentMCPMocks) {
	t.Helper()
	srv := newServerWithNullLogger(t)

	mockTeam := mocks.NewMockTeamServiceInterface(t)
	mockArtifact := mocks.NewMockArtifactServiceInterface(t)
	mockAttachment := mocks.NewMockAttachmentServiceInterface(t)

	c := &TestContainer{
		TeamServiceMock:       mockTeam,
		ArtifactServiceMock:   mockArtifact,
		AttachmentServiceMock: mockAttachment,
	}
	srv.container = c
	// Rebuild the registry against the mock container (mirrors newAttachmentTestServer).
	srv.attachmentAuthorizers = setupAttachmentAuthorizers(c)

	stubUserTeams(mockTeam, []models.Team{memberTeam()})
	return srv, attachmentMCPMocks{attachment: mockAttachment, artifact: mockArtifact}
}

// stubArtifactOwnerAllowed makes the artifact authorizer succeed for the sample
// artifact owner under the member user + team.
func stubArtifactOwnerAllowed(m *mocks.MockArtifactServiceInterface) {
	m.On("GetArtifactByIDInTeam", testMemberUserID, testTeamUUID, testAttachmentArtifact).
		Return(sampleArtifact(), nil).Maybe()
}

// stubArtifactOwnerDenied makes the artifact authorizer report the owner as not
// found, which the authorizer maps to access-denied.
func stubArtifactOwnerDenied(m *mocks.MockArtifactServiceInterface) {
	m.On("GetArtifactByIDInTeam", testMemberUserID, testTeamUUID, testAttachmentArtifact).
		Return(nil, repositories.ErrArtifactNotFound).Maybe()
}

const helloBase64 = "aGVsbG8gd29ybGQh" // "hello world!" — 12 bytes, matches sampleAttachment()

func TestUploadAttachment_Success(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)
	stubArtifactOwnerAllowed(m.artifact)
	m.attachment.On("Upload", mock.Anything, mock.MatchedBy(func(p services.UploadAttachmentParams) bool {
		return p.TeamID == testTeamUUID &&
			p.UserID == testMemberUserID &&
			p.OwnerType == ownerTypeArtifact &&
			p.OwnerID == testAttachmentArtifact &&
			p.FileName == "notes.txt" &&
			p.DeclaredSize == 12
	})).Return(sampleAttachment(), nil)

	params := &UploadAttachmentParams{
		TeamID:            testTeamSlug, // exercise slug resolution
		OwnerType:         ownerTypeArtifact,
		OwnerID:           testAttachmentArtifact,
		FileName:          "notes.txt",
		FileContentBase64: helloBase64,
	}
	res, structured, err := srv.uploadAttachment(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected success, got %s", extractText(t, res))

	resp, ok := structured.(*attachmentUploadResponse)
	require.True(t, ok)
	assert.Equal(t, testAttachmentID, resp.ID)
	assert.Equal(t, "notes.txt", resp.FileName)
	assert.Equal(t, "text/plain", resp.ContentType)
	assert.Equal(t, int64(12), resp.SizeBytes)
	assert.Equal(t, "/api/v1/"+testTeamUUID+"/attachments/"+testAttachmentID, resp.DownloadURL)

	// The text content must be the same JSON.
	var fromText attachmentUploadResponse
	require.NoError(t, json.Unmarshal([]byte(extractText(t, res)), &fromText))
	assert.Equal(t, *resp, fromText)
}

// TestUploadAttachment_RelativePath verifies the MCP tool threads relative_path
// to the service and echoes it in the response (#338).
func TestUploadAttachment_RelativePath(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)
	stubArtifactOwnerAllowed(m.artifact)
	att := sampleAttachment()
	att.RelativePath = "scripts/helper.txt"
	m.attachment.On("Upload", mock.Anything, mock.MatchedBy(func(p services.UploadAttachmentParams) bool {
		return p.RelativePath == "scripts/helper.txt"
	})).Return(att, nil)

	params := &UploadAttachmentParams{
		TeamID:            testTeamSlug,
		OwnerType:         ownerTypeArtifact,
		OwnerID:           testAttachmentArtifact,
		FileName:          "helper.txt",
		FileContentBase64: helloBase64,
		RelativePath:      "scripts/helper.txt",
	}
	res, structured, err := srv.uploadAttachment(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	require.False(t, res.IsError, "expected success, got %s", extractText(t, res))
	resp, ok := structured.(*attachmentUploadResponse)
	require.True(t, ok)
	assert.Equal(t, "scripts/helper.txt", resp.RelativePath)
	m.attachment.AssertExpectations(t)
}

func TestUploadAttachment_InvalidBase64(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)
	stubArtifactOwnerAllowed(m.artifact)

	params := &UploadAttachmentParams{
		TeamID:            testTeamUUID,
		OwnerType:         ownerTypeArtifact,
		OwnerID:           testAttachmentArtifact,
		FileName:          "notes.txt",
		FileContentBase64: "this is not base64!!!",
	}
	res, structured, err := srv.uploadAttachment(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	assert.Nil(t, structured)
	require.True(t, res.IsError)
	assert.Contains(t, extractText(t, res), "not valid base64")
	m.attachment.AssertNotCalled(t, "Upload")
}

func TestUploadAttachment_OversizedBase64(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)

	// One char over the encoded length of the 5 MB per-file limit; rejected before decode.
	tooLong := strings.Repeat("A", base64.StdEncoding.EncodedLen(int(services.MaxAttachmentFileSize))+1)
	params := &UploadAttachmentParams{
		TeamID:            testTeamUUID,
		OwnerType:         ownerTypeArtifact,
		OwnerID:           testAttachmentArtifact,
		FileName:          "big.bin",
		FileContentBase64: tooLong,
	}
	res, structured, err := srv.uploadAttachment(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	assert.Nil(t, structured)
	require.True(t, res.IsError)
	assert.Contains(t, extractText(t, res), "too large")
	m.attachment.AssertNotCalled(t, "Upload")
	// Memory-bounding guard runs before authorization, so the owner is never checked.
	m.artifact.AssertNotCalled(t, "GetArtifactByIDInTeam")
}

func TestUploadAttachment_UnknownOwnerType(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)

	params := &UploadAttachmentParams{
		TeamID:            testTeamUUID,
		OwnerType:         "memory", // not a registered owner type (artifact/prompt/blueprint are)
		OwnerID:           testAttachmentArtifact,
		FileName:          "notes.txt",
		FileContentBase64: helloBase64,
	}
	res, structured, err := srv.uploadAttachment(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	assert.Nil(t, structured)
	require.True(t, res.IsError)
	// Generic, anti-enumeration message — must not reveal the supported owner types.
	assert.Contains(t, extractText(t, res), "does not exist or is not accessible")
	m.attachment.AssertNotCalled(t, "Upload")
}

func TestUploadAttachment_ForbiddenOwner(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)
	stubArtifactOwnerDenied(m.artifact)

	params := &UploadAttachmentParams{
		TeamID:            testTeamUUID,
		OwnerType:         ownerTypeArtifact,
		OwnerID:           testAttachmentArtifact,
		FileName:          "notes.txt",
		FileContentBase64: helloBase64,
	}
	res, structured, err := srv.uploadAttachment(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	assert.Nil(t, structured)
	require.True(t, res.IsError)
	assert.Contains(t, extractText(t, res), "does not exist or is not accessible")
	m.attachment.AssertNotCalled(t, "Upload")
}

func TestUploadAttachment_NonMemberTeamDenied(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)

	params := &UploadAttachmentParams{
		TeamID:            testOtherTeamUUID, // user is not a member
		OwnerType:         ownerTypeArtifact,
		OwnerID:           testAttachmentArtifact,
		FileName:          "notes.txt",
		FileContentBase64: helloBase64,
	}
	res, structured, err := srv.uploadAttachment(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	assert.Nil(t, structured)
	assertGenericAccessDenied(t, res)
	m.attachment.AssertNotCalled(t, "Upload")
}

func TestUploadAttachment_InvalidOwnerID(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)

	params := &UploadAttachmentParams{
		TeamID:            testTeamUUID,
		OwnerType:         ownerTypeArtifact,
		OwnerID:           "not-a-uuid",
		FileName:          "notes.txt",
		FileContentBase64: helloBase64,
	}
	res, structured, err := srv.uploadAttachment(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	assert.Nil(t, structured)
	require.True(t, res.IsError)
	assert.Contains(t, extractText(t, res), "owner_id must be a valid UUID")
	m.attachment.AssertNotCalled(t, "Upload")
}

func TestUploadAttachment_MissingFields(t *testing.T) {
	srv, _ := newAttachmentMCPTestServer(t)

	params := &UploadAttachmentParams{
		TeamID:            testTeamUUID,
		OwnerType:         "",
		OwnerID:           testAttachmentArtifact,
		FileName:          "notes.txt",
		FileContentBase64: helloBase64,
	}
	res, _, err := srv.uploadAttachment(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Contains(t, extractText(t, res), "required")
}

func TestListAttachments_Success(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)
	stubArtifactOwnerAllowed(m.artifact)
	att := sampleAttachment()
	m.attachment.On("List", mock.Anything, ownerTypeArtifact, testAttachmentArtifact).
		Return(&models.AttachmentListResponse{
			Attachments:    []models.Attachment{*att},
			TotalCount:     1,
			TotalSizeBytes: att.SizeBytes,
		}, nil)

	params := &ListAttachmentsParams{
		TeamID:    testTeamUUID,
		OwnerType: ownerTypeArtifact,
		OwnerID:   testAttachmentArtifact,
	}
	res, structured, err := srv.listAttachments(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	require.False(t, res.IsError, "expected success, got %s", extractText(t, res))

	resp, ok := structured.(*attachmentListResponse)
	require.True(t, ok)
	require.Len(t, resp.Attachments, 1)
	assert.Equal(t, 1, resp.TotalCount)
	assert.Equal(t, att.SizeBytes, resp.TotalSizeBytes)
	item := resp.Attachments[0]
	assert.Equal(t, testAttachmentID, item.ID)
	assert.Equal(t, "notes.txt", item.FileName)
	assert.Equal(t, "/api/v1/"+testTeamUUID+"/attachments/"+testAttachmentID, item.DownloadURL)
}

func TestListAttachments_ForbiddenOwner(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)
	stubArtifactOwnerDenied(m.artifact)

	params := &ListAttachmentsParams{
		TeamID:    testTeamUUID,
		OwnerType: ownerTypeArtifact,
		OwnerID:   testAttachmentArtifact,
	}
	res, structured, err := srv.listAttachments(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	assert.Nil(t, structured)
	require.True(t, res.IsError)
	assert.Contains(t, extractText(t, res), "does not exist or is not accessible")
	m.attachment.AssertNotCalled(t, "List")
}

func TestListAttachments_UnknownOwnerType(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)

	params := &ListAttachmentsParams{
		TeamID:    testTeamUUID,
		OwnerType: "memory", // not a registered owner type (artifact/prompt/blueprint are)
		OwnerID:   testAttachmentArtifact,
	}
	res, structured, err := srv.listAttachments(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	assert.Nil(t, structured)
	require.True(t, res.IsError)
	assert.Contains(t, extractText(t, res), "does not exist or is not accessible")
	m.attachment.AssertNotCalled(t, "List")
}

func TestDeleteAttachment_Success(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)
	stubArtifactOwnerAllowed(m.artifact)
	m.attachment.On("GetByIDInTeam", mock.Anything, testTeamUUID, testAttachmentID).
		Return(sampleAttachment(), nil)
	m.attachment.On("Delete", mock.Anything, ownerTypeArtifact, testAttachmentArtifact, testAttachmentID).
		Return(nil)

	params := &DeleteAttachmentParams{TeamID: testTeamUUID, AttachmentID: testAttachmentID}
	res, structured, err := srv.deleteAttachment(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	require.False(t, res.IsError, "expected success, got %s", extractText(t, res))

	resp, ok := structured.(*attachmentDeleteResponse)
	require.True(t, ok)
	assert.Equal(t, testAttachmentID, resp.ID)
	assert.True(t, resp.Deleted)
}

func TestDeleteAttachment_NotFound(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)
	m.attachment.On("GetByIDInTeam", mock.Anything, testTeamUUID, testAttachmentID).
		Return(nil, repositories.ErrAttachmentNotFound)

	params := &DeleteAttachmentParams{TeamID: testTeamUUID, AttachmentID: testAttachmentID}
	res, structured, err := srv.deleteAttachment(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	assert.Nil(t, structured)
	require.True(t, res.IsError)
	assert.Contains(t, extractText(t, res), "not found or not accessible")
	m.attachment.AssertNotCalled(t, "Delete")
}

func TestDeleteAttachment_ForbiddenOwner(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)
	stubArtifactOwnerDenied(m.artifact)
	m.attachment.On("GetByIDInTeam", mock.Anything, testTeamUUID, testAttachmentID).
		Return(sampleAttachment(), nil)

	params := &DeleteAttachmentParams{TeamID: testTeamUUID, AttachmentID: testAttachmentID}
	res, structured, err := srv.deleteAttachment(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	assert.Nil(t, structured)
	require.True(t, res.IsError)
	assert.Contains(t, extractText(t, res), "does not exist or is not accessible")
	m.attachment.AssertNotCalled(t, "Delete")
}

func TestDeleteAttachment_NonMemberTeamDenied(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)

	params := &DeleteAttachmentParams{TeamID: testOtherTeamUUID, AttachmentID: testAttachmentID}
	res, structured, err := srv.deleteAttachment(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	assert.Nil(t, structured)
	assertGenericAccessDenied(t, res)
	m.attachment.AssertNotCalled(t, "GetByIDInTeam")
	m.attachment.AssertNotCalled(t, "Delete")
}

func TestDeleteAttachment_InvalidID(t *testing.T) {
	srv, m := newAttachmentMCPTestServer(t)

	params := &DeleteAttachmentParams{TeamID: testTeamUUID, AttachmentID: "nope"}
	res, structured, err := srv.deleteAttachment(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	assert.Nil(t, structured)
	require.True(t, res.IsError)
	assert.Contains(t, extractText(t, res), "attachment_id must be a valid UUID")
	m.attachment.AssertNotCalled(t, "GetByIDInTeam")
}
