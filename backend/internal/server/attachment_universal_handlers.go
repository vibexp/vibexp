package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// Universal attachments handlers (/api/v1/{team_id}/attachments). owner_type and
// owner_id are supplied by the caller for collection operations (upload body, list
// query) and read from the stored row for item operations (download/delete by id).
// Authorization for every operation runs through the owner-authorizer registry, so
// the endpoint enforces the owning resource's existing access boundary and only
// exposes owner types that have a registered authorizer.

// authorizeAttachmentOwner runs the registered owner authorizer for (ownerType,
// ownerID) and writes the matching error response, returning ok=false on denial.
// Unknown owner_type and access-denied both surface as 404 so the endpoint never
// reveals whether an owner exists.
func (s *Server) authorizeAttachmentOwner(
	w http.ResponseWriter, r *http.Request, ownerType, ownerID string,
) bool {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	err := s.attachmentAuthorizers.Authorize(r.Context(), ownerType, userID, teamID, ownerID)
	switch {
	case err == nil:
		return true
	case errors.Is(err, services.ErrAttachmentOwnerTypeUnknown):
		writeErrorResponse(w, nil, "not_found", "Unsupported attachment owner_type", http.StatusNotFound)
	case errors.Is(err, services.ErrAttachmentOwnerAccessDenied):
		writeErrorResponse(w, nil, "not_found", "Attachment owner not found", http.StatusNotFound)
	default:
		s.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"handler":    "authorizeAttachmentOwner",
			"owner_type": ownerType,
			"owner_id":   ownerID,
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to authorize attachment owner")
		writeErrorResponse(w, nil, "internal_error", "Failed to authorize attachment owner", http.StatusInternalServerError)
	}
	return false
}

func (s *Server) handleUploadAttachment(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")

	// FormFile parses the (body-size-capped) multipart form once; owner fields are
	// then read from the parsed form rather than via FormValue, which would re-parse
	// without an explicit size bound.
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErrorResponse(w, nil, "bad_request", "A file is required (multipart field 'file')", http.StatusBadRequest)
		return
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			s.logger.WithError(cerr).Warn("Failed to close uploaded file")
		}
	}()

	ownerType := multipartValue(r, "owner_type")
	ownerID := multipartValue(r, "owner_id")
	if ownerType == "" || ownerID == "" {
		writeErrorResponse(w, nil, "bad_request",
			"owner_type and owner_id form fields are required", http.StatusBadRequest)
		return
	}
	// owner_id is a UUID column; reject malformed ids before hitting the authorizer
	// so a bad value is a clear 400 rather than a downstream lookup error.
	if !isValidUUID(ownerID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid owner_id format", http.StatusBadRequest)
		return
	}
	if !s.authorizeAttachmentOwner(w, r, ownerType, ownerID) {
		return
	}

	att, err := s.container.AttachmentService().Upload(r.Context(), services.UploadAttachmentParams{
		TeamID:       teamID,
		UserID:       userID,
		OwnerType:    ownerType,
		OwnerID:      ownerID,
		FileName:     header.Filename,
		DeclaredSize: header.Size,
		File:         file,
	})
	if err != nil {
		s.handleAttachmentUploadError(w, userID, ownerID, header.Filename, err)
		return
	}
	writeCreated(w, att, s.logger)
}

// multipartValue returns the first trimmed value for key from the already-parsed
// multipart form (populated by a prior FormFile call). Reading from the parsed form
// avoids r.FormValue, which re-parses the body without an explicit size bound.
func multipartValue(r *http.Request, key string) string {
	if r.MultipartForm == nil {
		return ""
	}
	values := r.MultipartForm.Value[key]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func (s *Server) handleListAttachments(w http.ResponseWriter, r *http.Request) {
	ownerType := strings.TrimSpace(r.URL.Query().Get("owner_type"))
	ownerID := strings.TrimSpace(r.URL.Query().Get("owner_id"))
	if ownerType == "" || ownerID == "" {
		writeErrorResponse(w, nil, "bad_request",
			"owner_type and owner_id query parameters are required", http.StatusBadRequest)
		return
	}
	if !isValidUUID(ownerID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid owner_id format", http.StatusBadRequest)
		return
	}
	if !s.authorizeAttachmentOwner(w, r, ownerType, ownerID) {
		return
	}

	resp, err := s.container.AttachmentService().List(r.Context(), ownerType, ownerID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"handler":    "handleListAttachments",
			"owner_type": ownerType,
			"owner_id":   ownerID,
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to list attachments")
		writeErrorResponse(w, nil, "internal_error", "Failed to list attachments", http.StatusInternalServerError)
		return
	}
	writeOK(w, resp, s.logger)
}

func (s *Server) handleDownloadAttachment(w http.ResponseWriter, r *http.Request) {
	att, ok := s.resolveAttachmentForItemOp(w, r)
	if !ok {
		return
	}

	rc, err := s.container.AttachmentService().Download(r.Context(), att)
	if err != nil {
		s.handleAttachmentDownloadError(w, att.ID, err)
		return
	}
	defer func() {
		if cerr := rc.Close(); cerr != nil {
			s.logger.WithError(cerr).Warn("Failed to close attachment reader")
		}
	}()
	s.streamAttachment(w, att, rc, "handleDownloadAttachment")
}

func (s *Server) handleDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	att, ok := s.resolveAttachmentForItemOp(w, r)
	if !ok {
		return
	}

	if err := s.container.AttachmentService().Delete(r.Context(), att.OwnerType, att.OwnerID, att.ID); err != nil {
		s.handleAttachmentGetError(w, att.ID, err)
		return
	}
	writeNoContent(w)
}

// resolveAttachmentForItemOp loads the attachment for an id-keyed item operation
// (download/delete) scoped to the validated team, then authorizes against its
// stored owner via the registry. It writes the error response and returns ok=false
// on any failure (bad id, not found, or access denied).
func (s *Server) resolveAttachmentForItemOp(
	w http.ResponseWriter, r *http.Request,
) (*models.Attachment, bool) {
	attachmentID := chi.URLParam(r, "id")
	if !isValidUUID(attachmentID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid attachment id format", http.StatusBadRequest)
		return nil, false
	}
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	att, err := s.container.AttachmentService().GetByIDInTeam(r.Context(), teamID, attachmentID)
	if err != nil {
		s.handleAttachmentGetError(w, attachmentID, err)
		return nil, false
	}
	if !s.authorizeAttachmentOwner(w, r, att.OwnerType, att.OwnerID) {
		return nil, false
	}
	return att, true
}

// streamAttachment writes the standard download headers and copies the body. It is
// shared by the universal and (deprecated) artifact-nested download handlers so the
// security headers stay identical across both. Always served as an opaque download
// (never inline) so no attachment can be rendered in the browser — the SVG/HTML XSS
// mitigation.
func (s *Server) streamAttachment(
	w http.ResponseWriter, att *models.Attachment, rc io.Reader, handler string,
) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", sanitizeFilename(att.FileName)))
	w.Header().Set("Content-Length", strconv.FormatInt(att.SizeBytes, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, copyErr := io.Copy(w, rc); copyErr != nil {
		// Headers are already sent; log and stop. Cannot change the status now.
		s.logger.WithFields(logrus.Fields{
			"service":       "vibexp-api",
			"handler":       handler,
			"attachment_id": att.ID,
			"error":         fmt.Sprintf("%+v", copyErr),
		}).Error("Failed to stream attachment body")
	}
}
