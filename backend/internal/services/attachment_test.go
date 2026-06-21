package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

const (
	svcOwnerType = "artifact"
	svcOwnerID   = "770e8400-e29b-41d4-a716-446655440000"
	svcTeamID    = "550e8400-e29b-41d4-a716-446655440000"
	svcUserID    = "user-123"
)

// fakeObjectStore is an in-memory storage.ObjectStore for service tests.
type fakeObjectStore struct {
	objects   map[string][]byte
	uploadErr error
	deleted   []string
}

func newFakeObjectStore() *fakeObjectStore {
	return &fakeObjectStore{objects: make(map[string][]byte)}
}

func (f *fakeObjectStore) Upload(_ context.Context, key, _ string, r io.Reader) error {
	if f.uploadErr != nil {
		return f.uploadErr
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.objects[key] = b
	return nil
}

func (f *fakeObjectStore) Download(_ context.Context, key string) (io.ReadCloser, error) {
	b, ok := f.objects[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *fakeObjectStore) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	delete(f.objects, key)
	return nil
}

func newTestLogger() *logrus.Logger {
	l, _ := test.NewNullLogger()
	l.SetLevel(logrus.ErrorLevel)
	return l
}

func uploadParams(fileName string, content []byte) UploadAttachmentParams {
	return UploadAttachmentParams{
		TeamID:       svcTeamID,
		UserID:       svcUserID,
		OwnerType:    svcOwnerType,
		OwnerID:      svcOwnerID,
		FileName:     fileName,
		DeclaredSize: int64(len(content)),
		File:         bytes.NewReader(content),
	}
}

func TestAttachmentService_Upload_Success(t *testing.T) {
	repo := repomocks.NewMockAttachmentRepository(t)
	repo.On("SumSizeByOwner", mock.Anything, svcOwnerType, svcOwnerID).Return(int64(0), nil)
	repo.On("Create", mock.Anything, mock.AnythingOfType("*models.Attachment")).Return(nil)

	store := newFakeObjectStore()
	svc := NewAttachmentService(repo, store, newTestLogger())

	content := []byte("hello, this is a text attachment")
	att, err := svc.Upload(context.Background(), uploadParams("notes.txt", content))

	assert.NoError(t, err)
	assert.Equal(t, "text/plain", att.ContentType)
	assert.Equal(t, int64(len(content)), att.SizeBytes)
	assert.Equal(t, "notes.txt", att.FileName)
	assert.True(t, strings.HasPrefix(att.GCSObjectKey, svcTeamID+"/"+svcOwnerType+"/"+svcOwnerID+"/"))
	assert.Len(t, store.objects, 1)
}

func TestAttachmentService_Upload_DisallowedExtension(t *testing.T) {
	repo := repomocks.NewMockAttachmentRepository(t)
	svc := NewAttachmentService(repo, newFakeObjectStore(), newTestLogger())

	_, err := svc.Upload(context.Background(), uploadParams("malware.exe", []byte("MZ\x90\x00binary")))
	assert.ErrorIs(t, err, ErrAttachmentDisallowedType)
}

func TestAttachmentService_Upload_SpoofedExtension(t *testing.T) {
	repo := repomocks.NewMockAttachmentRepository(t)
	svc := NewAttachmentService(repo, newFakeObjectStore(), newTestLogger())

	// A .png whose bytes are an ELF executable header — sniffs as octet-stream,
	// not image/png, so it must be rejected.
	elf := append([]byte{0x7f, 'E', 'L', 'F'}, bytes.Repeat([]byte{0x00}, 32)...)
	_, err := svc.Upload(context.Background(), uploadParams("image.png", elf))
	assert.ErrorIs(t, err, ErrAttachmentDisallowedType)
}

func TestAttachmentService_Upload_FileTooLarge(t *testing.T) {
	repo := repomocks.NewMockAttachmentRepository(t)
	svc := NewAttachmentService(repo, newFakeObjectStore(), newTestLogger())

	params := uploadParams("big.txt", []byte("small content"))
	params.DeclaredSize = MaxAttachmentFileSize + 1
	_, err := svc.Upload(context.Background(), params)
	assert.ErrorIs(t, err, ErrAttachmentTooLarge)
}

func TestAttachmentService_Upload_TotalSizeExceeded(t *testing.T) {
	repo := repomocks.NewMockAttachmentRepository(t)
	repo.On("SumSizeByOwner", mock.Anything, svcOwnerType, svcOwnerID).
		Return(MaxAttachmentTotalSize-10, nil)
	svc := NewAttachmentService(repo, newFakeObjectStore(), newTestLogger())

	params := uploadParams("notes.txt", []byte("this content is more than ten bytes"))
	_, err := svc.Upload(context.Background(), params)
	assert.ErrorIs(t, err, ErrAttachmentTotalSizeExceeded)
}

func TestAttachmentService_Upload_StorageNotConfigured(t *testing.T) {
	repo := repomocks.NewMockAttachmentRepository(t)
	svc := NewAttachmentService(repo, nil, newTestLogger())

	_, err := svc.Upload(context.Background(), uploadParams("notes.txt", []byte("content")))
	assert.ErrorIs(t, err, ErrAttachmentStorageNotConfigured)
}

func TestAttachmentService_Upload_OversizedActualBytesRejected(t *testing.T) {
	// Declared size lies (small), but actual content exceeds the per-file limit.
	repo := repomocks.NewMockAttachmentRepository(t)
	repo.On("SumSizeByOwner", mock.Anything, svcOwnerType, svcOwnerID).Return(int64(0), nil)
	store := newFakeObjectStore()
	svc := NewAttachmentService(repo, store, newTestLogger())

	big := append([]byte("text "), bytes.Repeat([]byte("a"), int(MaxAttachmentFileSize)+10)...)
	params := uploadParams("big.txt", big)
	params.DeclaredSize = 5 // lie

	_, err := svc.Upload(context.Background(), params)
	assert.ErrorIs(t, err, ErrAttachmentTooLarge)
	// The partial object must have been cleaned up.
	assert.NotEmpty(t, store.deleted)
}

func TestAttachmentService_DeleteAllForOwner(t *testing.T) {
	repo := repomocks.NewMockAttachmentRepository(t)
	deleted := []models.Attachment{
		{ID: "a1", GCSObjectKey: "k1"},
		{ID: "a2", GCSObjectKey: "k2"},
	}
	repo.On("DeleteByOwner", mock.Anything, svcOwnerType, svcOwnerID).Return(deleted, nil)
	store := newFakeObjectStore()
	svc := NewAttachmentService(repo, store, newTestLogger())

	err := svc.DeleteAllForOwner(context.Background(), svcOwnerType, svcOwnerID)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"k1", "k2"}, store.deleted)
}

func TestAttachmentService_Delete_RemovesRowThenObject(t *testing.T) {
	repo := repomocks.NewMockAttachmentRepository(t)
	att := &models.Attachment{ID: "a1", GCSObjectKey: "team/artifact/owner/k1"}
	repo.On("GetByID", mock.Anything, svcOwnerType, svcOwnerID, "a1").Return(att, nil)
	repo.On("Delete", mock.Anything, svcOwnerType, svcOwnerID, "a1").Return(nil)
	store := newFakeObjectStore()
	svc := NewAttachmentService(repo, store, newTestLogger())

	err := svc.Delete(context.Background(), svcOwnerType, svcOwnerID, "a1")
	assert.NoError(t, err)
	assert.Equal(t, []string{"team/artifact/owner/k1"}, store.deleted)
}

func TestAttachmentService_GetByIDInTeam(t *testing.T) {
	repo := repomocks.NewMockAttachmentRepository(t)
	att := &models.Attachment{ID: "a1", TeamID: svcTeamID, OwnerType: svcOwnerType, OwnerID: svcOwnerID}
	repo.On("GetByIDInTeam", mock.Anything, svcTeamID, "a1").Return(att, nil)
	svc := NewAttachmentService(repo, newFakeObjectStore(), newTestLogger())

	got, err := svc.GetByIDInTeam(context.Background(), svcTeamID, "a1")
	assert.NoError(t, err)
	assert.Equal(t, att, got)
}

func TestBuildAttachmentObjectKey(t *testing.T) {
	key := buildAttachmentObjectKey(svcTeamID, svcOwnerType, svcOwnerID, "../../etc/passwd")
	assert.True(t, strings.HasPrefix(key, svcTeamID+"/"+svcOwnerType+"/"+svcOwnerID+"/"))
	// Path-escaping segments must be reduced to the base name.
	assert.True(t, strings.HasSuffix(key, "-passwd"))
	assert.NotContains(t, key, "..")
}
