package server

import (
	"context"
	stderrors "errors"
	"fmt"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	modelprovidersgen "github.com/vibexp/vibexp/internal/server/gen/modelproviders"
	"github.com/vibexp/vibexp/internal/services"
)

// The six model-provider CRUD/validate operations of this domain (issue #837,
// epic #122), converted from hand-marshaled chi handlers to the generated
// modelprovidersgen.StrictServerInterface. Each is mounted twice — bare and
// under `settings/` — so the twelve generated methods below are thin adapters
// over one shared implementation per operation and the pair cannot drift.
//
// The strict server itself (modelProvidersStrictServer) and its two transport
// error handlers already existed for the cross-team copy (#830) and are reused
// unchanged; see model_provider_copy_handlers.go.
//
// This is a refactor, not a contract change: every status code, error code and
// response body is the one the deleted handlers produced. Errors are returned
// as *apierrors.APIError and written by modelProvidersResponseErrorHandler,
// which keeps the RFC 9457 bodies and the MODEL_PROVIDER_* codes intact.

// Validation detail strings. Create and validate deliberately differ — the two
// hand-written validators used different wording and clients may match on it.
const (
	msgModelProviderValidationFailed = "Model provider validation failed. Please check the required fields."
	msgModelProviderProbeIncomplete  = "Provider validation request is missing required fields"
)

// --- Bare mount (/api/v1/{team_id}/model-providers) ---------------------------

func (m *modelProvidersStrictServer) CreateModelProvider(
	ctx context.Context, request modelprovidersgen.CreateModelProviderRequestObject,
) (modelprovidersgen.CreateModelProviderResponseObject, error) {
	provider, err := m.create(ctx, request.TeamId.String(), request.Body)
	if err != nil {
		return nil, err
	}
	return modelprovidersgen.CreateModelProvider200JSONResponse(provider), nil
}

func (m *modelProvidersStrictServer) ListModelProviders(
	ctx context.Context, request modelprovidersgen.ListModelProvidersRequestObject,
) (modelprovidersgen.ListModelProvidersResponseObject, error) {
	providers, err := m.list(ctx, request.TeamId.String())
	if err != nil {
		return nil, err
	}
	return modelprovidersgen.ListModelProviders200JSONResponse(providers), nil
}

func (m *modelProvidersStrictServer) GetModelProvider(
	ctx context.Context, request modelprovidersgen.GetModelProviderRequestObject,
) (modelprovidersgen.GetModelProviderResponseObject, error) {
	provider, err := m.get(ctx, request.TeamId.String(), request.Id)
	if err != nil {
		return nil, err
	}
	return modelprovidersgen.GetModelProvider200JSONResponse(provider), nil
}

func (m *modelProvidersStrictServer) UpdateModelProvider(
	ctx context.Context, request modelprovidersgen.UpdateModelProviderRequestObject,
) (modelprovidersgen.UpdateModelProviderResponseObject, error) {
	provider, err := m.update(ctx, request.TeamId.String(), request.Id, request.Body)
	if err != nil {
		return nil, err
	}
	return modelprovidersgen.UpdateModelProvider200JSONResponse(provider), nil
}

func (m *modelProvidersStrictServer) DeleteModelProvider(
	ctx context.Context, request modelprovidersgen.DeleteModelProviderRequestObject,
) (modelprovidersgen.DeleteModelProviderResponseObject, error) {
	if err := m.delete(ctx, request.TeamId.String(), request.Id); err != nil {
		return nil, err
	}
	return modelprovidersgen.DeleteModelProvider204Response{}, nil
}

func (m *modelProvidersStrictServer) ValidateModelProvider(
	ctx context.Context, request modelprovidersgen.ValidateModelProviderRequestObject,
) (modelprovidersgen.ValidateModelProviderResponseObject, error) {
	result, err := m.validate(ctx, request.TeamId.String(), request.Body)
	if err != nil {
		return nil, err
	}
	return modelprovidersgen.ValidateModelProvider200JSONResponse(result), nil
}

// --- Settings mount (/api/v1/{team_id}/settings/model-providers) --------------

func (m *modelProvidersStrictServer) CreateModelProviderSettings(
	ctx context.Context, request modelprovidersgen.CreateModelProviderSettingsRequestObject,
) (modelprovidersgen.CreateModelProviderSettingsResponseObject, error) {
	provider, err := m.create(ctx, request.TeamId.String(), request.Body)
	if err != nil {
		return nil, err
	}
	return modelprovidersgen.CreateModelProviderSettings200JSONResponse(provider), nil
}

func (m *modelProvidersStrictServer) ListModelProvidersSettings(
	ctx context.Context, request modelprovidersgen.ListModelProvidersSettingsRequestObject,
) (modelprovidersgen.ListModelProvidersSettingsResponseObject, error) {
	providers, err := m.list(ctx, request.TeamId.String())
	if err != nil {
		return nil, err
	}
	return modelprovidersgen.ListModelProvidersSettings200JSONResponse(providers), nil
}

func (m *modelProvidersStrictServer) GetModelProviderSettings(
	ctx context.Context, request modelprovidersgen.GetModelProviderSettingsRequestObject,
) (modelprovidersgen.GetModelProviderSettingsResponseObject, error) {
	provider, err := m.get(ctx, request.TeamId.String(), request.Id)
	if err != nil {
		return nil, err
	}
	return modelprovidersgen.GetModelProviderSettings200JSONResponse(provider), nil
}

func (m *modelProvidersStrictServer) UpdateModelProviderSettings(
	ctx context.Context, request modelprovidersgen.UpdateModelProviderSettingsRequestObject,
) (modelprovidersgen.UpdateModelProviderSettingsResponseObject, error) {
	provider, err := m.update(ctx, request.TeamId.String(), request.Id, request.Body)
	if err != nil {
		return nil, err
	}
	return modelprovidersgen.UpdateModelProviderSettings200JSONResponse(provider), nil
}

func (m *modelProvidersStrictServer) DeleteModelProviderSettings(
	ctx context.Context, request modelprovidersgen.DeleteModelProviderSettingsRequestObject,
) (modelprovidersgen.DeleteModelProviderSettingsResponseObject, error) {
	if err := m.delete(ctx, request.TeamId.String(), request.Id); err != nil {
		return nil, err
	}
	return modelprovidersgen.DeleteModelProviderSettings204Response{}, nil
}

func (m *modelProvidersStrictServer) ValidateModelProviderSettings(
	ctx context.Context, request modelprovidersgen.ValidateModelProviderSettingsRequestObject,
) (modelprovidersgen.ValidateModelProviderSettingsResponseObject, error) {
	result, err := m.validate(ctx, request.TeamId.String(), request.Body)
	if err != nil {
		return nil, err
	}
	return modelprovidersgen.ValidateModelProviderSettings200JSONResponse(result), nil
}

// --- Shared implementations ---------------------------------------------------

// create is the shared implementation behind CreateModelProvider and its
// settings twin. It answers 200 (not 201), as the spec documents.
func (m *modelProvidersStrictServer) create(
	ctx context.Context, teamID string, body *modelprovidersgen.CreateModelProviderJSONRequestBody,
) (modelprovidersgen.ModelProviderResponse, error) {
	var zero modelprovidersgen.ModelProviderResponse

	userID, err := authedUserID(ctx)
	if err != nil {
		return zero, err
	}
	if body == nil {
		return zero, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}

	req := createModelProviderRequestFromGen(body)
	// The spec's `required` keywords are not enforced by the generated decoder,
	// so the field-level validation stays here and keeps producing
	// MODEL_PROVIDER_VALIDATION_FAILED with per-field validation_errors.
	if apiErr := validateCreateModelProviderFields(&req); apiErr != nil {
		m.logValidationFailure("CreateModelProvider", userID)
		return zero, apiErr
	}

	provider, err := m.s.container.ModelProviderService().CreateModelProvider(ctx, teamID, userID, req)
	if err != nil {
		return zero, m.createError(userID, req.Name, err)
	}

	m.s.logger.With(
		"service", serverLogServiceName,
		"handler", "CreateModelProvider",
		"user_id", userID,
		"provider_id", provider.ID,
		"name", req.Name,
	).Info("Model provider created successfully")

	return toGenModelProviderResponse(modelProviderResponseFromRow(provider)), nil
}

func (m *modelProvidersStrictServer) createError(userID, name string, err error) error {
	if forbidden := providerPermissionError(err); forbidden != nil {
		return forbidden
	}
	m.s.logger.With(
		"service", serverLogServiceName,
		"handler", "CreateModelProvider",
		"user_id", userID,
		"name", name,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to create model provider")

	if stderrors.Is(err, services.ErrModelProviderAlreadyExists) {
		return apierrors.NewModelProviderAlreadyExistsError(name)
	}
	return apierrors.NewModelProviderCreateFailedError(
		"Unable to create model provider. Please check your configuration and try again.",
	)
}

// list is the shared implementation behind ListModelProviders and its settings
// twin. It is authorized by tenancy alone, so it has no permission-denied arm.
func (m *modelProvidersStrictServer) list(
	ctx context.Context, teamID string,
) ([]modelprovidersgen.ModelProviderResponse, error) {
	providers, err := m.s.container.ModelProviderService().GetModelProvidersByTeamID(ctx, teamID)
	if err != nil {
		m.s.logger.With(
			"service", serverLogServiceName,
			"handler", "ListModelProviders",
			"user_id", loggableUserID(ctx),
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get model providers")
		return nil, apierrors.NewDatabaseError(
			"Failed to retrieve model providers. Please try again later.",
		)
	}

	// The response is a bare JSON array, so an empty result must marshal as `[]`
	// and never `null`. Generated response types cannot use models.JSONArray[T],
	// so the guarantee is made here at the single construction site (#125).
	out := make([]modelprovidersgen.ModelProviderResponse, 0, len(providers))
	for i := range providers {
		out = append(out, toGenModelProviderResponse(providers[i]))
	}
	return out, nil
}

// get is the shared implementation behind GetModelProvider and its settings
// twin. Like list, it is authorized by tenancy alone.
func (m *modelProvidersStrictServer) get(
	ctx context.Context, teamID, providerID string,
) (modelprovidersgen.ModelProviderResponse, error) {
	var zero modelprovidersgen.ModelProviderResponse

	if apiErr := requireModelProviderID(providerID); apiErr != nil {
		return zero, apiErr
	}

	provider, err := m.s.container.ModelProviderService().GetModelProvider(ctx, teamID, providerID)
	if err != nil {
		m.s.logger.With(
			"service", serverLogServiceName,
			"handler", "GetModelProvider",
			"user_id", loggableUserID(ctx),
			"provider_id", providerID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get model provider")
		if stderrors.Is(err, services.ErrModelProviderNotFound) {
			return zero, apierrors.NewModelProviderNotFoundError(providerID)
		}
		return zero, apierrors.NewDatabaseError(fmt.Sprintf(
			"Failed to retrieve model provider '%s'. Please try again later.", providerID,
		))
	}

	return toGenModelProviderResponse(*provider), nil
}

// update is the shared implementation behind UpdateModelProvider and its
// settings twin. Note there is deliberately no 409 arm: the spec documents none
// on this operation, so a rename collision keeps falling through to
// MODEL_PROVIDER_UPDATE_FAILED exactly as it did before the conversion.
func (m *modelProvidersStrictServer) update(
	ctx context.Context,
	teamID, providerID string,
	body *modelprovidersgen.UpdateModelProviderJSONRequestBody,
) (modelprovidersgen.ModelProviderResponse, error) {
	var zero modelprovidersgen.ModelProviderResponse

	userID, err := authedUserID(ctx)
	if err != nil {
		return zero, err
	}
	if apiErr := requireModelProviderID(providerID); apiErr != nil {
		return zero, apiErr
	}
	if body == nil {
		return zero, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}

	req := updateModelProviderRequestFromGen(body)
	provider, err := m.s.container.ModelProviderService().
		UpdateModelProvider(ctx, teamID, userID, providerID, req)
	if err != nil {
		return zero, m.updateError(userID, providerID, err)
	}

	m.s.logger.With(
		"service", serverLogServiceName,
		"handler", "UpdateModelProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Model provider updated successfully")

	return toGenModelProviderResponse(modelProviderResponseFromRow(provider)), nil
}

func (m *modelProvidersStrictServer) updateError(userID, providerID string, err error) error {
	if forbidden := providerPermissionError(err); forbidden != nil {
		return forbidden
	}
	m.s.logger.With(
		"service", serverLogServiceName,
		"handler", "UpdateModelProvider",
		"user_id", userID,
		"provider_id", providerID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to update model provider")

	if stderrors.Is(err, services.ErrModelProviderNotFound) {
		return apierrors.NewModelProviderNotFoundError(providerID)
	}
	return apierrors.NewModelProviderUpdateFailedError(
		"Unable to update model provider. Please check your configuration and try again.",
	)
}

// delete is the shared implementation behind DeleteModelProvider and its
// settings twin.
func (m *modelProvidersStrictServer) delete(ctx context.Context, teamID, providerID string) error {
	userID, err := authedUserID(ctx)
	if err != nil {
		return err
	}
	if apiErr := requireModelProviderID(providerID); apiErr != nil {
		return apiErr
	}

	if err := m.s.container.ModelProviderService().
		DeleteModelProvider(ctx, teamID, userID, providerID); err != nil {
		return m.deleteError(userID, providerID, err)
	}

	m.s.logger.With(
		"service", serverLogServiceName,
		"handler", "DeleteModelProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Model provider deleted successfully")
	return nil
}

func (m *modelProvidersStrictServer) deleteError(userID, providerID string, err error) error {
	if forbidden := providerPermissionError(err); forbidden != nil {
		return forbidden
	}
	m.s.logger.With(
		"service", serverLogServiceName,
		"handler", "DeleteModelProvider",
		"user_id", userID,
		"provider_id", providerID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to delete model provider")

	switch {
	case stderrors.Is(err, services.ErrModelProviderNotFound):
		return apierrors.NewModelProviderNotFoundError(providerID)
	case stderrors.Is(err, services.ErrLastModelProviderDelete):
		return apierrors.NewModelProviderLastDeleteBlockedError()
	default:
		return apierrors.NewModelProviderDeleteFailedError(
			"Unable to delete model provider. Please try again later.",
		)
	}
}

// validate is the shared implementation behind ValidateModelProvider and its
// settings twin. It probes a caller-supplied base_url, so it is gated in the
// service (#464) rather than by tenancy alone. A provider that is reachable but
// misconfigured is reported in the 200 body, not as an error.
func (m *modelProvidersStrictServer) validate(
	ctx context.Context, teamID string, body *modelprovidersgen.ValidateModelProviderJSONRequestBody,
) (modelprovidersgen.ValidateModelProviderResponse, error) {
	var zero modelprovidersgen.ValidateModelProviderResponse

	userID, err := authedUserID(ctx)
	if err != nil {
		return zero, err
	}
	if body == nil {
		return zero, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}

	req := validateModelProviderRequestFromGen(body)
	if apiErr := validateModelProviderProbeFields(&req); apiErr != nil {
		m.logValidationFailure("ValidateModelProvider", userID)
		return zero, apiErr
	}

	result, err := m.s.container.ModelProviderService().ValidateModelProvider(ctx, teamID, userID, req)
	if err != nil {
		if forbidden := providerPermissionError(err); forbidden != nil {
			return zero, forbidden
		}
		m.s.logger.With(
			"service", serverLogServiceName,
			"handler", "ValidateModelProvider",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to validate model provider")
		// Service errors (network issues, etc.) are internal errors — don't
		// expose the raw error.
		return zero, apierrors.NewInternalError(
			"Provider validation failed due to a service error. Please try again later.",
		)
	}

	m.s.logger.With(
		"service", serverLogServiceName,
		"handler", "ValidateModelProvider",
		"user_id", userID,
		"is_valid", result.IsValid,
		"message", result.Message,
	).Info("Model provider validation completed")

	return toGenValidateModelProviderResponse(result), nil
}

func (m *modelProvidersStrictServer) logValidationFailure(handler, userID string) {
	m.s.logger.With(
		"service", serverLogServiceName,
		"handler", handler,
		"user_id", userID,
	).Error("Model provider request validation failed")
}

// --- Validation ---------------------------------------------------------------

// validateCreateModelProviderFields enforces the create body's required fields,
// returning the RFC 9457 error to send or nil when the body is acceptable. Pure
// over the decoded request so the rule is testable on its own.
func validateCreateModelProviderFields(req *models.CreateModelProviderRequest) error {
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

	if len(validationErrors) == 0 {
		return nil
	}
	return apierrors.NewModelProviderValidationError(msgModelProviderValidationFailed, validationErrors)
}

// validateModelProviderProbeFields enforces the validate-probe body's required
// fields. Its detail string differs from the create path's on purpose.
func validateModelProviderProbeFields(req *models.ValidateModelProviderRequest) error {
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
	return apierrors.NewModelProviderValidationError(msgModelProviderProbeIncomplete, validationErrors)
}

// requireModelProviderID rejects an empty {id} path segment with the same
// MODEL_PROVIDER_VALIDATION_FAILED body the chi handlers produced. chi cannot
// actually route an empty segment to these operations, so this is a defensive
// guard kept for parity.
func requireModelProviderID(providerID string) error {
	if providerID != "" {
		return nil
	}
	return apierrors.NewModelProviderValidationError(
		msgProviderIDRequiredInPath,
		[]apierrors.ValidationError{apierrors.NewRequiredFieldError("id")},
	)
}

// --- Conversion ----------------------------------------------------------------

// modelProviderResponseFromRow lifts a persisted row into the domain read model
// the create/update/copy service methods do not return. has_api_key is the only
// signal the stored key leaves in a payload.
func modelProviderResponseFromRow(provider *models.ModelProvider) models.ModelProviderResponse {
	return models.ModelProviderResponse{
		ModelProvider: *provider,
		HasAPIKey:     provider.APIKeyEncrypted != nil && *provider.APIKeyEncrypted != "",
	}
}

func createModelProviderRequestFromGen(
	body *modelprovidersgen.CreateModelProviderJSONRequestBody,
) models.CreateModelProviderRequest {
	return models.CreateModelProviderRequest{
		Name:          body.Name,
		ProviderType:  body.ProviderType,
		Model:         body.Model,
		IsDefault:     body.IsDefault,
		BaseURL:       body.BaseUrl,
		APIKey:        body.ApiKey,
		Configuration: derefConfiguration(body.Configuration),
	}
}

func updateModelProviderRequestFromGen(
	body *modelprovidersgen.UpdateModelProviderJSONRequestBody,
) models.UpdateModelProviderRequest {
	return models.UpdateModelProviderRequest{
		Name:          body.Name,
		ProviderType:  body.ProviderType,
		Model:         body.Model,
		IsDefault:     body.IsDefault,
		BaseURL:       body.BaseUrl,
		APIKey:        body.ApiKey,
		Configuration: derefConfiguration(body.Configuration),
	}
}

func validateModelProviderRequestFromGen(
	body *modelprovidersgen.ValidateModelProviderJSONRequestBody,
) models.ValidateModelProviderRequest {
	return models.ValidateModelProviderRequest{
		ProviderType:  body.ProviderType,
		Model:         body.Model,
		BaseURL:       body.BaseUrl,
		APIKey:        body.ApiKey,
		Configuration: derefConfiguration(body.Configuration),
	}
}

// toGenModelProviderResponse converts the domain read model to the generated
// response type. api_key_encrypted has no counterpart in the generated struct,
// so the encrypted key cannot leak by construction — the hand-marshaled path
// had to nil it out explicitly.
func toGenModelProviderResponse(
	provider models.ModelProviderResponse,
) modelprovidersgen.ModelProviderResponse {
	return modelprovidersgen.ModelProviderResponse{
		Id:            provider.ID,
		UserId:        provider.UserID,
		TeamId:        parseOptionalUUID(provider.TeamID),
		Name:          provider.Name,
		ProviderType:  provider.ProviderType,
		Model:         provider.Model,
		IsDefault:     provider.IsDefault,
		BaseUrl:       provider.BaseURL,
		Configuration: provider.Configuration,
		CreatedAt:     provider.CreatedAt,
		UpdatedAt:     provider.UpdatedAt,
		Version:       provider.Version,
		HasApiKey:     provider.HasAPIKey,
	}
}

// toGenValidateModelProviderResponse converts the probe result. The `details`
// object is always emitted (as it was by the hand-marshaled path — `omitempty`
// on a struct field does nothing) and each field inside it is omitted when zero,
// so the payload is unchanged. Unlike embedding validation there is no
// `dimension`: a model provider is accepted on reachability + auth alone.
func toGenValidateModelProviderResponse(
	result *models.ValidateModelProviderResponse,
) modelprovidersgen.ValidateModelProviderResponse {
	out := modelprovidersgen.ValidateModelProviderResponse{
		IsValid: result.IsValid,
		Message: result.Message,
	}
	out.Details = &struct {
		ErrorDetails   *string `json:"error_details,omitempty"`
		ResponseTimeMs *int    `json:"response_time_ms,omitempty"`
		StatusCode     *int    `json:"status_code,omitempty"`
	}{
		ErrorDetails:   optionalString(result.Details.ErrorDetails),
		ResponseTimeMs: optionalInt(result.Details.ResponseTime),
		StatusCode:     optionalInt(result.Details.StatusCode),
	}
	return out
}
