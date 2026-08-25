package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	modelprovidersgen "github.com/vibexp/vibexp/internal/server/gen/modelproviders"
	"github.com/vibexp/vibexp/internal/services"
)

// Problem details for the cross-team model-provider copy (#830, epic #827).
const (
	modelProviderMsgCopyFailed = "Failed to copy the model provider"
	// modelProviderMsgCopyForbidden is the ONE message a denied copy ever
	// returns, whichever team the caller was refused on. Naming the team would
	// tell a caller entitled to neither whether the source team exists (#829).
	modelProviderMsgCopyForbidden = "You must have permission to manage provider settings " +
		"in both the source and the destination team"
	msgSourceTeamIDRequired     = "source_team_id is required"
	msgSourceProviderIDRequired = "source_provider_id is required"
)

// modelProvidersStrictServer implements modelprovidersgen.StrictServerInterface.
//
// This file holds the cross-team copy (#830), which shipped strict-server-typed
// because a NEW endpoint has to; the twelve CRUD/validate operations that make
// up the rest of the interface live in model_provider_strict_handlers.go, where
// #837 converted them off their hand-written chi handlers. Every response body
// in the domain is now checked against openapi.yaml by the compiler rather than
// by hand.
type modelProvidersStrictServer struct {
	s *Server
}

var _ modelprovidersgen.StrictServerInterface = (*modelProvidersStrictServer)(nil)

// CopyModelProviderFromTeam handles
// POST /api/v1/{team_id}/settings/model-providers/copy
func (m *modelProvidersStrictServer) CopyModelProviderFromTeam(
	ctx context.Context, request modelprovidersgen.CopyModelProviderFromTeamRequestObject,
) (modelprovidersgen.CopyModelProviderFromTeamResponseObject, error) {
	teamID := request.TeamId.String()
	userID := ctx.Value(contextKeyUserID).(string)

	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("request body is required")
	}
	// oapi-codegen enforces neither `required` nor additionalProperties on a
	// request body, so `{}` binds both uuids as the ZERO uuid rather than
	// failing. Left unguarded they reach the service, fail authorization
	// against a team that cannot exist, and answer 403 — where the spec
	// documents 400, and a 403 would imply the all-zero team is merely one the
	// caller lacks access to (#829).
	if request.Body.SourceTeamId == uuid.Nil {
		return nil, apierrors.NewBadRequestError(msgSourceTeamIDRequired)
	}
	if request.Body.SourceProviderId == uuid.Nil {
		return nil, apierrors.NewBadRequestError(msgSourceProviderIDRequired)
	}
	if err := validateCopyModelProviderOverrides(request.Body); err != nil {
		return nil, err
	}

	provider, err := m.s.container.ModelProviderService().CopyFromTeam(ctx, services.CopyModelProviderParams{
		TeamID:           teamID,
		SourceTeamID:     request.Body.SourceTeamId.String(),
		SourceProviderID: request.Body.SourceProviderId.String(),
		UserID:           userID,
		Name:             request.Body.Name,
		ProviderType:     request.Body.ProviderType,
		Model:            request.Body.Model,
		BaseURL:          request.Body.BaseUrl,
		Configuration:    request.Body.Configuration,
	})
	if err != nil {
		return nil, m.copyError(teamID, err)
	}

	return modelprovidersgen.CopyModelProviderFromTeam200JSONResponse(
		toGenModelProviderResponse(modelProviderResponseFromRow(provider)),
	), nil
}

// copyError maps a service failure to its documented status. The
// permission-denied branch is checked BEFORE the generic mapping, exactly as
// writeIfPermissionDenied does for the chi handlers in this domain — otherwise
// an authorization failure is reported as a generic "copy failed" 500.
func (m *modelProvidersStrictServer) copyError(teamID string, err error) error {
	switch {
	case errors.Is(err, services.ErrPermissionDenied):
		return apierrors.NewForbiddenError(modelProviderMsgCopyForbidden)
	case errors.Is(err, services.ErrCopySourceRequired),
		errors.Is(err, services.ErrCopySourceIsDestination):
		return apierrors.NewBadRequestError(err.Error())
	case errors.Is(err, services.ErrModelProviderNotFound):
		return apierrors.NewModelProviderNotFoundError("")
	case errors.Is(err, services.ErrModelProviderAlreadyExists):
		return apierrors.NewModelProviderAlreadyExistsError("")
	}

	m.s.logger.With(
		"service", serverLogServiceName,
		"handler", "CopyModelProviderFromTeam",
		"team_id", teamID,
		"error", err.Error(),
	).Error(modelProviderMsgCopyFailed)
	return apierrors.NewInternalError(modelProviderMsgCopyFailed)
}

// validateCopyModelProviderOverrides repeats, for the copy, the required-field
// check the create handler does and the service does not: an override that is
// PRESENT must be non-empty. Absent overrides are the normal case — they mean
// "copy the source's value" — so only a sent-but-empty one is an error, and it
// reports through the same MODEL_PROVIDER_VALIDATION_FAILED payload create uses.
func validateCopyModelProviderOverrides(body *modelprovidersgen.CopyModelProviderRequest) error {
	var validationErrors []apierrors.ValidationError

	for _, field := range []struct {
		name  string
		value *string
	}{
		{"name", body.Name},
		{"provider_type", body.ProviderType},
		{"model", body.Model},
	} {
		if field.value != nil && *field.value == "" {
			validationErrors = append(validationErrors, apierrors.NewRequiredFieldError(field.name))
		}
	}

	if len(validationErrors) > 0 {
		return apierrors.NewModelProviderValidationError(
			"Model provider validation failed. Please check the required fields.",
			validationErrors,
		)
	}
	return nil
}

// modelProvidersBindErrorHandler translates parameter- and body-binding
// failures from the generated wrapper into RFC 9457 problem details.
//
// A body that fails to decode keeps the exact message the hand-written decoders
// used (#837), so clients that matched on it are unaffected by the conversion —
// and the generated decoder's own wording ("can't decode JSON body: unexpected
// EOF", "invalid UUID length: 10") stays out of the API.
func (s *Server) modelProvidersBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var invalidParam *modelprovidersgen.InvalidParamFormatError
	if errors.As(err, &invalidParam) {
		if invalidParam.ParamName == "team_id" {
			apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("team_id must be a valid UUID"))
			return
		}
		apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(err.Error()))
		return
	}

	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON))
}

// modelProvidersResponseErrorHandler writes errors returned by the strict
// handler as RFC 9457 problem details. The generated typed error responses
// would write application/json, so they are deliberately bypassed.
func (s *Server) modelProvidersResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With("error", err).Error("Model providers strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Internal server error"))
}
