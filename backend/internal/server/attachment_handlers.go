package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// ownerTypeArtifact is the attachment owner_type for artifacts. The attachment
// subsystem is generic; the artifact-nested routes below use this constant, and
// owner types are registered in setupAttachmentAuthorizers.
const ownerTypeArtifact = "artifact"

// ownerTypePrompt is the attachment owner_type for prompts. Prompts have no
// attachment-specific handlers — they ride the universal attachments endpoint —
// so this is referenced only when registering the prompt authorizer (see
// setupAttachmentAuthorizers).
const ownerTypePrompt = "prompt"

// ownerTypeBlueprint is the attachment owner_type for blueprints. Like prompts,
// blueprints ride the universal attachments endpoint with no dedicated handlers —
// referenced only when registering the blueprint authorizer.
const ownerTypeBlueprint = "blueprint"

// resolveArtifactForAttachment decodes/validates the artifact URL params and
// resolves the artifact scoped to the validated team, so attachment routes
// enforce the same access boundary as the artifact routes they nest under. It
// writes the error response and returns ok=false on any failure.
func (s *Server) resolveArtifactForAttachment(
	w http.ResponseWriter, r *http.Request, handler string,
) (*models.Artifact, bool) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	projectID := chi.URLParam(r, "project_id")
	slug := chi.URLParam(r, "slug")

	decodedProjectID, decodedSlug, ok := s.decodeArtifactURLParams(w, userID, handler, projectID, slug)
	if !ok {
		return nil, false
	}
	if !isValidUUID(decodedProjectID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid project_id format", http.StatusBadRequest)
		return nil, false
	}

	artifact, err := s.container.ArtifactService().GetArtifactByProjectIDAndSlugInTeam(
		userID, teamID, decodedProjectID, decodedSlug,
	)
	if err != nil {
		s.handleGetArtifactError(w, userID, decodedProjectID, decodedSlug, err)
		return nil, false
	}
	return artifact, true
}

func (s *Server) handleUploadArtifactAttachment(w http.ResponseWriter, r *http.Request) {
	artifact, ok := s.resolveArtifactForAttachment(w, r, "handleUploadArtifactAttachment")
	if !ok {
		return
	}
	userID := r.Context().Value(contextKeyUserID).(string)

	file, header, err := r.FormFile("file")
	if err != nil {
		writeErrorResponse(w, nil, "bad_request", "A file is required (multipart field 'file')", http.StatusBadRequest)
		return
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			s.logger.With("error", cerr).Warn("Failed to close uploaded file")
		}
	}()

	att, err := s.container.AttachmentService().Upload(r.Context(), services.UploadAttachmentParams{
		TeamID:       artifact.TeamID,
		UserID:       userID,
		OwnerType:    ownerTypeArtifact,
		OwnerID:      artifact.ID,
		FileName:     header.Filename,
		DeclaredSize: header.Size,
		File:         file,
	})
	if err != nil {
		s.handleAttachmentUploadError(w, userID, artifact.ID, header.Filename, err)
		return
	}

	writeCreated(w, att, s.logger)
}

func (s *Server) handleListArtifactAttachments(w http.ResponseWriter, r *http.Request) {
	artifact, ok := s.resolveArtifactForAttachment(w, r, "handleListArtifactAttachments")
	if !ok {
		return
	}

	resp, err := s.container.AttachmentService().List(r.Context(), ownerTypeArtifact, artifact.ID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListArtifactAttachments",
			"artifact_id", artifact.ID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list attachments")
		writeErrorResponse(w, nil, "internal_error", "Failed to list attachments", http.StatusInternalServerError)
		return
	}
	writeOK(w, resp, s.logger)
}

func (s *Server) handleDownloadArtifactAttachment(w http.ResponseWriter, r *http.Request) {
	artifact, ok := s.resolveArtifactForAttachment(w, r, "handleDownloadArtifactAttachment")
	if !ok {
		return
	}
	attachmentID := chi.URLParam(r, "id")
	if !isValidUUID(attachmentID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid attachment id format", http.StatusBadRequest)
		return
	}

	att, err := s.container.AttachmentService().Get(r.Context(), ownerTypeArtifact, artifact.ID, attachmentID)
	if err != nil {
		s.handleAttachmentGetError(w, attachmentID, err)
		return
	}

	rc, err := s.container.AttachmentService().Download(r.Context(), att)
	if err != nil {
		s.handleAttachmentDownloadError(w, attachmentID, err)
		return
	}
	defer func() {
		if cerr := rc.Close(); cerr != nil {
			s.logger.With("error", cerr).Warn("Failed to close attachment reader")
		}
	}()
	s.streamAttachment(w, att, rc, "handleDownloadArtifactAttachment")
}

func (s *Server) handleDeleteArtifactAttachment(w http.ResponseWriter, r *http.Request) {
	artifact, ok := s.resolveArtifactForAttachment(w, r, "handleDeleteArtifactAttachment")
	if !ok {
		return
	}
	attachmentID := chi.URLParam(r, "id")
	if !isValidUUID(attachmentID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid attachment id format", http.StatusBadRequest)
		return
	}

	err := s.container.AttachmentService().Delete(r.Context(), ownerTypeArtifact, artifact.ID, attachmentID)
	if err != nil {
		s.handleAttachmentGetError(w, attachmentID, err)
		return
	}
	writeNoContent(w)
}

// deleteArtifactAttachments removes all attachments for an artifact during the
// artifact-delete path. Non-fatal and nil-safe (the service may be absent in
// unit tests), mirroring deleteArtifactEmbeddings.
func (s *Server) deleteArtifactAttachments(userID, artifactID string) {
	svc := s.container.AttachmentService()
	if svc == nil {
		return
	}
	if err := svc.DeleteAllForOwner(context.Background(), ownerTypeArtifact, artifactID); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleDeleteArtifact",
			"user_id", userID,
			"artifact_id", artifactID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to delete artifact attachments (non-fatal)")
	}
}

// handleAttachmentUploadError maps attachment upload errors to HTTP responses.
func (s *Server) handleAttachmentUploadError(w http.ResponseWriter, userID, artifactID, fileName string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUploadArtifactAttachment",
		"user_id", userID,
		"artifact_id", artifactID,
		"file_name", fileName,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to upload attachment")

	switch {
	case errors.Is(err, services.ErrAttachmentStorageNotConfigured):
		writeErrorResponse(w, nil, "service_unavailable",
			"Attachment storage is not available", http.StatusServiceUnavailable)
	case errors.Is(err, services.ErrAttachmentTooLarge):
		writeErrorResponse(w, nil, "bad_request", "File exceeds the 5 MB per-file limit", http.StatusBadRequest)
	case errors.Is(err, services.ErrAttachmentTotalSizeExceeded):
		writeErrorResponse(w, nil, "bad_request",
			"Attachments exceed the 10 MB total limit for this artifact", http.StatusBadRequest)
	case errors.Is(err, services.ErrAttachmentDisallowedType):
		writeErrorResponse(w, nil, "bad_request", "File type is not allowed", http.StatusBadRequest)
	case errors.Is(err, services.ErrAttachmentEmpty):
		writeErrorResponse(w, nil, "bad_request", "File is empty", http.StatusBadRequest)
	default:
		writeErrorResponse(w, nil, "internal_error", "Failed to upload attachment", http.StatusInternalServerError)
	}
}

// handleAttachmentGetError maps not-found vs internal errors for get/delete.
func (s *Server) handleAttachmentGetError(w http.ResponseWriter, attachmentID string, err error) {
	if errors.Is(err, repositories.ErrAttachmentNotFound) {
		writeErrorResponse(w, nil, "not_found", "Attachment not found", http.StatusNotFound)
		return
	}
	s.logger.With(
		"service", "vibexp-api",
		"handler", "attachment",
		"attachment_id", attachmentID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to access attachment")
	writeErrorResponse(w, nil, "internal_error", "Failed to access attachment", http.StatusInternalServerError)
}

// handleAttachmentDownloadError maps download errors (storage unavailable vs internal).
func (s *Server) handleAttachmentDownloadError(w http.ResponseWriter, attachmentID string, err error) {
	if errors.Is(err, services.ErrAttachmentStorageNotConfigured) {
		writeErrorResponse(w, nil, "service_unavailable",
			"Attachment storage is not available", http.StatusServiceUnavailable)
		return
	}
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleDownloadArtifactAttachment",
		"attachment_id", attachmentID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to open attachment for download")
	writeErrorResponse(w, nil, "internal_error", "Failed to download attachment", http.StatusInternalServerError)
}

// sanitizeFilename strips characters that could break the Content-Disposition
// header (quotes, control chars, path separators).
func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer("\"", "", "\\", "", "\r", "", "\n", "", "/", "_")
	return replacer.Replace(name)
}
