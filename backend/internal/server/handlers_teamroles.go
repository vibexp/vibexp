package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	teamrolesgen "github.com/vibexp/vibexp/internal/server/gen/teamroles"
	"github.com/vibexp/vibexp/internal/services"
)

// teamRolesStrictServer implements teamrolesgen.StrictServerInterface (#222):
// the two role-management operations (change a member's role, transfer
// ownership) are served through oapi-codegen strict-server bindings generated
// from openapi.yaml, so a spec/handler payload mismatch is a compile error.
//
// They carry their own `Team Roles` tag and package rather than joining the
// `Teams` tag, because include-tags is the unit of generation: tagging them
// Teams would emit a StrictServerInterface for every legacy Teams operation and
// fail to compile until all of them were rewritten. See
// oapi-codegen-teamroles.yaml, and the friction-6 rationale in
// oapi-codegen-types.yaml for the one-package-per-domain decision.
type teamRolesStrictServer struct {
	s *Server
}

var _ teamrolesgen.StrictServerInterface = (*teamRolesStrictServer)(nil)

// Strict handler implementations return *apierrors.APIError directly (it
// implements error). teamRolesResponseErrorHandler renders it as RFC 9457
// application/problem+json — the typed gen.*JSONResponse error bodies would
// write application/json, so they are deliberately bypassed (see #1768).

// UpdateTeamMemberRole handles PATCH /api/v1/teams/{id}/members/{userId}/role
func (t *teamRolesStrictServer) UpdateTeamMemberRole(
	ctx context.Context, request teamrolesgen.UpdateTeamMemberRoleRequestObject,
) (teamrolesgen.UpdateTeamMemberRoleResponseObject, error) {
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}

	teamID := request.Id.String()
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("Request body is required")
	}

	detail, err := t.s.container.TeamService().UpdateMemberRole(
		ctx, userID, teamID, request.UserId, models.TeamMemberRole(request.Body.Role),
	)
	if err != nil {
		return nil, t.mapTeamRoleError(ctx, err, "UpdateTeamMemberRole", teamID)
	}

	return teamrolesgen.UpdateTeamMemberRole200JSONResponse{
		Member: toGenTeamMemberDetail(*detail),
	}, nil
}

// TransferTeamOwnership handles POST /api/v1/teams/{id}/transfer-ownership
func (t *teamRolesStrictServer) TransferTeamOwnership(
	ctx context.Context, request teamrolesgen.TransferTeamOwnershipRequestObject,
) (teamrolesgen.TransferTeamOwnershipResponseObject, error) {
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}

	teamID := request.Id.String()
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("Request body is required")
	}
	if request.Body.NewOwnerId == "" {
		return nil, apierrors.NewBadRequestError("new_owner_id is required")
	}

	team, err := t.s.container.TeamService().TransferOwnership(ctx, userID, teamID, request.Body.NewOwnerId)
	if err != nil {
		return nil, t.mapTeamRoleError(ctx, err, "TransferTeamOwnership", teamID)
	}

	genTeam, err := toGenTeam(*team)
	if err != nil {
		t.s.logger.With("handler", "TransferTeamOwnership", "team_id", teamID, "error", err).
			Error("Failed to render transferred team")
		return nil, apierrors.NewInternalError("Failed to transfer team ownership")
	}

	return teamrolesgen.TransferTeamOwnership200JSONResponse{Team: genTeam}, nil
}

// mapTeamRoleError translates service errors into RFC 9457 responses. Both
// operations share a vocabulary, so they share the mapping — every branch is
// reachable from both.
func (t *teamRolesStrictServer) mapTeamRoleError(
	_ context.Context, err error, handler, teamID string,
) error {
	switch {
	case errors.Is(err, services.ErrPermissionDenied):
		return apierrors.NewForbiddenError("You do not have permission to perform this action on this team")
	case errors.Is(err, services.ErrCannotChangeOwnerRole):
		return apierrors.NewForbiddenError("The team owner's role cannot be changed; transfer ownership instead")
	case errors.Is(err, services.ErrInvalidMemberRole):
		return apierrors.NewBadRequestError("Role must be either member or admin")
	case errors.Is(err, services.ErrAlreadyTeamOwner):
		return apierrors.NewBadRequestError("That user already owns this team")
	case errors.Is(err, services.ErrTeamNotFound), errors.Is(err, repositories.ErrTeamNotFound):
		return apierrors.NewResourceNotFoundError("team", "Team not found")
	case errors.Is(err, repositories.ErrTeamMemberNotFound):
		return apierrors.NewResourceNotFoundError("team member", "User is not a member of this team")
	}

	// A personal workspace cannot be transferred; it is a typed error, not a
	// sentinel.
	var personal *services.PersonalWorkspaceError
	if errors.As(err, &personal) {
		return apierrors.NewForbiddenError("A personal workspace cannot be transferred")
	}

	t.s.logger.With(
		"service", "vibexp-api",
		"handler", handler,
		"team_id", teamID,
		"error", err.Error(),
	).Error("Team role operation failed")

	return apierrors.NewInternalError("Failed to complete the team role operation")
}

// authedUserID reads the authenticated user injected by the auth middleware.
// The generated layer knows nothing about auth, so this mirrors what the legacy
// handlers do — but without their unchecked type assertion, which panics when
// the middleware has not run.
func authedUserID(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(contextKeyUserID).(string)
	if !ok || userID == "" {
		return "", apierrors.NewAuthRequiredError("Authentication required")
	}
	return userID, nil
}

func toGenTeamMemberDetail(d models.TeamMemberDetail) teamrolesgen.TeamMemberDetail {
	// JoinedAt is RFC3339 in the model; a parse failure would mean we wrote it
	// wrong, so fall back to the zero time rather than failing the request.
	joinedAt, err := time.Parse(time.RFC3339, d.JoinedAt)
	if err != nil {
		joinedAt = time.Time{}
	}

	detail := teamrolesgen.TeamMemberDetail{
		UserId:   d.UserID,
		Email:    openapi_types.Email(d.Email),
		Name:     d.Name,
		Role:     teamrolesgen.TeamMemberDetailRole(d.Role),
		JoinedAt: joinedAt,
	}
	if d.InvitationStatus != nil {
		status := teamrolesgen.TeamMemberDetailInvitationStatus(*d.InvitationStatus)
		detail.InvitationStatus = &status
	}
	return detail
}

func toGenTeam(t models.Team) (teamrolesgen.Team, error) {
	id, err := uuid.Parse(t.ID)
	if err != nil {
		return teamrolesgen.Team{}, err
	}

	// permissions is a required array, so it must marshal as [] rather than
	// null (#125 Layer C). The generated type cannot use the models.JSONArray
	// shim, so make(...,0) here is what upholds the guarantee.
	permissions := make([]teamrolesgen.TeamPermissions, 0, len(t.Permissions))
	for _, perm := range t.Permissions {
		permissions = append(permissions, teamrolesgen.TeamPermissions(perm))
	}

	team := teamrolesgen.Team{
		Id:          id,
		OwnerId:     t.OwnerID,
		Name:        t.Name,
		Slug:        t.Slug,
		Description: t.Description,
		IsPersonal:  t.IsPersonal,
		Permissions: permissions,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
	if t.Role != "" {
		role := t.Role
		team.Role = &role
	}
	return team, nil
}

// teamRolesBindErrorHandler maps request-binding failures from the generated
// layer into this domain's RFC 9457 400 responses (the generated default writes
// a plain-text 400).
func (s *Server) teamRolesBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()

	var invalidParam *teamrolesgen.InvalidParamFormatError
	if errors.As(err, &invalidParam) {
		if invalidParam.ParamName == "id" {
			msg = "team id must be a valid UUID"
		}
	}

	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msg))
}

// teamRolesResponseErrorHandler writes errors returned by the strict handler
// implementations. *apierrors.APIError carries the intended RFC 9457 error;
// anything else is defensive and maps to a generic 500.
func (s *Server) teamRolesResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With("error", err).Error("Team Roles strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Internal server error"))
}
