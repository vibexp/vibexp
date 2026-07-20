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
	relationsgen "github.com/vibexp/vibexp/internal/server/gen/relations"
	"github.com/vibexp/vibexp/internal/services"
)

// relationsMsgInternalError is the generic problem detail for unexpected
// relation-service failures.
const relationsMsgInternalError = "Internal server error"

// relationsStrictServer implements relationsgen.StrictServerInterface (epic
// #421): the typed-resource-relations create/list/confirm/delete operations are
// served through oapi-codegen strict-server bindings generated from
// openapi.yaml, so a spec/handler payload mismatch is a compile error. Handlers
// are thin: they delegate to the #422 RelationService and map its errors to RFC
// 9457 problem+json.
type relationsStrictServer struct {
	s *Server
}

var _ relationsgen.StrictServerInterface = (*relationsStrictServer)(nil)

// relatedOnReadCap bounds the depth-1 typed neighborhood surfaced on a resource
// detail GET (issue #424).
const relatedOnReadCap = 20

// relatedForResource loads a resource's depth-1 typed neighborhood for the
// `related` field on its detail GET: both directions, newest first, capped at
// relatedOnReadCap. Best-effort — a relations-load failure is logged and yields
// an empty list (never null) rather than failing the resource read; `related`
// is supplementary to the resource itself.
func (s *Server) relatedForResource(
	ctx context.Context, userID, teamID, resourceType, resourceID string,
) models.JSONArray[models.RelatedResource] {
	svc := s.container.RelationService()
	if svc == nil {
		return nil
	}
	resp, err := svc.ListByResource(
		ctx, userID, teamID, resourceType, resourceID, 1, relatedOnReadCap,
	)
	if err != nil {
		s.logger.With(
			"handler", "relatedForResource",
			"team_id", teamID,
			"resource_type", resourceType,
			"resource_id", resourceID,
			"error", err.Error(),
		).Warn("Failed to load related resources for detail GET")
		return nil
	}
	return resp.Related
}

// ListRelations handles GET /api/v1/{team_id}/relations
func (rs *relationsStrictServer) ListRelations(
	ctx context.Context, request relationsgen.ListRelationsRequestObject,
) (relationsgen.ListRelationsResponseObject, error) {
	teamID := request.TeamId.String()
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}

	page, limit := derefPageLimit(request.Params.Page, request.Params.Limit)

	resp, err := rs.s.container.RelationService().ListByResource(
		ctx, userID, teamID, request.Params.ResourceType, request.Params.ResourceId.String(), page, limit,
	)
	if err != nil {
		return nil, rs.relationError(ctx, "ListRelations", teamID, err)
	}

	genResp, convErr := toGenRelationListResponse(resp)
	if convErr != nil {
		return nil, rs.relationConversionError("ListRelations", teamID, convErr)
	}
	return relationsgen.ListRelations200JSONResponse(genResp), nil
}

// CreateRelation handles POST /api/v1/{team_id}/relations. Creation is
// idempotent: a newly-inserted edge is 201, a pre-existing one is 200.
func (rs *relationsStrictServer) CreateRelation(
	ctx context.Context, request relationsgen.CreateRelationRequestObject,
) (relationsgen.CreateRelationResponseObject, error) {
	teamID := request.TeamId.String()
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("request body is required")
	}

	created, wasCreated, err := rs.s.container.RelationService().Create(ctx, userID, teamID, &models.CreateRelationRequest{
		FromType:     string(request.Body.FromType),
		FromID:       request.Body.FromId.String(),
		ToType:       string(request.Body.ToType),
		ToID:         request.Body.ToId.String(),
		RelationType: string(request.Body.RelationType),
		Origin:       string(request.Body.Origin),
	})
	if err != nil {
		return nil, rs.relationError(ctx, "CreateRelation", teamID, err)
	}

	genRelation, convErr := toGenRelation(*created)
	if convErr != nil {
		return nil, rs.relationConversionError("CreateRelation", teamID, convErr)
	}
	if wasCreated {
		return relationsgen.CreateRelation201JSONResponse(genRelation), nil
	}
	return relationsgen.CreateRelation200JSONResponse(genRelation), nil
}

// ConfirmRelation handles POST /api/v1/{team_id}/relations/{relation_id}/confirm
func (rs *relationsStrictServer) ConfirmRelation(
	ctx context.Context, request relationsgen.ConfirmRelationRequestObject,
) (relationsgen.ConfirmRelationResponseObject, error) {
	teamID := request.TeamId.String()
	relationID := request.RelationId.String()
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}

	confirmed, err := rs.s.container.RelationService().Confirm(ctx, userID, teamID, relationID)
	if err != nil {
		return nil, rs.relationError(ctx, "ConfirmRelation", teamID, err)
	}

	genRelation, convErr := toGenRelation(*confirmed)
	if convErr != nil {
		return nil, rs.relationConversionError("ConfirmRelation", teamID, convErr)
	}
	return relationsgen.ConfirmRelation200JSONResponse(genRelation), nil
}

// DeleteRelation handles DELETE /api/v1/{team_id}/relations/{relation_id}
func (rs *relationsStrictServer) DeleteRelation(
	ctx context.Context, request relationsgen.DeleteRelationRequestObject,
) (relationsgen.DeleteRelationResponseObject, error) {
	teamID := request.TeamId.String()
	relationID := request.RelationId.String()
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}

	if delErr := rs.s.container.RelationService().Delete(ctx, userID, teamID, relationID); delErr != nil {
		return nil, rs.relationError(ctx, "DeleteRelation", teamID, delErr)
	}
	return relationsgen.DeleteRelation204Response{}, nil
}

// relationError maps a RelationService error to an RFC 9457 APIError. Recognized
// domain errors become 400/403/404/409; anything else is logged and 500.
func (rs *relationsStrictServer) relationError(
	_ context.Context, handler, teamID string, err error,
) *apierrors.APIError {
	if apiErr := mapRelationServiceError(err); apiErr != nil {
		return apiErr
	}
	rs.s.logger.With(
		"handler", handler,
		"team_id", teamID,
		"error", err.Error(),
	).Error("Relation operation failed")
	return apierrors.NewInternalError(relationsMsgInternalError)
}

func (rs *relationsStrictServer) relationConversionError(handler, teamID string, err error) *apierrors.APIError {
	rs.s.logger.With(
		"handler", handler,
		"team_id", teamID,
		"error", err.Error(),
	).Error("Failed to convert relation to spec type")
	return apierrors.NewInternalError(relationsMsgInternalError)
}

// mapRelationServiceError translates the RelationService's errors (a mix of
// sentinels and validation strings) into problem+json APIErrors. It returns nil
// for an unrecognized error so the caller maps it to a 500.
func mapRelationServiceError(err error) *apierrors.APIError {
	switch {
	case errors.Is(err, services.ErrPermissionDenied):
		return apierrors.NewForbiddenError("You do not have permission to perform this action")
	case errors.Is(err, services.ErrRelationSelfLink),
		errors.Is(err, services.ErrRelationInvalidType),
		errors.Is(err, services.ErrRelationCrossProject):
		return apierrors.NewBadRequestError(err.Error())
	case errors.Is(err, services.ErrRelationResourceNotFound):
		return apierrors.NewResourceNotFoundError("resource", "One of the relation endpoints was not found")
	case errors.Is(err, repositories.ErrRelationNotFound):
		return apierrors.NewResourceNotFoundError("relation", "Relation not found")
	case errors.Is(err, services.ErrRelationAlreadyConfirmed):
		return apierrors.NewAPIError(
			apierrors.CodeResourceConflict,
			apierrors.GetErrorTitle(apierrors.CodeResourceConflict),
			"Relation is already confirmed",
			http.StatusConflict,
		)
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "user is not a member"):
		return apierrors.NewForbiddenError("You are not a member of this team")
	case strings.Contains(msg, "invalid resource type"),
		strings.Contains(msg, "invalid relation type"),
		strings.Contains(msg, "invalid origin"):
		return apierrors.NewBadRequestError(msg)
	default:
		return nil
	}
}

func toGenRelation(item models.Relation) (relationsgen.Relation, error) {
	id, err := uuid.Parse(item.ID)
	if err != nil {
		return relationsgen.Relation{}, fmt.Errorf("relation id %q is not a UUID: %w", item.ID, err)
	}
	teamID, err := uuid.Parse(item.TeamID)
	if err != nil {
		return relationsgen.Relation{}, fmt.Errorf("relation team_id %q is not a UUID: %w", item.TeamID, err)
	}
	projectID, err := uuid.Parse(item.ProjectID)
	if err != nil {
		return relationsgen.Relation{}, fmt.Errorf("relation project_id %q is not a UUID: %w", item.ProjectID, err)
	}
	fromID, err := uuid.Parse(item.FromID)
	if err != nil {
		return relationsgen.Relation{}, fmt.Errorf("relation from_id %q is not a UUID: %w", item.FromID, err)
	}
	toID, err := uuid.Parse(item.ToID)
	if err != nil {
		return relationsgen.Relation{}, fmt.Errorf("relation to_id %q is not a UUID: %w", item.ToID, err)
	}

	rel := relationsgen.Relation{
		Id:           id,
		TeamId:       teamID,
		ProjectId:    projectID,
		FromType:     item.FromType,
		FromId:       fromID,
		ToType:       item.ToType,
		ToId:         toID,
		RelationType: relationsgen.RelationRelationType(item.RelationType),
		Origin:       relationsgen.RelationOrigin(item.Origin),
		Status:       relationsgen.RelationStatus(item.Status),
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}
	if item.CreatedBy != nil {
		createdBy, parseErr := uuid.Parse(*item.CreatedBy)
		if parseErr != nil {
			return relationsgen.Relation{}, fmt.Errorf("relation created_by %q is not a UUID: %w", *item.CreatedBy, parseErr)
		}
		rel.CreatedBy = &createdBy
	}
	if item.ConfirmedBy != nil {
		confirmedBy, parseErr := uuid.Parse(*item.ConfirmedBy)
		if parseErr != nil {
			return relationsgen.Relation{}, fmt.Errorf("relation confirmed_by %q is not a UUID: %w", *item.ConfirmedBy, parseErr)
		}
		rel.ConfirmedBy = &confirmedBy
	}
	return rel, nil
}

func toGenRelatedResource(item models.RelatedResource) (relationsgen.RelatedResource, error) {
	relationID, err := uuid.Parse(item.RelationID)
	if err != nil {
		return relationsgen.RelatedResource{}, fmt.Errorf("related relation_id %q is not a UUID: %w", item.RelationID, err)
	}
	resourceID, err := uuid.Parse(item.ResourceID)
	if err != nil {
		return relationsgen.RelatedResource{}, fmt.Errorf("related resource_id %q is not a UUID: %w", item.ResourceID, err)
	}

	row := relationsgen.RelatedResource{
		RelationId:   relationID,
		RelationType: relationsgen.RelatedResourceRelationType(item.RelationType),
		Direction:    relationsgen.RelatedResourceDirection(item.Direction),
		Origin:       relationsgen.RelatedResourceOrigin(item.Origin),
		Status:       relationsgen.RelatedResourceStatus(item.Status),
		ResourceType: item.ResourceType,
		ResourceId:   resourceID,
		Title:        item.Title,
		Slug:         item.Slug,
		CreatedAt:    item.CreatedAt,
	}
	if item.ProjectID != nil {
		projectID, parseErr := uuid.Parse(*item.ProjectID)
		if parseErr != nil {
			return relationsgen.RelatedResource{},
				fmt.Errorf("related project_id %q is not a UUID: %w", *item.ProjectID, parseErr)
		}
		row.ProjectId = &projectID
	}
	return row, nil
}

func toGenRelationListResponse(resp *models.RelationListResponse) (relationsgen.RelationListResponse, error) {
	// make(...,0) guarantees the required `relations` array serializes as [] not null.
	related := make([]relationsgen.RelatedResource, 0, len(resp.Related))
	for i := range resp.Related {
		g, err := toGenRelatedResource(resp.Related[i])
		if err != nil {
			return relationsgen.RelationListResponse{}, err
		}
		related = append(related, g)
	}
	return relationsgen.RelationListResponse{
		Relations:  related,
		TotalCount: resp.TotalCount,
		Page:       resp.Page,
		PerPage:    resp.PerPage,
		TotalPages: resp.TotalPages,
	}, nil
}

// relationsBindErrorHandler translates parameter-binding failures from the
// generated layer into this domain's RFC 9457 400 responses.
func (s *Server) relationsBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()

	var invalidParam *relationsgen.InvalidParamFormatError
	if errors.As(err, &invalidParam) {
		switch invalidParam.ParamName {
		case "team_id":
			msg = "team_id must be a valid UUID"
		case "relation_id":
			msg = "relation_id must be a valid UUID"
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

// relationsResponseErrorHandler writes errors returned by the strict handler
// implementations. *apierrors.APIError carries the intended RFC 9457 error;
// anything else is defensive and maps to a generic 500.
func (s *Server) relationsResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With("error", err).Error("Relations strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(relationsMsgInternalError))
}
