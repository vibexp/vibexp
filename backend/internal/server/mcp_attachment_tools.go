package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// Attachment MCP tools. These expose the universal attachment subsystem
// (AttachmentService + the owner-authorizer registry from #1809) over MCP so AI
// agents can upload, list, and delete file attachments on any attachable owner.
// They are deliberately thin: every handler resolves and membership-checks the
// team via resolveTeam, authorizes the owner via the same registry the HTTP
// endpoint uses, then delegates to the already-validated service.

// attachmentOwnerDeniedText is the generic, anti-enumeration message returned
// whenever an owner_type is not registered or the owner is not accessible. It
// never reveals whether the owner exists or which owner_types are supported.
const attachmentOwnerDeniedText = "The specified attachment owner does not exist or is not accessible. " +
	"Check owner_type and owner_id, and that you have access to the owning resource."

// attachmentNotFoundText is the generic message for an attachment id that does
// not resolve within the team. Like attachmentOwnerDeniedText it leaks nothing
// about existence.
const attachmentNotFoundText = "Attachment not found or not accessible."

// UploadAttachmentParams defines the parameters for the upload_attachment tool.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type UploadAttachmentParams struct {
	TeamID            string `json:"team_id" jsonschema:"REQUIRED. The team UUID or slug to operate within. Call vibexp_io_list_teams first if you don't have one."`
	OwnerType         string `json:"owner_type" jsonschema:"The attachable resource type the file belongs to, e.g. \"artifact\". Must be a supported owner type."`
	OwnerID           string `json:"owner_id" jsonschema:"UUID of the owning resource (e.g. the artifact id) the file is attached to."`
	FileName          string `json:"file_name" jsonschema:"File name including extension, e.g. \"report.pdf\". The extension must be in the allowlist (png, jpg, jpeg, gif, webp, pdf, txt, md, csv, json, docx, xlsx, zip)."`
	FileContentBase64 string `json:"file_content_base64" jsonschema:"The file content as a standard base64-encoded string. Decoded size must not exceed 5 MB per file (10 MB total per owner)."`
}

// ListAttachmentsParams defines the parameters for the list_attachments tool.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type ListAttachmentsParams struct {
	TeamID    string `json:"team_id" jsonschema:"REQUIRED. The team UUID or slug to operate within. Call vibexp_io_list_teams first if you don't have one."`
	OwnerType string `json:"owner_type" jsonschema:"The attachable resource type to list attachments for, e.g. \"artifact\"."`
	OwnerID   string `json:"owner_id" jsonschema:"UUID of the owning resource whose attachments should be listed."`
}

// DeleteAttachmentParams defines the parameters for the delete_attachment tool.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type DeleteAttachmentParams struct {
	TeamID       string `json:"team_id" jsonschema:"REQUIRED. The team UUID or slug to operate within. Call vibexp_io_list_teams first if you don't have one."`
	AttachmentID string `json:"attachment_id" jsonschema:"UUID of the attachment to delete. The owning resource is resolved from the stored row and authorized before deletion."`
}

// attachmentUploadResponse is the slim response returned by the upload tool.
type attachmentUploadResponse struct {
	ID          string `json:"id"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	// DownloadURL is a root-relative API path (GET) the caller resolves against
	// the API origin it is already authenticated to. It is relative rather than
	// absolute because the MCP and API origins differ and there is no configured
	// public API base URL.
	DownloadURL string `json:"download_url"`
}

// attachmentListItem is the per-item shape returned by the list tool.
type attachmentListItem struct {
	ID          string `json:"id"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	CreatedAt   string `json:"created_at"`
	DownloadURL string `json:"download_url"`
}

// attachmentListResponse is the list shape returned by the list tool.
type attachmentListResponse struct {
	Attachments    []attachmentListItem `json:"attachments"`
	TotalCount     int                  `json:"total_count"`
	TotalSizeBytes int64                `json:"total_size_bytes"`
}

// attachmentDeleteResponse is the response returned by the delete tool.
type attachmentDeleteResponse struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// attachmentDownloadURL builds the root-relative API download path for an
// attachment. Both segments are path-escaped.
func attachmentDownloadURL(teamID, attachmentID string) string {
	return fmt.Sprintf("/api/v1/%s/attachments/%s",
		url.PathEscape(teamID), url.PathEscape(attachmentID))
}

// authorizeAttachmentOwnerMCP runs the owner-authorizer registry for (ownerType,
// ownerID) and returns nil on success. Both unknown owner_type and access-denied
// collapse to the same generic anti-enumeration error so the tool never reveals
// whether an owner exists or which owner types are registered. Unexpected errors
// are logged and surfaced as a generic failure.
func (s *Server) authorizeAttachmentOwnerMCP(
	ctx context.Context, tool, ownerType, userID, teamID, ownerID string,
) *mcp.CallToolResult {
	err := s.attachmentAuthorizers.Authorize(ctx, ownerType, userID, teamID, ownerID)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, services.ErrAttachmentOwnerTypeUnknown),
		errors.Is(err, services.ErrAttachmentOwnerAccessDenied):
		return mcpTextError(attachmentOwnerDeniedText)
	default:
		slog.Error(
			"Failed to authorize attachment owner via MCP",
			"tool", tool,
			"user_id", userID,
			"team_id", teamID,
			"owner_type", ownerType,
			"error", fmt.Sprintf("%+v", err),
		)
		return mcpTextError("Failed to authorize attachment owner")
	}
}

// mcpAttachmentUploadError maps AttachmentService.Upload errors to safe,
// caller-facing MCP errors, mirroring handleAttachmentUploadError. The raw error
// is never echoed back to the caller.
func mcpAttachmentUploadError(err error) *mcp.CallToolResult {
	switch {
	case errors.Is(err, services.ErrAttachmentStorageNotConfigured):
		return mcpTextError("Attachment storage is not available")
	case errors.Is(err, services.ErrAttachmentTooLarge):
		return mcpTextError("File exceeds the 5 MB per-file limit")
	case errors.Is(err, services.ErrAttachmentTotalSizeExceeded):
		return mcpTextError("Attachments exceed the 10 MB total limit for this resource")
	case errors.Is(err, services.ErrAttachmentDisallowedType):
		return mcpTextError("File type is not allowed")
	case errors.Is(err, services.ErrAttachmentEmpty):
		return mcpTextError("File is empty")
	default:
		return mcpTextError("Failed to upload attachment")
	}
}

// mcpJSONResult marshals v and returns a CallToolResult carrying both the JSON
// text content and the structured content, matching the other write tools.
func mcpJSONResult(v any) (*mcp.CallToolResult, any, error) {
	jsonData, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal response to JSON: %w", err)
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(jsonData)}},
		StructuredContent: v,
	}, v, nil
}

// uploadAttachment implements vibexp_io_upload_attachment: it stores a single
// base64-inline file against an owner in the resolved team.
//
//nolint:funlen // Resolve team, validate inputs, pre-decode guard, authorize, decode, upload, marshal.
func (s *Server) uploadAttachment(
	ctx context.Context, _ *mcp.CallToolRequest, params *UploadAttachmentParams, userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	ownerType := strings.TrimSpace(params.OwnerType)
	ownerID := strings.TrimSpace(params.OwnerID)
	fileName := strings.TrimSpace(params.FileName)
	if ownerType == "" || ownerID == "" || fileName == "" {
		return mcpTextError("owner_type, owner_id, and file_name are required"), nil, nil
	}
	if !isValidUUID(ownerID) {
		return mcpTextError("owner_id must be a valid UUID"), nil, nil
	}
	if params.FileContentBase64 == "" {
		return mcpTextError("file_content_base64 is required"), nil, nil
	}

	// Reject oversized payloads before allocating the decoded buffer. The encoded
	// length for a 5 MB file is a hard upper bound on a valid base64 string.
	maxBase64Len := base64.StdEncoding.EncodedLen(int(services.MaxAttachmentFileSize))
	if len(params.FileContentBase64) > maxBase64Len {
		return mcpTextError(fmt.Sprintf(
			"file_content_base64 is too large; the decoded file must not exceed %d bytes (5 MB)",
			services.MaxAttachmentFileSize)), nil, nil
	}

	if denied := s.authorizeAttachmentOwnerMCP(
		ctx, "vibexp_io_upload_attachment", ownerType, userID, teamID, ownerID,
	); denied != nil {
		return denied, nil, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(params.FileContentBase64)
	if err != nil {
		return mcpTextError("file_content_base64 is not valid base64"), nil, nil
	}
	if len(decoded) == 0 {
		return mcpTextError("File is empty"), nil, nil
	}

	att, err := s.container.AttachmentService().Upload(ctx, services.UploadAttachmentParams{
		TeamID:       teamID,
		UserID:       userID,
		OwnerType:    ownerType,
		OwnerID:      ownerID,
		FileName:     fileName,
		DeclaredSize: int64(len(decoded)),
		File:         bytes.NewReader(decoded),
	})
	if err != nil {
		slog.Warn(
			"Failed to upload attachment via MCP",
			"tool", "vibexp_io_upload_attachment",
			"user_id", userID,
			"team_id", teamID,
			"owner_type", ownerType,
			"error", fmt.Sprintf("%+v", err),
		)
		return mcpAttachmentUploadError(err), nil, nil
	}

	return mcpJSONResult(&attachmentUploadResponse{
		ID:          att.ID,
		FileName:    att.FileName,
		ContentType: att.ContentType,
		SizeBytes:   att.SizeBytes,
		DownloadURL: attachmentDownloadURL(teamID, att.ID),
	})
}

// listAttachments implements vibexp_io_list_attachments: it returns attachment
// metadata (plus a download URL) for an owner in the resolved team.
func (s *Server) listAttachments(
	ctx context.Context, _ *mcp.CallToolRequest, params *ListAttachmentsParams, userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	ownerType := strings.TrimSpace(params.OwnerType)
	ownerID := strings.TrimSpace(params.OwnerID)
	if ownerType == "" || ownerID == "" {
		return mcpTextError("owner_type and owner_id are required"), nil, nil
	}
	if !isValidUUID(ownerID) {
		return mcpTextError("owner_id must be a valid UUID"), nil, nil
	}

	if denied := s.authorizeAttachmentOwnerMCP(
		ctx, "vibexp_io_list_attachments", ownerType, userID, teamID, ownerID,
	); denied != nil {
		return denied, nil, nil
	}

	resp, err := s.container.AttachmentService().List(ctx, ownerType, ownerID)
	if err != nil {
		slog.Error(
			"Failed to list attachments via MCP",
			"tool", "vibexp_io_list_attachments",
			"user_id", userID,
			"team_id", teamID,
			"owner_type", ownerType,
			"error", fmt.Sprintf("%+v", err),
		)
		return mcpTextError("Failed to list attachments"), nil, nil
	}

	items := make([]attachmentListItem, 0, len(resp.Attachments))
	for i := range resp.Attachments {
		a := resp.Attachments[i]
		items = append(items, attachmentListItem{
			ID:          a.ID,
			FileName:    a.FileName,
			ContentType: a.ContentType,
			SizeBytes:   a.SizeBytes,
			CreatedAt:   a.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			DownloadURL: attachmentDownloadURL(teamID, a.ID),
		})
	}

	return mcpJSONResult(&attachmentListResponse{
		Attachments:    items,
		TotalCount:     resp.TotalCount,
		TotalSizeBytes: resp.TotalSizeBytes,
	})
}

// deleteAttachment implements vibexp_io_delete_attachment: it resolves the owner
// from the stored row, authorizes it, and deletes the attachment.
func (s *Server) deleteAttachment(
	ctx context.Context, _ *mcp.CallToolRequest, params *DeleteAttachmentParams, userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	attachmentID := strings.TrimSpace(params.AttachmentID)
	if attachmentID == "" {
		return mcpTextError("attachment_id is required"), nil, nil
	}
	if !isValidUUID(attachmentID) {
		return mcpTextError("attachment_id must be a valid UUID"), nil, nil
	}

	att, err := s.container.AttachmentService().GetByIDInTeam(ctx, teamID, attachmentID)
	if err != nil {
		if errors.Is(err, repositories.ErrAttachmentNotFound) {
			return mcpTextError(attachmentNotFoundText), nil, nil
		}
		slog.Error(
			"Failed to load attachment via MCP",
			"tool", "vibexp_io_delete_attachment",
			"user_id", userID,
			"team_id", teamID,
			"attachment_id", attachmentID,
			"error", fmt.Sprintf("%+v", err),
		)
		return mcpTextError("Failed to delete attachment"), nil, nil
	}

	if denied := s.authorizeAttachmentOwnerMCP(
		ctx, "vibexp_io_delete_attachment", att.OwnerType, userID, teamID, att.OwnerID,
	); denied != nil {
		return denied, nil, nil
	}

	if err := s.container.AttachmentService().Delete(ctx, att.OwnerType, att.OwnerID, att.ID); err != nil {
		if errors.Is(err, repositories.ErrAttachmentNotFound) {
			return mcpTextError(attachmentNotFoundText), nil, nil
		}
		slog.Error(
			"Failed to delete attachment via MCP",
			"tool", "vibexp_io_delete_attachment",
			"user_id", userID,
			"team_id", teamID,
			"attachment_id", attachmentID,
			"error", fmt.Sprintf("%+v", err),
		)
		return mcpTextError("Failed to delete attachment"), nil, nil
	}

	return mcpJSONResult(&attachmentDeleteResponse{ID: att.ID, Deleted: true})
}
