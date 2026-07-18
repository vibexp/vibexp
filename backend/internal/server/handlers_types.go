package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	typesgen "github.com/vibexp/vibexp/internal/server/gen/types"
	"github.com/vibexp/vibexp/internal/services"
)

// Problem details for failed type (category) operations.
const (
	typesMsgListFailed   = "Failed to list types"
	typesMsgCreateFailed = "Failed to create type"
)

// typesStrictServer implements typesgen.StrictServerInterface (#1846): the
// team-customizable type (category) CRUD operations are served through
// oapi-codegen strict-server bindings generated from openapi.yaml, so a
// spec/handler payload mismatch is a compile error for this domain. As the
// second spec-first domain it generates into its own package
// (internal/server/gen/types) so the Notifications wiring stays untouched —
// see oapi-codegen-types.yaml for the friction-6 rationale.
type typesStrictServer struct {
	s *Server
}

var _ typesgen.StrictServerInterface = (*typesStrictServer)(nil)

// Strict handler implementations return *apierrors.APIError directly (it
// implements error). typesResponseErrorHandler renders it as RFC 9457
// application/problem+json — the typed gen.*JSONResponse error bodies would
// write application/json, so they are deliberately bypassed (see #1768).

// ListTypes handles GET /api/v1/{team_id}/types
func (t *typesStrictServer) ListTypes(
	ctx context.Context, request typesgen.ListTypesRequestObject,
) (typesgen.ListTypesResponseObject, error) {
	teamID := request.TeamId.String()
	resourceType := request.Params.ResourceType

	items, err := t.s.container.TypeService().List(ctx, teamID, resourceType)
	if err != nil {
		if errors.Is(err, services.ErrTypeResourceTypeUnsupported) {
			return nil, apierrors.NewBadRequestError(err.Error())
		}
		t.s.logger.With(
			"handler", "ListTypes",
			"team_id", teamID,
			"resource_type", resourceType,
			"error", err.Error(),
		).Error(typesMsgListFailed)
		return nil, apierrors.NewInternalError(typesMsgListFailed)
	}

	genTypes, convErr := toGenTypes(items)
	if convErr != nil {
		t.s.logger.With(
			"handler", "ListTypes",
			"team_id", teamID,
			"error", convErr.Error(),
		).Error("Failed to convert types to spec types")
		return nil, apierrors.NewInternalError(typesMsgListFailed)
	}

	return typesgen.ListTypes200JSONResponse(typesgen.TypeListResponse{
		Types:      genTypes,
		TotalCount: len(genTypes),
	}), nil
}

// CreateType handles POST /api/v1/{team_id}/types
func (t *typesStrictServer) CreateType(
	ctx context.Context, request typesgen.CreateTypeRequestObject,
) (typesgen.CreateTypeResponseObject, error) {
	teamID := request.TeamId.String()
	userID := ctx.Value(contextKeyUserID).(string)

	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("request body is required")
	}

	created, err := t.s.container.TypeService().CreateCustom(ctx, services.CreateTypeParams{
		TeamID:       teamID,
		UserID:       userID,
		ResourceType: request.Body.ResourceType,
		Slug:         request.Body.Slug,
		Name:         request.Body.Name,
	})
	if err != nil {
		if isTypeValidationError(err) {
			return nil, apierrors.NewBadRequestError(err.Error())
		}
		if errors.Is(err, repositories.ErrTypeAlreadyExists) {
			return nil, apierrors.NewResourceExistsError("type",
				"A type with this slug already exists for this resource")
		}
		t.s.logger.With(
			"handler", "CreateType",
			"team_id", teamID,
			"error", err.Error(),
		).Error(typesMsgCreateFailed)
		return nil, apierrors.NewInternalError(typesMsgCreateFailed)
	}

	genType, convErr := toGenType(*created)
	if convErr != nil {
		t.s.logger.With(
			"handler", "CreateType",
			"team_id", teamID,
			"error", convErr.Error(),
		).Error("Failed to convert created type to spec type")
		return nil, apierrors.NewInternalError(typesMsgCreateFailed)
	}

	return typesgen.CreateType201JSONResponse(genType), nil
}

// DeleteType handles DELETE /api/v1/{team_id}/types/{id}
func (t *typesStrictServer) DeleteType(
	ctx context.Context, request typesgen.DeleteTypeRequestObject,
) (typesgen.DeleteTypeResponseObject, error) {
	teamID := request.TeamId.String()
	id := request.Id.String()

	if err := t.s.container.TypeService().Delete(ctx, teamID, id); err != nil {
		if errors.Is(err, repositories.ErrTypeNotFound) {
			return nil, apierrors.NewResourceNotFoundError("type", "Type not found")
		}
		t.s.logger.With(
			"handler", "DeleteType",
			"team_id", teamID,
			"type_id", id,
			"error", err.Error(),
		).Error("Failed to delete type")
		return nil, apierrors.NewInternalError("Failed to delete type")
	}

	return typesgen.DeleteType204Response{}, nil
}

// isTypeValidationError reports whether err is one of the TypeService input
// validation sentinels (all map to 400).
func isTypeValidationError(err error) bool {
	return errors.Is(err, services.ErrTypeSlugRequired) ||
		errors.Is(err, services.ErrTypeSlugInvalid) ||
		errors.Is(err, services.ErrTypeSlugTooLong) ||
		errors.Is(err, services.ErrTypeNameRequired) ||
		errors.Is(err, services.ErrTypeNameTooLong) ||
		errors.Is(err, services.ErrTypeResourceTypeUnsupported)
}

// toGenTypes converts service types to the generated spec types.
func toGenTypes(items []models.Type) ([]typesgen.Type, error) {
	out := make([]typesgen.Type, 0, len(items))
	for i := range items {
		g, err := toGenType(items[i])
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func toGenType(item models.Type) (typesgen.Type, error) {
	id, err := uuid.Parse(item.ID)
	if err != nil {
		return typesgen.Type{}, fmt.Errorf("type id %q is not a UUID: %w", item.ID, err)
	}

	g := typesgen.Type{
		Id:           id,
		ResourceType: item.ResourceType,
		Slug:         item.Slug,
		Name:         item.Name,
		IsSystem:     item.IsSystem,
		CreatedAt:    item.CreatedAt,
	}

	// team_id is omitted for global system defaults (empty on the model).
	if item.TeamID != "" {
		teamID, parseErr := uuid.Parse(item.TeamID)
		if parseErr != nil {
			return typesgen.Type{}, fmt.Errorf("type team_id %q is not a UUID: %w", item.TeamID, parseErr)
		}
		g.TeamId = &teamID
	}

	return g, nil
}

// typesBindErrorHandler translates parameter-binding failures from the
// generated layer into this domain's RFC 9457 400 responses (the generated
// default writes a plain-text 400).
func (s *Server) typesBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()

	var invalidParam *typesgen.InvalidParamFormatError
	if errors.As(err, &invalidParam) {
		switch invalidParam.ParamName {
		case "team_id":
			msg = "team_id must be a valid UUID"
		case "id":
			msg = "type id must be a valid UUID"
		case "resource_type":
			msg = "resource_type is required"
		}
	}

	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msg))
}

// typesResponseErrorHandler writes errors returned by the strict handler
// implementations. *apierrors.APIError carries the intended RFC 9457 error;
// anything else is defensive and maps to a generic 500.
func (s *Server) typesResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With("error", err).Error("Types strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Internal server error"))
}
