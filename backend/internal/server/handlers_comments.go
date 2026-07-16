package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	commentsgen "github.com/vibexp/vibexp/internal/server/gen/comments"
	"github.com/vibexp/vibexp/internal/services"
)

// commentsStrictServer implements commentsgen.StrictServerInterface (epic #272):
// the resource-comments CRUD + recent-activity operations are served through
// oapi-codegen strict-server bindings generated from openapi.yaml, so a
// spec/handler payload mismatch is a compile error. Comments are REST-only —
// there is deliberately no MCP surface. Handlers are thin: they delegate to the
// #273 CommentService and map its errors to RFC 9457 problem+json.
type commentsStrictServer struct {
	s *Server
}

var _ commentsgen.StrictServerInterface = (*commentsStrictServer)(nil)

// ListComments handles GET /api/v1/{team_id}/comments
func (c *commentsStrictServer) ListComments(
	ctx context.Context, request commentsgen.ListCommentsRequestObject,
) (commentsgen.ListCommentsResponseObject, error) {
	teamID := request.TeamId.String()
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}

	page, limit := derefPageLimit(request.Params.Page, request.Params.Limit)

	resp, err := c.s.container.CommentService().ListByResource(
		ctx, userID, teamID, request.Params.ResourceType, request.Params.ResourceId.String(), page, limit,
	)
	if err != nil {
		return nil, c.commentError(ctx, "ListComments", teamID, err)
	}

	genResp, convErr := toGenCommentListResponse(resp)
	if convErr != nil {
		return nil, c.commentConversionError("ListComments", teamID, convErr)
	}
	return commentsgen.ListComments200JSONResponse(genResp), nil
}

// CreateComment handles POST /api/v1/{team_id}/comments
func (c *commentsStrictServer) CreateComment(
	ctx context.Context, request commentsgen.CreateCommentRequestObject,
) (commentsgen.CreateCommentResponseObject, error) {
	teamID := request.TeamId.String()
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("request body is required")
	}

	created, err := c.s.container.CommentService().Create(ctx, userID, teamID, &models.CreateCommentRequest{
		ResourceType: request.Body.ResourceType,
		ResourceID:   request.Body.ResourceId.String(),
		Content:      request.Body.Content,
	})
	if err != nil {
		return nil, c.commentError(ctx, "CreateComment", teamID, err)
	}

	genComment, convErr := toGenComment(*created)
	if convErr != nil {
		return nil, c.commentConversionError("CreateComment", teamID, convErr)
	}
	return commentsgen.CreateComment201JSONResponse(genComment), nil
}

// ListRecentComments handles GET /api/v1/{team_id}/comments/recent
func (c *commentsStrictServer) ListRecentComments(
	ctx context.Context, request commentsgen.ListRecentCommentsRequestObject,
) (commentsgen.ListRecentCommentsResponseObject, error) {
	teamID := request.TeamId.String()
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}

	limit := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}

	activities, err := c.s.container.CommentService().ListRecentByTeam(ctx, userID, teamID, limit)
	if err != nil {
		return nil, c.commentError(ctx, "ListRecentComments", teamID, err)
	}

	rows, convErr := toGenRecentComments(activities)
	if convErr != nil {
		return nil, c.commentConversionError("ListRecentComments", teamID, convErr)
	}
	return commentsgen.ListRecentComments200JSONResponse(commentsgen.RecentCommentListResponse{
		Comments:   rows,
		TotalCount: len(rows),
	}), nil
}

// UpdateComment handles PATCH /api/v1/{team_id}/comments/{comment_id}
func (c *commentsStrictServer) UpdateComment(
	ctx context.Context, request commentsgen.UpdateCommentRequestObject,
) (commentsgen.UpdateCommentResponseObject, error) {
	teamID := request.TeamId.String()
	commentID := request.CommentId.String()
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("request body is required")
	}

	updated, err := c.s.container.CommentService().Update(
		ctx, userID, teamID, commentID, &models.UpdateCommentRequest{Content: request.Body.Content},
	)
	if err != nil {
		return nil, c.commentError(ctx, "UpdateComment", teamID, err)
	}

	genComment, convErr := toGenComment(*updated)
	if convErr != nil {
		return nil, c.commentConversionError("UpdateComment", teamID, convErr)
	}
	return commentsgen.UpdateComment200JSONResponse(genComment), nil
}

// DeleteComment handles DELETE /api/v1/{team_id}/comments/{comment_id}
func (c *commentsStrictServer) DeleteComment(
	ctx context.Context, request commentsgen.DeleteCommentRequestObject,
) (commentsgen.DeleteCommentResponseObject, error) {
	teamID := request.TeamId.String()
	commentID := request.CommentId.String()
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}

	if delErr := c.s.container.CommentService().Delete(ctx, userID, teamID, commentID); delErr != nil {
		return nil, c.commentError(ctx, "DeleteComment", teamID, delErr)
	}
	return commentsgen.DeleteComment204Response{}, nil
}

// derefPageLimit unwraps the optional page/limit query params; zero values fall
// through to the service's clamping defaults (page 1, limit 20, max 100).
func derefPageLimit(page, limit *int) (int, int) {
	var p, l int
	if page != nil {
		p = *page
	}
	if limit != nil {
		l = *limit
	}
	return p, l
}

// commentError maps a CommentService error to an RFC 9457 APIError. Recognized
// domain errors become 400/403/404; anything else is logged and surfaced as 500.
func (c *commentsStrictServer) commentError(
	_ context.Context, handler, teamID string, err error,
) *apierrors.APIError {
	if apiErr := mapCommentServiceError(err); apiErr != nil {
		return apiErr
	}
	c.s.logger.With(
		"handler", handler,
		"team_id", teamID,
		"error", err.Error(),
	).Error("Comment operation failed")
	return apierrors.NewInternalError("Internal server error")
}

func (c *commentsStrictServer) commentConversionError(handler, teamID string, err error) *apierrors.APIError {
	c.s.logger.With(
		"handler", handler,
		"team_id", teamID,
		"error", err.Error(),
	).Error("Failed to convert comment to spec type")
	return apierrors.NewInternalError("Internal server error")
}

// mapCommentServiceError translates the CommentService's errors (a mix of
// sentinels and validation strings) into problem+json APIErrors. It returns nil
// for an unrecognized error so the caller maps it to a 500.
func mapCommentServiceError(err error) *apierrors.APIError {
	switch {
	case errors.Is(err, services.ErrPermissionDenied):
		return apierrors.NewForbiddenError("You do not have permission to perform this action")
	case errors.Is(err, repositories.ErrCommentNotFound):
		return apierrors.NewResourceNotFoundError("comment", "Comment not found")
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "resource not found"):
		return apierrors.NewResourceNotFoundError("resource", "Target resource not found")
	case strings.Contains(msg, "user is not a member"):
		return apierrors.NewForbiddenError("You are not a member of this team")
	case strings.Contains(msg, "content is required"),
		strings.Contains(msg, "content exceeds"),
		strings.Contains(msg, "invalid resource type"):
		return apierrors.NewBadRequestError(msg)
	default:
		return nil
	}
}

func toGenComment(item models.Comment) (commentsgen.Comment, error) {
	id, err := uuid.Parse(item.ID)
	if err != nil {
		return commentsgen.Comment{}, fmt.Errorf("comment id %q is not a UUID: %w", item.ID, err)
	}
	teamID, err := uuid.Parse(item.TeamID)
	if err != nil {
		return commentsgen.Comment{}, fmt.Errorf("comment team_id %q is not a UUID: %w", item.TeamID, err)
	}
	resourceID, err := uuid.Parse(item.ResourceID)
	if err != nil {
		return commentsgen.Comment{}, fmt.Errorf("comment resource_id %q is not a UUID: %w", item.ResourceID, err)
	}
	userID, err := uuid.Parse(item.UserID)
	if err != nil {
		return commentsgen.Comment{}, fmt.Errorf("comment user_id %q is not a UUID: %w", item.UserID, err)
	}

	return commentsgen.Comment{
		Id:           id,
		TeamId:       teamID,
		ResourceType: item.ResourceType,
		ResourceId:   resourceID,
		UserId:       userID,
		Content:      item.Content,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}, nil
}

func toGenCommentListResponse(resp *models.CommentListResponse) (commentsgen.CommentListResponse, error) {
	// make(...,0) guarantees the required `comments` array serializes as [] not null.
	comments := make([]commentsgen.Comment, 0, len(resp.Comments))
	for i := range resp.Comments {
		g, err := toGenComment(resp.Comments[i])
		if err != nil {
			return commentsgen.CommentListResponse{}, err
		}
		comments = append(comments, g)
	}
	return commentsgen.CommentListResponse{
		Comments:   comments,
		TotalCount: resp.TotalCount,
		Page:       resp.Page,
		PerPage:    resp.PerPage,
		TotalPages: resp.TotalPages,
	}, nil
}

func toGenRecentComments(items []models.CommentActivity) ([]commentsgen.RecentComment, error) {
	// make(...,0) guarantees the required `comments` array serializes as [] not null.
	out := make([]commentsgen.RecentComment, 0, len(items))
	for i := range items {
		g, err := toGenRecentComment(items[i])
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func toGenRecentComment(item models.CommentActivity) (commentsgen.RecentComment, error) {
	resourceID, err := uuid.Parse(item.ResourceID)
	if err != nil {
		return commentsgen.RecentComment{},
			fmt.Errorf("recent comment resource_id %q is not a UUID: %w", item.ResourceID, err)
	}
	userID, err := uuid.Parse(item.UserID)
	if err != nil {
		return commentsgen.RecentComment{}, fmt.Errorf("recent comment user_id %q is not a UUID: %w", item.UserID, err)
	}

	row := commentsgen.RecentComment{
		UserId:        userID,
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
		ResourceType:  item.ResourceType,
		ResourceId:    resourceID,
		ResourceTitle: item.ResourceTitle,
		Slug:          item.Slug,
	}
	if item.ProjectID != nil {
		projectID, parseErr := uuid.Parse(*item.ProjectID)
		if parseErr != nil {
			return commentsgen.RecentComment{},
				fmt.Errorf("recent comment project_id %q is not a UUID: %w", *item.ProjectID, parseErr)
		}
		row.ProjectId = &projectID
	}
	return row, nil
}

// commentsBindErrorHandler translates parameter-binding failures from the
// generated layer into this domain's RFC 9457 400 responses.
func (s *Server) commentsBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()

	var invalidParam *commentsgen.InvalidParamFormatError
	if errors.As(err, &invalidParam) {
		switch invalidParam.ParamName {
		case "team_id":
			msg = "team_id must be a valid UUID"
		case "comment_id":
			msg = "comment_id must be a valid UUID"
		case "resource_id":
			msg = "resource_id must be a valid UUID"
		case "resource_type":
			msg = "resource_type is required"
		case "page", "limit":
			msg = invalidParam.ParamName + " must be an integer"
		}
	}

	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msg))
}

// commentsResponseErrorHandler writes errors returned by the strict handler
// implementations. *apierrors.APIError carries the intended RFC 9457 error;
// anything else is defensive and maps to a generic 500.
func (s *Server) commentsResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With("error", err).Error("Comments strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Internal server error"))
}
