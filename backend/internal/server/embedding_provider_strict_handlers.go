package server

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	embeddingprovidersgen "github.com/vibexp/vibexp/internal/server/gen/embeddingproviders"
	"github.com/vibexp/vibexp/internal/services"
)

// embeddingProvidersStrictServer implements the generated
// embeddingprovidersgen.StrictServerInterface (issue #472, epic #122): the six
// embedding-provider CRUD/validate operations, each mounted twice (bare and
// under `settings/`). The two mounts share one unexported implementation per
// operation, so the twelve generated methods below are thin adapters and the
// behaviour of the pair cannot drift.
//
// Errors are returned as *apierrors.APIError and written by
// embeddingProvidersResponseErrorHandler, which keeps the RFC 9457 bodies and
// the PROVIDER_* codes byte-identical to the hand-marshaled handlers this
// replaced.
type embeddingProvidersStrictServer struct {
	s *Server
}

var _ embeddingprovidersgen.StrictServerInterface = (*embeddingProvidersStrictServer)(nil)

// --- Bare mount (/api/v1/{team_id}/embedding-providers) ----------------------

func (e *embeddingProvidersStrictServer) CreateEmbeddingProvider(
	ctx context.Context, request embeddingprovidersgen.CreateEmbeddingProviderRequestObject,
) (embeddingprovidersgen.CreateEmbeddingProviderResponseObject, error) {
	provider, err := e.create(ctx, request.TeamId, request.Body)
	if err != nil {
		return nil, err
	}
	return embeddingprovidersgen.CreateEmbeddingProvider200JSONResponse(provider), nil
}

func (e *embeddingProvidersStrictServer) ListEmbeddingProviders(
	ctx context.Context, request embeddingprovidersgen.ListEmbeddingProvidersRequestObject,
) (embeddingprovidersgen.ListEmbeddingProvidersResponseObject, error) {
	providers, err := e.list(ctx, request.TeamId)
	if err != nil {
		return nil, err
	}
	return embeddingprovidersgen.ListEmbeddingProviders200JSONResponse(providers), nil
}

func (e *embeddingProvidersStrictServer) GetEmbeddingProvider(
	ctx context.Context, request embeddingprovidersgen.GetEmbeddingProviderRequestObject,
) (embeddingprovidersgen.GetEmbeddingProviderResponseObject, error) {
	provider, err := e.get(ctx, request.TeamId, request.Id)
	if err != nil {
		return nil, err
	}
	return embeddingprovidersgen.GetEmbeddingProvider200JSONResponse(provider), nil
}

func (e *embeddingProvidersStrictServer) UpdateEmbeddingProvider(
	ctx context.Context, request embeddingprovidersgen.UpdateEmbeddingProviderRequestObject,
) (embeddingprovidersgen.UpdateEmbeddingProviderResponseObject, error) {
	provider, err := e.update(ctx, request.TeamId, request.Id, request.Body)
	if err != nil {
		return nil, err
	}
	return embeddingprovidersgen.UpdateEmbeddingProvider200JSONResponse(provider), nil
}

func (e *embeddingProvidersStrictServer) DeleteEmbeddingProvider(
	ctx context.Context, request embeddingprovidersgen.DeleteEmbeddingProviderRequestObject,
) (embeddingprovidersgen.DeleteEmbeddingProviderResponseObject, error) {
	if err := e.delete(ctx, request.TeamId, request.Id); err != nil {
		return nil, err
	}
	return embeddingprovidersgen.DeleteEmbeddingProvider204Response{}, nil
}

func (e *embeddingProvidersStrictServer) ValidateEmbeddingProvider(
	ctx context.Context, request embeddingprovidersgen.ValidateEmbeddingProviderRequestObject,
) (embeddingprovidersgen.ValidateEmbeddingProviderResponseObject, error) {
	result, err := e.validate(ctx, request.TeamId, request.Body)
	if err != nil {
		return nil, err
	}
	return embeddingprovidersgen.ValidateEmbeddingProvider200JSONResponse(result), nil
}

// --- Settings mount (/api/v1/{team_id}/settings/embedding-providers) ---------

func (e *embeddingProvidersStrictServer) CreateEmbeddingProviderSettings(
	ctx context.Context, request embeddingprovidersgen.CreateEmbeddingProviderSettingsRequestObject,
) (embeddingprovidersgen.CreateEmbeddingProviderSettingsResponseObject, error) {
	provider, err := e.create(ctx, request.TeamId, request.Body)
	if err != nil {
		return nil, err
	}
	return embeddingprovidersgen.CreateEmbeddingProviderSettings200JSONResponse(provider), nil
}

func (e *embeddingProvidersStrictServer) ListEmbeddingProvidersSettings(
	ctx context.Context, request embeddingprovidersgen.ListEmbeddingProvidersSettingsRequestObject,
) (embeddingprovidersgen.ListEmbeddingProvidersSettingsResponseObject, error) {
	providers, err := e.list(ctx, request.TeamId)
	if err != nil {
		return nil, err
	}
	return embeddingprovidersgen.ListEmbeddingProvidersSettings200JSONResponse(providers), nil
}

func (e *embeddingProvidersStrictServer) GetEmbeddingProviderSettings(
	ctx context.Context, request embeddingprovidersgen.GetEmbeddingProviderSettingsRequestObject,
) (embeddingprovidersgen.GetEmbeddingProviderSettingsResponseObject, error) {
	provider, err := e.get(ctx, request.TeamId, request.Id)
	if err != nil {
		return nil, err
	}
	return embeddingprovidersgen.GetEmbeddingProviderSettings200JSONResponse(provider), nil
}

func (e *embeddingProvidersStrictServer) UpdateEmbeddingProviderSettings(
	ctx context.Context, request embeddingprovidersgen.UpdateEmbeddingProviderSettingsRequestObject,
) (embeddingprovidersgen.UpdateEmbeddingProviderSettingsResponseObject, error) {
	provider, err := e.update(ctx, request.TeamId, request.Id, request.Body)
	if err != nil {
		return nil, err
	}
	return embeddingprovidersgen.UpdateEmbeddingProviderSettings200JSONResponse(provider), nil
}

func (e *embeddingProvidersStrictServer) DeleteEmbeddingProviderSettings(
	ctx context.Context, request embeddingprovidersgen.DeleteEmbeddingProviderSettingsRequestObject,
) (embeddingprovidersgen.DeleteEmbeddingProviderSettingsResponseObject, error) {
	if err := e.delete(ctx, request.TeamId, request.Id); err != nil {
		return nil, err
	}
	return embeddingprovidersgen.DeleteEmbeddingProviderSettings204Response{}, nil
}

func (e *embeddingProvidersStrictServer) ValidateEmbeddingProviderSettings(
	ctx context.Context, request embeddingprovidersgen.ValidateEmbeddingProviderSettingsRequestObject,
) (embeddingprovidersgen.ValidateEmbeddingProviderSettingsResponseObject, error) {
	result, err := e.validate(ctx, request.TeamId, request.Body)
	if err != nil {
		return nil, err
	}
	return embeddingprovidersgen.ValidateEmbeddingProviderSettings200JSONResponse(result), nil
}

// --- Shared implementations --------------------------------------------------

// create is the shared implementation behind CreateEmbeddingProvider and its
// settings twin. A newly added provider enqueues a missing-only team re-embed so
// existing content is populated automatically (see enqueueTeamReembed).
func (e *embeddingProvidersStrictServer) create(
	ctx context.Context,
	teamID openapi_types.UUID,
	body *embeddingprovidersgen.CreateEmbeddingProviderJSONRequestBody,
) (embeddingprovidersgen.EmbeddingProviderResponse, error) {
	var zero embeddingprovidersgen.EmbeddingProviderResponse

	userID, err := authedUserID(ctx)
	if err != nil {
		return zero, err
	}
	if body == nil {
		return zero, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}

	req := createRequestFromGen(body)
	// The spec's `required` keywords are not enforced by the generated decoder,
	// so the field-level validation stays here and keeps producing
	// PROVIDER_VALIDATION_FAILED with per-field validation_errors.
	if apiErr := validateCreateEmbeddingProviderFields(&req); apiErr != nil {
		e.logValidationFailure("CreateEmbeddingProvider", userID)
		return zero, apiErr
	}

	teamIDStr := teamID.String()
	provider, err := e.s.container.EmbeddingProviderService().
		CreateEmbeddingProvider(ctx, teamIDStr, userID, req)
	if err != nil {
		return zero, e.createError(userID, req.Name, err)
	}

	e.s.logger.With(
		"service", serverLogServiceName,
		"handler", "CreateEmbeddingProvider",
		"user_id", userID,
		"provider_id", provider.ID,
		"name", req.Name,
	).Info("Embedding provider created successfully")

	e.s.enqueueTeamReembed(teamIDStr, false)

	return toGenEmbeddingProviderResponse(models.EmbeddingProviderResponse{
		EmbeddingProvider: *provider,
		HasAPIKey:         hasAPIKey(provider),
	}), nil
}

func (e *embeddingProvidersStrictServer) createError(userID, name string, err error) error {
	if forbidden := providerPermissionError(err); forbidden != nil {
		return forbidden
	}
	e.s.logger.With(
		"service", serverLogServiceName,
		"handler", "CreateEmbeddingProvider",
		"user_id", userID,
		"name", name,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to create embedding provider")

	if stderrors.Is(err, services.ErrProviderAlreadyExists) {
		return apierrors.NewProviderAlreadyExistsError(name)
	}
	return apierrors.NewProviderCreateFailedError(
		"Unable to create embedding provider. Please check your configuration and try again.",
	)
}

// list is the shared implementation behind ListEmbeddingProviders and its
// settings twin.
func (e *embeddingProvidersStrictServer) list(
	ctx context.Context, teamID openapi_types.UUID,
) ([]embeddingprovidersgen.EmbeddingProviderResponse, error) {
	providers, err := e.s.container.EmbeddingProviderService().
		GetEmbeddingProvidersByTeamID(ctx, teamID.String())
	if err != nil {
		e.s.logger.With(
			"service", serverLogServiceName,
			"handler", "ListEmbeddingProviders",
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get embedding providers")
		return nil, apierrors.NewDatabaseError(
			"Failed to retrieve embedding providers. Please try again later.",
		)
	}

	// The response is a bare JSON array, so an empty result must marshal as `[]`
	// and never `null`. Generated response types cannot use models.JSONArray[T],
	// so the guarantee is made here at the single construction site (#125).
	out := make([]embeddingprovidersgen.EmbeddingProviderResponse, 0, len(providers))
	for i := range providers {
		out = append(out, toGenEmbeddingProviderResponse(providers[i]))
	}
	return out, nil
}

// get is the shared implementation behind GetEmbeddingProvider and its settings
// twin.
func (e *embeddingProvidersStrictServer) get(
	ctx context.Context, teamID openapi_types.UUID, providerID string,
) (embeddingprovidersgen.EmbeddingProviderResponse, error) {
	var zero embeddingprovidersgen.EmbeddingProviderResponse

	if apiErr := requireProviderID(providerID); apiErr != nil {
		return zero, apiErr
	}

	provider, err := e.s.container.EmbeddingProviderService().
		GetEmbeddingProvider(ctx, teamID.String(), providerID)
	if err != nil {
		e.s.logger.With(
			"service", serverLogServiceName,
			"handler", "GetEmbeddingProvider",
			"provider_id", providerID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get embedding provider")
		if stderrors.Is(err, services.ErrProviderNotFound) {
			return zero, apierrors.NewProviderNotFoundError(providerID)
		}
		return zero, apierrors.NewDatabaseError(fmt.Sprintf(
			"Failed to retrieve embedding provider '%s'. Please try again later.", providerID,
		))
	}

	return toGenEmbeddingProviderResponse(*provider), nil
}

// update is the shared implementation behind UpdateEmbeddingProvider and its
// settings twin. It reads the provider first so an edit that changes the
// embedding identity can trigger a team re-embed.
func (e *embeddingProvidersStrictServer) update(
	ctx context.Context,
	teamID openapi_types.UUID,
	providerID string,
	body *embeddingprovidersgen.UpdateEmbeddingProviderJSONRequestBody,
) (embeddingprovidersgen.EmbeddingProviderResponse, error) {
	var zero embeddingprovidersgen.EmbeddingProviderResponse

	userID, err := authedUserID(ctx)
	if err != nil {
		return zero, err
	}
	if apiErr := requireProviderID(providerID); apiErr != nil {
		return zero, apiErr
	}
	if body == nil {
		return zero, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}

	req := updateRequestFromGen(body)
	if ve := appendPrefixLengthErrors(nil, req.QueryPrefix, req.DocumentPrefix); len(ve) > 0 {
		return zero, apierrors.NewProviderValidationError(
			"Embedding provider validation failed. Please check the prefix fields.", ve,
		)
	}

	teamIDStr := teamID.String()
	// Best-effort: if the pre-update read fails, skip the re-embed check rather
	// than fail the update.
	oldProvider, oldErr := e.s.container.EmbeddingProviderService().
		GetEmbeddingProvider(ctx, teamIDStr, providerID)
	if oldErr != nil {
		oldProvider = nil
	}

	provider, err := e.s.container.EmbeddingProviderService().
		UpdateEmbeddingProvider(ctx, teamIDStr, userID, providerID, req)
	if err != nil {
		return zero, e.updateError(userID, providerID, err)
	}

	e.s.reembedTeamIfProviderIdentityChanged(teamIDStr, oldProvider, provider)

	e.s.logger.With(
		"service", serverLogServiceName,
		"handler", "UpdateEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Embedding provider updated successfully")

	return toGenEmbeddingProviderResponse(models.EmbeddingProviderResponse{
		EmbeddingProvider: *provider,
		HasAPIKey:         hasAPIKey(provider),
	}), nil
}

func (e *embeddingProvidersStrictServer) updateError(userID, providerID string, err error) error {
	if forbidden := providerPermissionError(err); forbidden != nil {
		return forbidden
	}
	e.s.logger.With(
		"service", serverLogServiceName,
		"handler", "UpdateEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to update embedding provider")

	if stderrors.Is(err, services.ErrProviderNotFound) {
		return apierrors.NewProviderNotFoundError(providerID)
	}
	return apierrors.NewProviderUpdateFailedError(
		"Unable to update embedding provider. Please check your configuration and try again.",
	)
}

// delete is the shared implementation behind DeleteEmbeddingProvider and its
// settings twin.
func (e *embeddingProvidersStrictServer) delete(
	ctx context.Context, teamID openapi_types.UUID, providerID string,
) error {
	userID, err := authedUserID(ctx)
	if err != nil {
		return err
	}
	if apiErr := requireProviderID(providerID); apiErr != nil {
		return apiErr
	}

	if err := e.s.container.EmbeddingProviderService().
		DeleteEmbeddingProvider(ctx, teamID.String(), userID, providerID); err != nil {
		return e.deleteError(userID, providerID, err)
	}

	e.s.logger.With(
		"service", serverLogServiceName,
		"handler", "DeleteEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Embedding provider deleted successfully")
	return nil
}

func (e *embeddingProvidersStrictServer) deleteError(userID, providerID string, err error) error {
	if forbidden := providerPermissionError(err); forbidden != nil {
		return forbidden
	}
	e.s.logger.With(
		"service", serverLogServiceName,
		"handler", "DeleteEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to delete embedding provider")

	switch {
	case stderrors.Is(err, services.ErrProviderNotFound):
		return apierrors.NewProviderNotFoundError(providerID)
	case stderrors.Is(err, services.ErrLastProviderDelete):
		return apierrors.NewProviderLastDeleteBlockedError()
	default:
		return apierrors.NewProviderDeleteFailedError(
			"Unable to delete embedding provider. Please try again later.",
		)
	}
}

// validate is the shared implementation behind ValidateEmbeddingProvider and its
// settings twin. It probes a caller-supplied base_url, so it is gated in the
// service (#464) rather than by tenancy alone.
func (e *embeddingProvidersStrictServer) validate(
	ctx context.Context,
	teamID openapi_types.UUID,
	body *embeddingprovidersgen.ValidateEmbeddingProviderJSONRequestBody,
) (embeddingprovidersgen.ValidateEmbeddingProviderResponse, error) {
	var zero embeddingprovidersgen.ValidateEmbeddingProviderResponse

	userID, err := authedUserID(ctx)
	if err != nil {
		return zero, err
	}
	if body == nil {
		return zero, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}

	req := validateRequestFromGen(body)
	if apiErr := validateProbeRequestFields(&req); apiErr != nil {
		e.logValidationFailure("ValidateEmbeddingProvider", userID)
		return zero, apiErr
	}

	result, err := e.s.container.EmbeddingProviderService().
		ValidateEmbeddingProvider(ctx, teamID.String(), userID, req)
	if err != nil {
		if forbidden := providerPermissionError(err); forbidden != nil {
			return zero, forbidden
		}
		e.s.logger.With(
			"service", serverLogServiceName,
			"handler", "ValidateEmbeddingProvider",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to validate embedding provider")
		// Service errors (network issues, etc.) are internal errors — don't
		// expose the raw error.
		return zero, apierrors.NewInternalError(
			"Provider validation failed due to a service error. Please try again later.",
		)
	}

	e.s.logger.With(
		"service", serverLogServiceName,
		"handler", "ValidateEmbeddingProvider",
		"user_id", userID,
		"is_valid", result.IsValid,
		"message", result.Message,
	).Info("Embedding provider validation completed")

	return toGenValidateResponse(result), nil
}

func (e *embeddingProvidersStrictServer) logValidationFailure(handler, userID string) {
	e.s.logger.With(
		"service", serverLogServiceName,
		"handler", handler,
		"user_id", userID,
	).Error("Embedding provider request validation failed")
}

// --- Validation --------------------------------------------------------------

// validateCreateEmbeddingProviderFields enforces the create body's required
// fields and the instruction-prefix cap, returning the RFC 9457 error to send or
// nil when the body is acceptable. Pure over the decoded request so the rule is
// testable on its own.
func validateCreateEmbeddingProviderFields(req *models.CreateEmbeddingProviderRequest) error {
	var validationErrors []apierrors.ValidationError

	if req.Name == "" {
		validationErrors = append(validationErrors, apierrors.NewRequiredFieldError("name"))
	}
	if req.ProviderType == "" {
		validationErrors = append(validationErrors, apierrors.NewRequiredFieldError("provider_type"))
	}
	if req.Model == "" {
		validationErrors = append(validationErrors, apierrors.NewRequiredFieldError("model"))
	}
	validationErrors = appendPrefixLengthErrors(validationErrors, req.QueryPrefix, req.DocumentPrefix)

	if len(validationErrors) == 0 {
		return nil
	}
	return apierrors.NewProviderValidationError(
		"Embedding provider validation failed. Please check the required fields.",
		validationErrors,
	)
}

// validateProbeRequestFields enforces the validate-probe body's required fields.
func validateProbeRequestFields(req *models.ValidateEmbeddingProviderRequest) error {
	var validationErrors []apierrors.ValidationError

	if req.ProviderType == "" {
		validationErrors = append(validationErrors, apierrors.NewRequiredFieldError("provider_type"))
	}
	if req.BaseURL == "" {
		validationErrors = append(validationErrors, apierrors.NewRequiredFieldError("base_url"))
	}
	if req.Model == "" {
		validationErrors = append(validationErrors, apierrors.NewRequiredFieldError("model"))
	}

	if len(validationErrors) == 0 {
		return nil
	}
	return apierrors.NewProviderValidationError(
		"Provider validation request is missing required fields",
		validationErrors,
	)
}

// requireProviderID rejects an empty {id} path segment with the same
// PROVIDER_VALIDATION_FAILED body the chi handlers produced. chi cannot actually
// route an empty segment to these operations, so this is a defensive guard kept
// for parity.
func requireProviderID(providerID string) error {
	if providerID != "" {
		return nil
	}
	return apierrors.NewProviderValidationError(
		msgProviderIDRequiredInPath,
		[]apierrors.ValidationError{apierrors.NewRequiredFieldError("id")},
	)
}

// providerPermissionError maps a service permission failure to this domain's
// 403, mirroring writeIfPermissionDenied for the strict-server path. It returns
// nil when err is not a permission failure, so callers keep their "check this
// first" ordering.
func providerPermissionError(err error) error {
	if !stderrors.Is(err, services.ErrPermissionDenied) {
		return nil
	}
	return apierrors.NewForbiddenError(providerPermissionMessage)
}

// --- Conversion --------------------------------------------------------------

func hasAPIKey(provider *models.EmbeddingProvider) bool {
	return provider.APIKeyEncrypted != nil && *provider.APIKeyEncrypted != ""
}

func createRequestFromGen(
	body *embeddingprovidersgen.CreateEmbeddingProviderJSONRequestBody,
) models.CreateEmbeddingProviderRequest {
	return models.CreateEmbeddingProviderRequest{
		Name:           body.Name,
		ProviderType:   body.ProviderType,
		Model:          body.Model,
		ChunkSize:      body.ChunkSize,
		ChunkOverlap:   body.ChunkOverlap,
		Concurrency:    body.Concurrency,
		QueryPrefix:    body.QueryPrefix,
		DocumentPrefix: body.DocumentPrefix,
		IsDefault:      body.IsDefault,
		BaseURL:        body.BaseUrl,
		APIKey:         body.ApiKey,
		Configuration:  derefConfiguration(body.Configuration),
	}
}

func updateRequestFromGen(
	body *embeddingprovidersgen.UpdateEmbeddingProviderJSONRequestBody,
) models.UpdateEmbeddingProviderRequest {
	return models.UpdateEmbeddingProviderRequest{
		Name:           body.Name,
		ProviderType:   body.ProviderType,
		Model:          body.Model,
		ChunkSize:      body.ChunkSize,
		ChunkOverlap:   body.ChunkOverlap,
		Concurrency:    body.Concurrency,
		QueryPrefix:    body.QueryPrefix,
		DocumentPrefix: body.DocumentPrefix,
		IsDefault:      body.IsDefault,
		BaseURL:        body.BaseUrl,
		APIKey:         body.ApiKey,
		Configuration:  derefConfiguration(body.Configuration),
	}
}

func validateRequestFromGen(
	body *embeddingprovidersgen.ValidateEmbeddingProviderJSONRequestBody,
) models.ValidateEmbeddingProviderRequest {
	return models.ValidateEmbeddingProviderRequest{
		ProviderType:  body.ProviderType,
		Model:         body.Model,
		BaseURL:       body.BaseUrl,
		APIKey:        body.ApiKey,
		Configuration: derefConfiguration(body.Configuration),
	}
}

// derefConfiguration unwraps the generated `*map[string]interface{}` into the
// domain's plain map, keeping nil as nil so an omitted `configuration` is not
// turned into an empty object.
func derefConfiguration(cfg *map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return nil
	}
	return *cfg
}

// toGenEmbeddingProviderResponse converts the domain read model to the generated
// response type. api_key_encrypted has no counterpart in the generated struct,
// so the encrypted key cannot leak by construction — the old hand-marshaled path
// had to nil it out explicitly.
func toGenEmbeddingProviderResponse(
	provider models.EmbeddingProviderResponse,
) embeddingprovidersgen.EmbeddingProviderResponse {
	return embeddingprovidersgen.EmbeddingProviderResponse{
		Id:             provider.ID,
		UserId:         provider.UserID,
		TeamId:         parseOptionalUUID(provider.TeamID),
		Name:           provider.Name,
		ProviderType:   provider.ProviderType,
		Model:          provider.Model,
		ChunkSize:      provider.ChunkSize,
		ChunkOverlap:   provider.ChunkOverlap,
		Concurrency:    provider.Concurrency,
		QueryPrefix:    provider.QueryPrefix,
		DocumentPrefix: provider.DocumentPrefix,
		IsDefault:      provider.IsDefault,
		BaseUrl:        provider.BaseURL,
		Configuration:  provider.Configuration,
		CreatedAt:      provider.CreatedAt,
		UpdatedAt:      provider.UpdatedAt,
		Version:        provider.Version,
		HasApiKey:      provider.HasAPIKey,
	}
}

// parseOptionalUUID converts the stored team id to the generated
// `format: uuid` field. A stored value that is not a UUID is dropped rather than
// emitted, because an invalid uuid would fail spec validation for every caller.
func parseOptionalUUID(value *string) *openapi_types.UUID {
	if value == nil || *value == "" {
		return nil
	}
	parsed, err := uuid.Parse(*value)
	if err != nil {
		return nil
	}
	return &parsed
}

// toGenValidateResponse converts the probe result. The `details` object is
// always emitted (as it was by the hand-marshaled path) and each field inside it
// is omitted when zero, so the payload is unchanged.
func toGenValidateResponse(
	result *models.ValidateEmbeddingProviderResponse,
) embeddingprovidersgen.ValidateEmbeddingProviderResponse {
	out := embeddingprovidersgen.ValidateEmbeddingProviderResponse{
		IsValid: result.IsValid,
		Message: result.Message,
	}
	out.Details = &struct {
		Dimension      *int    `json:"dimension,omitempty"`
		ErrorDetails   *string `json:"error_details,omitempty"`
		ResponseTimeMs *int    `json:"response_time_ms,omitempty"`
		StatusCode     *int    `json:"status_code,omitempty"`
	}{
		Dimension:      optionalInt(result.Details.Dimension),
		ErrorDetails:   optionalString(result.Details.ErrorDetails),
		ResponseTimeMs: optionalInt(result.Details.ResponseTime),
		StatusCode:     optionalInt(result.Details.StatusCode),
	}
	return out
}

func optionalInt(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// --- Transport error handlers -------------------------------------------------

// embeddingProvidersBindErrorHandler translates parameter- and body-binding
// failures from the generated layer into this domain's RFC 9457 400 responses.
// A malformed JSON body keeps the exact message the hand-written decoder used,
// so clients that matched on it are unaffected.
func (s *Server) embeddingProvidersBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var invalidParam *embeddingprovidersgen.InvalidParamFormatError
	if stderrors.As(err, &invalidParam) {
		if invalidParam.ParamName == "team_id" {
			apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("team_id must be a valid UUID"))
			return
		}
		apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(err.Error()))
		return
	}

	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON))
}

// embeddingProvidersResponseErrorHandler writes errors returned by the strict
// handler implementations. *apierrors.APIError carries the intended RFC 9457
// error; anything else is defensive and maps to a generic 500.
func (s *Server) embeddingProvidersResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if stderrors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With("error", err).Error("EmbeddingProviders strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Internal server error"))
}
