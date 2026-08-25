package server

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	embeddingprovidersgen "github.com/vibexp/vibexp/internal/server/gen/embeddingproviders"
	"github.com/vibexp/vibexp/internal/services"
)

// Problem details for the cross-team embedding-provider copy (#831, epic #827).
const (
	embeddingProviderMsgCopyFailed = "Failed to copy the embedding provider"
	// embeddingProviderMsgCopyForbidden is the ONE message a denied copy ever
	// returns, whichever team the caller was refused on. Naming the team would
	// tell a caller entitled to neither whether the source team exists (#829).
	embeddingProviderMsgCopyForbidden = "You must have permission to manage provider settings " +
		"in both the source and the destination team"
	msgCopySourceTeamIDRequired     = "source_team_id is required"
	msgCopySourceProviderIDRequired = "source_provider_id is required"
)

// CopyEmbeddingProviderFromTeam handles
// POST /api/v1/{team_id}/settings/embedding-providers/copy
//
// The re-embed lives here rather than in the service on purpose: enqueueTeamReembed
// is a Server-level seam (it owns the per-team in-flight guard and the background
// goroutine), and the copy must not block on a regeneration that can take minutes.
func (e *embeddingProvidersStrictServer) CopyEmbeddingProviderFromTeam(
	ctx context.Context, request embeddingprovidersgen.CopyEmbeddingProviderFromTeamRequestObject,
) (embeddingprovidersgen.CopyEmbeddingProviderFromTeamResponseObject, error) {
	teamID := request.TeamId.String()

	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}
	if apiErr := e.validateCopyBody(request.Body, userID); apiErr != nil {
		return nil, apiErr
	}

	result, err := e.s.container.EmbeddingProviderService().
		CopyFromTeam(ctx, copyParamsFromGen(teamID, userID, request.Body))
	if err != nil {
		return nil, e.copyError(teamID, err)
	}

	activation := result.Activation
	// WIPE only when the team's embedding model actually moved. Deleting a team's
	// vectors is the one destructive act in this epic, so it is spent exactly
	// where it buys something: vectors produced by a different model are not
	// comparable to new queries, whereas a copy that changed only credentials or
	// the endpoint leaves every stored vector valid. Anything else fills gaps.
	wipe := activation.BecomesActive && activation.ModelChanged

	// Report what HAPPENED, not what was asked for: enqueueTeamReembed drops a
	// duplicate while a re-embed is already in flight for the team, and abandons
	// the run if the wipe itself fails. Echoing the request flag would claim a
	// deletion that never took place.
	enqueued := false
	if request.Body.Reprocess != nil && *request.Body.Reprocess {
		enqueued = e.s.enqueueTeamReembed(teamID, wipe)
	}

	e.s.logger.With(
		"service", serverLogServiceName,
		"handler", "CopyEmbeddingProviderFromTeam",
		"user_id", userID,
		"team_id", teamID,
		"provider_id", result.Provider.ID,
		"becomes_active", activation.BecomesActive,
		"model_changed", activation.ModelChanged,
		"reprocess_enqueued", enqueued,
	).Info("Embedding provider copied from another team")

	return embeddingprovidersgen.CopyEmbeddingProviderFromTeam200JSONResponse(
		embeddingprovidersgen.CopyEmbeddingProviderResponse{
			Provider: toGenEmbeddingProviderResponse(models.EmbeddingProviderResponse{
				EmbeddingProvider: *result.Provider,
				HasAPIKey:         hasAPIKey(result.Provider),
			}),
			Activation: embeddingprovidersgen.EmbeddingProviderCopyActivation{
				BecomesActive:              activation.BecomesActive,
				DisplacedModel:             activation.DisplacedModel,
				DisplacedEmbeddedResources: activation.DisplacedEmbeddedResources,
				ReprocessEnqueued:          enqueued,
				EmbeddingsWiped:            enqueued && wipe,
			},
		},
	), nil
}

// validateCopyBody runs every check the generated binder does not, returning the
// RFC 9457 error to send or nil when the body is acceptable.
func (e *embeddingProvidersStrictServer) validateCopyBody(
	body *embeddingprovidersgen.CopyEmbeddingProviderRequest, userID string,
) error {
	if body == nil {
		return apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}
	// oapi-codegen enforces neither `required` nor additionalProperties on a
	// request body, so `{}` binds both uuids as the ZERO uuid rather than
	// failing. Left unguarded they reach the service, fail authorization against
	// a team that cannot exist, and answer 403 — where the spec documents 400,
	// and a 403 would imply the all-zero team is merely one the caller lacks
	// access to (#829).
	if body.SourceTeamId == uuid.Nil {
		return apierrors.NewBadRequestError(msgCopySourceTeamIDRequired)
	}
	if body.SourceProviderId == uuid.Nil {
		return apierrors.NewBadRequestError(msgCopySourceProviderIDRequired)
	}
	if apiErr := validateCopyEmbeddingProviderOverrides(body); apiErr != nil {
		e.logValidationFailure("CopyEmbeddingProviderFromTeam", userID)
		return apiErr
	}
	return nil
}

// copyParamsFromGen maps the validated request body onto the service's params.
func copyParamsFromGen(
	teamID, userID string, body *embeddingprovidersgen.CopyEmbeddingProviderRequest,
) services.CopyEmbeddingProviderParams {
	return services.CopyEmbeddingProviderParams{
		TeamID:           teamID,
		SourceTeamID:     body.SourceTeamId.String(),
		SourceProviderID: body.SourceProviderId.String(),
		UserID:           userID,
		Name:             body.Name,
		ProviderType:     body.ProviderType,
		Model:            body.Model,
		BaseURL:          body.BaseUrl,
		ChunkSize:        body.ChunkSize,
		ChunkOverlap:     body.ChunkOverlap,
		Concurrency:      body.Concurrency,
		QueryPrefix:      body.QueryPrefix,
		DocumentPrefix:   body.DocumentPrefix,
		Configuration:    body.Configuration,
	}
}

// copyError maps a service failure to its documented status. The
// permission-denied branch is checked BEFORE the generic mapping, exactly as
// providerPermissionError does for the rest of this domain — otherwise an
// authorization failure is reported as a generic "copy failed" 500.
func (e *embeddingProvidersStrictServer) copyError(teamID string, err error) error {
	if forbidden := copyForbiddenError(err); forbidden != nil {
		return forbidden
	}
	switch {
	case stderrors.Is(err, services.ErrCopySourceRequired),
		stderrors.Is(err, services.ErrCopySourceIsDestination):
		return apierrors.NewBadRequestError(err.Error())
	case stderrors.Is(err, services.ErrProviderNotFound):
		return apierrors.NewProviderNotFoundError("")
	case stderrors.Is(err, services.ErrProviderAlreadyExists):
		return apierrors.NewProviderAlreadyExistsError("")
	}

	e.s.logger.With(
		"service", serverLogServiceName,
		"handler", "CopyEmbeddingProviderFromTeam",
		"team_id", teamID,
		"error", fmt.Sprintf("%+v", err),
	).Error(embeddingProviderMsgCopyFailed)
	return apierrors.NewInternalError(embeddingProviderMsgCopyFailed)
}

// copyForbiddenError maps a cross-team denial to the copy's own 403 message,
// which — unlike this domain's ordinary provider 403 — must not say which team
// was refused.
func copyForbiddenError(err error) error {
	if !stderrors.Is(err, services.ErrPermissionDenied) {
		return nil
	}
	return apierrors.NewForbiddenError(embeddingProviderMsgCopyForbidden)
}

// validateCopyEmbeddingProviderOverrides repeats, for the copy, the checks the
// create path does and the service does not: an override that is PRESENT must be
// non-empty, and the instruction prefixes stay under the 256-RUNE cap the create
// and update paths enforce. Absent overrides are the normal case — they mean
// "copy the source's value" — so only a sent one is examined, and the failure
// reports through the same PROVIDER_VALIDATION_FAILED payload create uses.
//
// The prefixes are checked with appendPrefixLengthErrors, the same helper the
// create and update handlers call, so a change to the cap cannot leave the copy
// path behind.
func validateCopyEmbeddingProviderOverrides(body *embeddingprovidersgen.CopyEmbeddingProviderRequest) error {
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

	validationErrors = appendPrefixLengthErrors(validationErrors, body.QueryPrefix, body.DocumentPrefix)

	if len(validationErrors) == 0 {
		return nil
	}
	return apierrors.NewProviderValidationError(
		"Embedding provider validation failed. Please check the copy overrides.",
		validationErrors,
	)
}
