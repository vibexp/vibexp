package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/repositories"
	statusesgen "github.com/vibexp/vibexp/internal/server/gen/statuses"
	"github.com/vibexp/vibexp/internal/services"
)

// Problem details for custom status operations.
const (
	statusesMsgListFailed   = "Failed to list statuses"
	statusesMsgCreateFailed = "Failed to create status"
	statusesMsgRenameFailed = "Failed to rename status"
	statusesMsgDeleteFailed = "Failed to delete status"
	statusesMsgForbidden    = "You do not have permission to manage this team's statuses"
	statusesMsgNotFound     = "Status not found"
	statusesMsgBodyRequired = "request body is required"
)

// statusesStrictServer implements statusesgen.StrictServerInterface: the
// team's custom resource statuses (epic #1263), served through oapi-codegen
// strict-server bindings so a spec/handler payload mismatch is a compile error.
//
// The handlers hold no rules of their own. Validation, the per-type limit,
// authorization and the audit entry all live in
// services.StatusManagementService; a handler maps its typed errors to RFC 9457
// responses. As in the other strict domains, a handler returns
// *apierrors.APIError and statusesResponseErrorHandler renders it as
// application/problem+json.
type statusesStrictServer struct {
	s *Server
}

var _ statusesgen.StrictServerInterface = (*statusesStrictServer)(nil)

// ListStatuses handles GET /api/v1/{team_id}/statuses
func (t *statusesStrictServer) ListStatuses(
	ctx context.Context, request statusesgen.ListStatusesRequestObject,
) (statusesgen.ListStatusesResponseObject, error) {
	teamID := request.TeamId.String()
	resourceType := string(request.Params.ResourceType)

	items, err := t.s.container.StatusManagementService().List(ctx, teamID, resourceType)
	if err != nil {
		return nil, t.failure(err, "ListStatuses", teamID, statusesMsgListFailed)
	}

	statuses := make([]statusesgen.StatusDefinition, 0, len(items))
	for i := range items {
		status, convErr := toGenStatus(&items[i])
		if convErr != nil {
			return nil, t.failure(convErr, "ListStatuses", teamID, statusesMsgListFailed)
		}
		statuses = append(statuses, status)
	}
	return statusesgen.ListStatuses200JSONResponse(statusesgen.StatusListResponse{Statuses: statuses}), nil
}

// CreateStatus handles POST /api/v1/{team_id}/statuses
func (t *statusesStrictServer) CreateStatus(
	ctx context.Context, request statusesgen.CreateStatusRequestObject,
) (statusesgen.CreateStatusResponseObject, error) {
	teamID := request.TeamId.String()
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError(statusesMsgBodyRequired)
	}

	created, err := t.s.container.StatusManagementService().Create(ctx, services.CreateStatusParams{
		TeamID:       teamID,
		UserID:       statusesUserID(ctx),
		ResourceType: string(request.Body.ResourceType),
		Slug:         request.Body.Slug,
		Name:         request.Body.Name,
	})
	if err != nil {
		return nil, t.failure(err, "CreateStatus", teamID, statusesMsgCreateFailed)
	}

	status, convErr := toGenStatus(created)
	if convErr != nil {
		return nil, t.failure(convErr, "CreateStatus", teamID, statusesMsgCreateFailed)
	}
	return statusesgen.CreateStatus201JSONResponse(status), nil
}

// UpdateStatus handles PATCH /api/v1/{team_id}/statuses/{id}
func (t *statusesStrictServer) UpdateStatus(
	ctx context.Context, request statusesgen.UpdateStatusRequestObject,
) (statusesgen.UpdateStatusResponseObject, error) {
	teamID := request.TeamId.String()
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError(statusesMsgBodyRequired)
	}

	renamed, err := t.s.container.StatusManagementService().Rename(ctx, services.RenameStatusParams{
		TeamID: teamID,
		UserID: statusesUserID(ctx),
		ID:     request.Id.String(),
		Name:   request.Body.Name,
	})
	if err != nil {
		return nil, t.failure(err, "UpdateStatus", teamID, statusesMsgRenameFailed)
	}

	status, convErr := toGenStatus(renamed)
	if convErr != nil {
		return nil, t.failure(convErr, "UpdateStatus", teamID, statusesMsgRenameFailed)
	}
	return statusesgen.UpdateStatus200JSONResponse(status), nil
}

// DeleteStatus handles DELETE /api/v1/{team_id}/statuses/{id}. The reassignment
// it triggers is one UPDATE over every resource carrying the status, inside the
// delete's transaction; it is not batched.
func (t *statusesStrictServer) DeleteStatus(
	ctx context.Context, request statusesgen.DeleteStatusRequestObject,
) (statusesgen.DeleteStatusResponseObject, error) {
	teamID := request.TeamId.String()

	deletion, err := t.s.container.StatusManagementService().Delete(ctx, services.DeleteStatusParams{
		TeamID: teamID,
		UserID: statusesUserID(ctx),
		ID:     request.Id.String(),
	})
	if err != nil {
		return nil, t.failure(err, "DeleteStatus", teamID, statusesMsgDeleteFailed)
	}

	return statusesgen.DeleteStatus200JSONResponse(statusesgen.DeleteStatusResponse{
		ReassignedCount: deletion.ReassignedCount,
		ReassignedTo:    deletion.ReassignedTo,
	}), nil
}

// failure maps a service error to its documented response, logging only the
// ones that become a 500.
func (t *statusesStrictServer) failure(err error, handler, teamID, internalMsg string) error {
	if apiErr := statusesClientError(err); apiErr != nil {
		return apiErr
	}
	t.s.logger.With(
		"handler", handler,
		"team_id", teamID,
		"error", err.Error(),
	).Error(internalMsg)
	return apierrors.NewInternalError(internalMsg)
}

// statusesClientError returns the 4xx a service error documents, or nil for
// one the caller did not cause.
func statusesClientError(err error) *apierrors.APIError {
	switch {
	case errors.Is(err, services.ErrPermissionDenied):
		return apierrors.NewForbiddenError(statusesMsgForbidden)
	case errors.Is(err, repositories.ErrTeamStatusNotFound):
		return apierrors.NewResourceNotFoundError("status", statusesMsgNotFound)
	case errors.Is(err, services.ErrStatusReserved):
		return apierrors.NewResourceExistsError("status", services.ErrStatusReserved.Error())
	case errors.Is(err, repositories.ErrTeamStatusExists):
		return apierrors.NewResourceExistsError("status",
			"A status with this slug already exists for this resource type")
	case errors.Is(err, services.ErrStatusResourceTypeUnsupported):
		return apierrors.NewBadRequestError("resource_type must be one of prompt, artifact, blueprint, memory")
	case isStatusValidationError(err):
		return apierrors.NewBadRequestError(err.Error())
	}
	return lockedResourcesProblem(err)
}

// isStatusValidationError reports whether err is one of the status input
// sentinels that map to 400.
func isStatusValidationError(err error) bool {
	return errors.Is(err, services.ErrStatusSlugInvalid) ||
		errors.Is(err, services.ErrStatusSlugTooLong) ||
		errors.Is(err, services.ErrStatusNameRequired) ||
		errors.Is(err, services.ErrStatusNameTooLong) ||
		errors.Is(err, services.ErrStatusLimitReached)
}

// statusesUserID reads the authenticated user the auth middleware put on the
// request context.
func statusesUserID(ctx context.Context) string {
	userID, _ := ctx.Value(contextKeyUserID).(string)
	return userID
}

// toGenStatus converts a managed status to the spec type. A system status has
// no id and no timestamps, so those keys are omitted for it.
func toGenStatus(item *services.ManagedStatus) (statusesgen.StatusDefinition, error) {
	status := statusesgen.StatusDefinition{
		ResourceType: statusesgen.StatusResourceType(item.ResourceType),
		Slug:         item.Slug,
		Name:         item.Name,
		IsSystem:     item.IsSystem,
		IsLive:       item.IsLive,
		UsageCount:   item.UsageCount,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}
	if item.ID != nil {
		id, err := uuid.Parse(*item.ID)
		if err != nil {
			return statusesgen.StatusDefinition{}, fmt.Errorf("status id %q is not a UUID: %w", *item.ID, err)
		}
		status.Id = &id
	}
	return status, nil
}

// statusBodyFields lists, per method, the only keys a status request body may
// carry.
var statusBodyFields = map[string][]string{
	http.MethodPost:  {"resource_type", "slug", "name"},
	http.MethodPatch: {"name"},
}

// rejectUnknownStatusBodyFields enforces the `additionalProperties: false` the
// spec declares on both status request bodies, which oapi-codegen does not. It
// matters most on the rename: without it a PATCH carrying `slug` would be
// answered 200 with the slug unchanged, and the caller would have no way to
// tell the field was ignored.
//
// A body that is empty or not a JSON object is left for the generated decoder
// to reject in its usual words.
func rejectUnknownStatusBodyFields(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, guarded := statusBodyFields[r.Method]
		if !guarded || r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}

		fields, ok := peekJSONObjectBody(w, r)
		if !ok {
			return
		}

		var unknown []string
		for key := range fields {
			if !slices.Contains(allowed, key) {
				unknown = append(unknown, key)
			}
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(fmt.Sprintf(
				"Unknown field(s): %s. Only %s may be set.",
				strings.Join(unknown, ", "), strings.Join(allowed, ", "))))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// statusesBindErrorHandler translates parameter-binding and body-decoding
// failures from the generated layer into RFC 9457 400 responses (the generated
// default writes plain text, and its wording leaks decoder internals).
func (s *Server) statusesBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	msg := msgInvalidBodyWellFormedJSON

	var invalidParam *statusesgen.InvalidParamFormatError
	var requiredParam *statusesgen.RequiredParamError
	switch {
	case errors.As(err, &invalidParam):
		switch invalidParam.ParamName {
		case "team_id":
			msg = "team_id must be a valid UUID"
		case "id":
			msg = "status id must be a valid UUID"
		default:
			msg = invalidParam.ParamName + " is invalid"
		}
	case errors.As(err, &requiredParam):
		msg = requiredParam.ParamName + " is required"
	}

	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msg))
}

// statusesResponseErrorHandler writes errors returned by the strict handler
// implementations. *apierrors.APIError carries the intended RFC 9457 error;
// anything else is defensive and maps to a generic 500.
func (s *Server) statusesResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With("error", err).Error("Statuses strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Internal server error"))
}
