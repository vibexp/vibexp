package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/storage"
)

// Attachment size limits. Per the issue: max 5 MB per file, max 10 MB total
// across all attachments of a single owner.
const (
	MaxAttachmentFileSize  int64 = 5 * 1024 * 1024
	MaxAttachmentTotalSize int64 = 10 * 1024 * 1024
	contentSniffLen              = 512
)

// Attachment service errors. Handlers map these to specific HTTP statuses.
var (
	// ErrAttachmentStorageNotConfigured is returned when no object store is
	// available (bucket unset, or client init failed in a credential-less env).
	ErrAttachmentStorageNotConfigured = errors.New("attachment storage is not configured")
	// ErrAttachmentTooLarge is returned when a single file exceeds the per-file limit.
	ErrAttachmentTooLarge = errors.New("file exceeds the 5 MB per-file limit")
	// ErrAttachmentTotalSizeExceeded is returned when the cumulative size for an
	// owner would exceed the per-owner total limit.
	ErrAttachmentTotalSizeExceeded = errors.New("attachments exceed the 10 MB total limit for this resource")
	// ErrAttachmentDisallowedType is returned when the file extension or sniffed
	// content type is not in the safe allowlist.
	ErrAttachmentDisallowedType = errors.New("file type is not allowed")
	// ErrAttachmentEmpty is returned when an uploaded file has no content.
	ErrAttachmentEmpty = errors.New("file is empty")
)

// allowedAttachmentType pairs the canonical stored content type for an
// extension with the set of content types http.DetectContentType may report
// for genuine files of that extension. The sniff check rejects a file whose
// real bytes don't match its claimed extension (e.g. an .exe renamed .png).
type allowedAttachmentType struct {
	contentType string
	sniff       []string
}

// allowedAttachmentTypes is the safe-type allowlist. Executables/scripts
// (.exe, .sh, .bat, .js, …) are intentionally absent and therefore rejected.
// SVG is intentionally absent (XSS risk); downloads are always served as
// attachments regardless.
var allowedAttachmentTypes = map[string]allowedAttachmentType{
	".png":  {contentType: "image/png", sniff: []string{"image/png"}},
	".jpg":  {contentType: "image/jpeg", sniff: []string{"image/jpeg"}},
	".jpeg": {contentType: "image/jpeg", sniff: []string{"image/jpeg"}},
	".gif":  {contentType: "image/gif", sniff: []string{"image/gif"}},
	".webp": {contentType: "image/webp", sniff: []string{"image/webp"}},
	".pdf":  {contentType: "application/pdf", sniff: []string{"application/pdf"}},
	".txt":  {contentType: "text/plain", sniff: []string{"text/plain"}},
	".md":   {contentType: "text/markdown", sniff: []string{"text/plain"}},
	".csv":  {contentType: "text/csv", sniff: []string{"text/plain", "text/csv"}},
	".json": {contentType: "application/json", sniff: []string{"text/plain"}},
	".docx": {
		contentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		sniff:       []string{"application/zip"},
	},
	".xlsx": {
		contentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		sniff:       []string{"application/zip"},
	},
	".zip": {contentType: "application/zip", sniff: []string{"application/zip"}},
}

// UploadAttachmentParams carries everything needed to store one file for an
// owner. File must support Seek so the service can sniff the content type and
// then rewind for the upload.
type UploadAttachmentParams struct {
	TeamID       string
	UserID       string
	OwnerType    string
	OwnerID      string
	FileName     string
	DeclaredSize int64 // best-effort size from the multipart header for pre-checks
	File         io.ReadSeeker
}

// AttachmentService implements AttachmentServiceInterface. It is fully generic:
// owner type is always a parameter, never hardcoded.
type AttachmentService struct {
	repo   repositories.AttachmentRepository
	store  storage.ObjectStore
	logger *logrus.Logger
}

// NewAttachmentService creates a new AttachmentService. store may be nil when
// object storage is unavailable; operations then return
// ErrAttachmentStorageNotConfigured rather than panicking.
func NewAttachmentService(
	repo repositories.AttachmentRepository,
	store storage.ObjectStore,
	logger *logrus.Logger,
) *AttachmentService {
	return &AttachmentService{repo: repo, store: store, logger: logger}
}

// countingReader wraps a reader and tracks the total number of bytes read, so
// the upload can enforce the real per-file limit even if the declared size lies.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func (s *AttachmentService) Upload(
	ctx context.Context, params UploadAttachmentParams,
) (*models.Attachment, error) {
	if s.store == nil {
		return nil, ErrAttachmentStorageNotConfigured
	}

	contentType, err := s.validateFileType(params.FileName, params.File)
	if err != nil {
		return nil, err
	}

	if params.DeclaredSize > MaxAttachmentFileSize {
		return nil, ErrAttachmentTooLarge
	}

	existing, err := s.repo.SumSizeByOwner(ctx, params.OwnerType, params.OwnerID)
	if err != nil {
		return nil, fmt.Errorf("failed to compute existing attachment size: %w", err)
	}
	if existing+params.DeclaredSize > MaxAttachmentTotalSize {
		return nil, ErrAttachmentTotalSizeExceeded
	}

	objectKey := buildAttachmentObjectKey(params.TeamID, params.OwnerType, params.OwnerID, params.FileName)

	// Bound the copy at one byte over the limit so we can detect (and reject) a
	// file whose declared size understated its true size.
	counter := &countingReader{r: io.LimitReader(params.File, MaxAttachmentFileSize+1)}
	if uploadErr := s.store.Upload(ctx, objectKey, contentType, counter); uploadErr != nil {
		return nil, fmt.Errorf("failed to upload attachment: %w", uploadErr)
	}

	if invalid := s.rejectInvalidSize(ctx, objectKey, counter.n, existing); invalid != nil {
		return nil, invalid
	}

	att := &models.Attachment{
		TeamID:       params.TeamID,
		UserID:       params.UserID,
		OwnerType:    params.OwnerType,
		OwnerID:      params.OwnerID,
		FileName:     filepath.Base(params.FileName),
		ContentType:  contentType,
		SizeBytes:    counter.n,
		GCSObjectKey: objectKey,
	}
	if createErr := s.repo.Create(ctx, att); createErr != nil {
		// Compensating delete so a failed metadata write doesn't orphan the object.
		s.bestEffortDelete(ctx, objectKey)
		return nil, fmt.Errorf("failed to persist attachment metadata: %w", createErr)
	}
	return att, nil
}

// rejectInvalidSize deletes the just-uploaded object and returns the matching
// error when the actual byte count is empty, over the per-file limit, or pushes
// the owner over the cumulative limit; returns nil when the size is acceptable.
func (s *AttachmentService) rejectInvalidSize(ctx context.Context, objectKey string, actual, existing int64) error {
	switch {
	case actual == 0:
		s.bestEffortDelete(ctx, objectKey)
		return ErrAttachmentEmpty
	case actual > MaxAttachmentFileSize:
		s.bestEffortDelete(ctx, objectKey)
		return ErrAttachmentTooLarge
	case existing+actual > MaxAttachmentTotalSize:
		s.bestEffortDelete(ctx, objectKey)
		return ErrAttachmentTotalSizeExceeded
	default:
		return nil
	}
}

func (s *AttachmentService) List(
	ctx context.Context, ownerType, ownerID string,
) (*models.AttachmentListResponse, error) {
	attachments, err := s.repo.ListByOwner(ctx, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	var total int64
	for i := range attachments {
		total += attachments[i].SizeBytes
	}
	return &models.AttachmentListResponse{
		Attachments:    attachments,
		TotalCount:     len(attachments),
		TotalSizeBytes: total,
	}, nil
}

func (s *AttachmentService) Get(
	ctx context.Context, ownerType, ownerID, attachmentID string,
) (*models.Attachment, error) {
	return s.repo.GetByID(ctx, ownerType, ownerID, attachmentID)
}

func (s *AttachmentService) GetByIDInTeam(
	ctx context.Context, teamID, attachmentID string,
) (*models.Attachment, error) {
	return s.repo.GetByIDInTeam(ctx, teamID, attachmentID)
}

func (s *AttachmentService) Download(
	ctx context.Context, attachment *models.Attachment,
) (io.ReadCloser, error) {
	if s.store == nil {
		return nil, ErrAttachmentStorageNotConfigured
	}
	return s.store.Download(ctx, attachment.GCSObjectKey)
}

func (s *AttachmentService) Delete(ctx context.Context, ownerType, ownerID, attachmentID string) error {
	att, err := s.repo.GetByID(ctx, ownerType, ownerID, attachmentID)
	if err != nil {
		return err
	}
	// Delete the metadata row first; a failed object delete then only leaves an
	// orphaned object (a storage leak handled by a future sweep), never a row
	// pointing at a missing object (which would 404 on download).
	if delErr := s.repo.Delete(ctx, ownerType, ownerID, attachmentID); delErr != nil {
		return delErr
	}
	s.bestEffortDelete(ctx, att.GCSObjectKey)
	return nil
}

func (s *AttachmentService) DeleteAllForOwner(ctx context.Context, ownerType, ownerID string) error {
	deleted, err := s.repo.DeleteByOwner(ctx, ownerType, ownerID)
	if err != nil {
		return err
	}
	for i := range deleted {
		s.bestEffortDelete(ctx, deleted[i].GCSObjectKey)
	}
	return nil
}

// bestEffortDelete removes an object from storage, logging (not failing) on
// error. Used for compensating deletes and post-row-delete cleanup.
func (s *AttachmentService) bestEffortDelete(ctx context.Context, objectKey string) {
	if s.store == nil {
		return
	}
	if err := s.store.Delete(ctx, objectKey); err != nil {
		s.logger.WithFields(logrus.Fields{
			"service":    "attachment",
			"object_key": objectKey,
			"error":      fmt.Sprintf("%+v", err),
		}).Warn("Failed to delete attachment object (orphaned, non-fatal)")
	}
}

// validateFileType enforces the safe-type allowlist by both extension and
// content sniff, then rewinds the reader for the subsequent upload. It returns
// the canonical content type to store for the file.
func (s *AttachmentService) validateFileType(fileName string, file io.ReadSeeker) (string, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	allowed, ok := allowedAttachmentTypes[ext]
	if !ok {
		return "", ErrAttachmentDisallowedType
	}

	head := make([]byte, contentSniffLen)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", fmt.Errorf("failed to read file header: %w", err)
	}
	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		return "", fmt.Errorf("failed to rewind file: %w", seekErr)
	}

	sniffed := strings.TrimSpace(strings.SplitN(http.DetectContentType(head[:n]), ";", 2)[0])
	for _, candidate := range allowed.sniff {
		if sniffed == candidate {
			return allowed.contentType, nil
		}
	}
	return "", ErrAttachmentDisallowedType
}

// buildAttachmentObjectKey builds the GCS object key
// {team_id}/{owner_type}/{owner_id}/{uuid}-{filename}. The filename is reduced
// to its base name so path separators can't escape the owner prefix.
func buildAttachmentObjectKey(teamID, ownerType, ownerID, fileName string) string {
	clean := strings.ReplaceAll(filepath.Base(fileName), "/", "_")
	return fmt.Sprintf("%s/%s/%s/%s-%s", teamID, ownerType, ownerID, uuid.NewString(), clean)
}
