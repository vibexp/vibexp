package server

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// maxEmbeddingPrefixLen caps query_prefix / document_prefix. They are prepended
// to every embedded query and document chunk, so a short instruction prefix is
// all that is intended; the cap keeps a stray large value from bloating every
// embed request.
const maxEmbeddingPrefixLen = 256

// appendPrefixLengthErrors appends a max-length validation error for each
// instruction prefix that exceeds maxEmbeddingPrefixLen. A nil pointer (field
// omitted) is skipped. Shared by the create and update handlers so both enforce
// the same cap.
func appendPrefixLengthErrors(
	ve []errors.ValidationError, queryPrefix, documentPrefix *string,
) []errors.ValidationError {
	if queryPrefix != nil && len(*queryPrefix) > maxEmbeddingPrefixLen {
		ve = append(ve, errors.NewMaxLengthError("query_prefix", maxEmbeddingPrefixLen))
	}
	if documentPrefix != nil && len(*documentPrefix) > maxEmbeddingPrefixLen {
		ve = append(ve, errors.NewMaxLengthError("document_prefix", maxEmbeddingPrefixLen))
	}
	return ve
}

// reembedTeamIfProviderIdentityChanged wipes and re-generates a team's embeddings
// when an update changed the provider's embedding identity (model, provider type,
// base URL, or document_prefix). Vectors produced by a different model — or with a
// different document instruction prefix — are not comparable to new queries, so
// the old ones are deleted and the team's entities are re-embedded in the
// background (the update response must not block on a large regeneration). A
// name/default/key/query_prefix-only edit leaves the stored vectors valid and is a
// no-op (query_prefix affects only the query side, never stored documents).
func (s *Server) reembedTeamIfProviderIdentityChanged(
	teamID string, old *models.EmbeddingProviderResponse, updated *models.EmbeddingProvider,
) {
	if old == nil || updated == nil {
		return
	}
	derefStr := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	if old.Model == updated.Model &&
		old.ProviderType == updated.ProviderType &&
		derefStr(old.BaseURL) == derefStr(updated.BaseURL) &&
		derefStr(old.DocumentPrefix) == derefStr(updated.DocumentPrefix) {
		return
	}

	// The identity changed, so the old vectors were produced by a different model
	// and are not comparable to new queries: wipe them and regenerate the team.
	s.enqueueTeamReembed(teamID, true)
}

// enqueueTeamReembed regenerates a team's embeddings in the background through the
// concurrency-bounded embedding path (#142): EmbeddingBackfillService republishes
// each entity's `.created` event, which the inline, per-provider-bounded
// EmbeddingWorker consumes, so a large regeneration never fans out unbounded onto
// the event bus. It is the single enqueue seam behind provider create, an identity
// change on update, and the reprocess action.
//
// When wipe is true the team's existing vectors are deleted first — used when a
// provider change makes the old vectors incomparable to new queries; otherwise
// only entities still missing an embedding are (re)generated. A per-team in-flight
// guard drops overlapping calls so a rapid provider change or a repeated reprocess
// click never stacks duplicate bursts for the same team.
func (s *Server) enqueueTeamReembed(teamID string, wipe bool) {
	logger := s.logger.With(
		"service", "vibexp-api",
		"component", "embedding-reembed",
		"team_id", teamID,
	)

	if _, inFlight := s.reembedInFlight.LoadOrStore(teamID, struct{}{}); inFlight {
		logger.Info("Team re-embed already in flight; skipping duplicate enqueue")
		return
	}

	if wipe {
		deleted, err := s.container.EmbeddingRepository().DeleteByTeam(context.Background(), teamID)
		if err != nil {
			s.reembedInFlight.Delete(teamID)
			logger.With("error", fmt.Sprintf("%+v", err)).
				Error("Failed to wipe team embeddings before re-embed")
			return
		}
		logger.With("deleted", deleted).
			Info("Wiped team embeddings; re-embedding in background")
	}

	go func() {
		defer s.reembedInFlight.Delete(teamID)
		// MissingOnly mirrors the intent: after a wipe every entity is missing, so
		// re-embed all; otherwise (create / reprocess) only fill the gaps.
		if _, err := s.container.EmbeddingBackfillService().Backfill(
			context.Background(),
			services.EmbeddingBackfillRequest{All: true, TeamID: teamID, MissingOnly: !wipe},
		); err != nil {
			logger.With("error", fmt.Sprintf("%+v", err)).
				Error("Background team re-embed failed")
		}
	}()
}

func (s *Server) handleCreateEmbeddingProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateEmbeddingProvider",
		"user_id", userID,
	).Info("Embedding provider creation request received")

	var req models.CreateEmbeddingProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logEmbeddingProviderError("handleCreateEmbeddingProvider", userID, "", err, "Failed to decode request body")
		apiErr := errors.NewBadRequestError(
			"Invalid request body. Please ensure the JSON is well-formed.",
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	if !s.validateCreateEmbeddingProviderRequest(w, r, userID, &req) {
		return
	}

	teamID := chi.URLParam(r, "team_id")
	provider, err := s.container.EmbeddingProviderService().CreateEmbeddingProvider(r.Context(), teamID, userID, req)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateEmbeddingProvider",
			"user_id", userID,
			"name", req.Name,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create embedding provider")

		// Check for duplicate provider error using sentinel error
		if stderrors.Is(err, services.ErrProviderAlreadyExists) {
			errors.WriteJSONError(w, r, errors.NewProviderAlreadyExistsError(req.Name))
			return
		}

		errors.WriteJSONError(w, r, errors.NewProviderCreateFailedError(
			"Unable to create embedding provider. Please check your configuration and try again.",
		))
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateEmbeddingProvider",
		"user_id", userID,
		"provider_id", provider.ID,
		"name", req.Name,
	).Info("Embedding provider created successfully")

	// Enqueue embedding generation for the team's existing entities so a newly
	// added provider populates automatically — previously only an identity change
	// on update re-embedded, so new providers started empty. Missing-only and in
	// the background, riding the #142 bounded path.
	s.enqueueTeamReembed(teamID, false)

	s.writeEmbeddingProviderResponse(w, provider)
}

func (s *Server) validateCreateEmbeddingProviderRequest(
	w http.ResponseWriter,
	r *http.Request,
	userID string,
	req *models.CreateEmbeddingProviderRequest,
) bool {
	var validationErrors []errors.ValidationError

	if req.Name == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateEmbeddingProvider",
			"user_id", userID,
		).Error("Embedding provider name is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("name"))
	}

	if req.ProviderType == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateEmbeddingProvider",
			"user_id", userID,
		).Error("Provider type is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("provider_type"))
	}

	if req.Model == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateEmbeddingProvider",
			"user_id", userID,
		).Error("Model is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("model"))
	}

	validationErrors = appendPrefixLengthErrors(validationErrors, req.QueryPrefix, req.DocumentPrefix)

	if len(validationErrors) > 0 {
		apiErr := errors.NewProviderValidationError(
			"Embedding provider validation failed. Please check the required fields.",
			validationErrors,
		)
		errors.WriteJSONError(w, r, apiErr)
		return false
	}

	return true
}

func (s *Server) logEmbeddingProviderError(
	handler, userID, providerID string,
	err error,
	msg string,
) {
	fields := []any{
		"service", "vibexp-api",
		"handler", handler,
		"user_id", userID,
		"error", fmt.Sprintf("%+v", err),
	}
	if providerID != "" {
		fields = append(fields, "provider_id", providerID)
	}
	s.logger.With(fields...).Error(msg)
}

func (s *Server) writeEmbeddingProviderResponse(w http.ResponseWriter, provider *models.EmbeddingProvider) {
	response := models.EmbeddingProviderResponse{
		EmbeddingProvider: *provider,
		HasAPIKey:         provider.APIKeyEncrypted != nil && *provider.APIKeyEncrypted != "",
	}
	response.APIKeyEncrypted = nil

	writeOK(w, response, s.logger)
}

func (s *Server) handleListEmbeddingProviders(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleListEmbeddingProviders",
		"user_id", userID,
	).Info("Embedding providers list request received")

	teamID := chi.URLParam(r, "team_id")
	providers, err := s.container.EmbeddingProviderService().GetEmbeddingProvidersByTeamID(r.Context(), teamID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListEmbeddingProviders",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get embedding providers")
		errors.WriteJSONError(w, r, errors.NewDatabaseError(
			"Failed to retrieve embedding providers. Please try again later.",
		))
		return
	}

	writeOK(w, providers, s.logger)
}

func (s *Server) handleGetEmbeddingCoverage(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetEmbeddingCoverage",
		"user_id", userID,
	).Info("Embedding coverage request received")

	coverage, err := s.container.EmbeddingStatusService().GetCoverage(r.Context(), teamID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetEmbeddingCoverage",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get embedding coverage")
		errors.WriteJSONError(w, r, errors.NewDatabaseError(
			"Failed to retrieve embedding coverage. Please try again later.",
		))
		return
	}

	writeOK(w, coverage, s.logger)
}

func (s *Server) handleGetEmbeddingProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	providerID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Embedding provider get request received")

	if providerID == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetEmbeddingProvider",
			"user_id", userID,
		).Error("Provider ID is required")
		apiErr := errors.NewProviderValidationError(
			"Provider ID is required in the URL path",
			[]errors.ValidationError{errors.NewRequiredFieldError("id")},
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	provider, err := s.container.EmbeddingProviderService().GetEmbeddingProvider(r.Context(), teamID, providerID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetEmbeddingProvider",
			"user_id", userID,
			"provider_id", providerID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get embedding provider")
		if stderrors.Is(err, services.ErrProviderNotFound) {
			errors.WriteJSONError(w, r, errors.NewProviderNotFoundError(providerID))
			return
		}
		errors.WriteJSONError(w, r, errors.NewDatabaseError(
			fmt.Sprintf("Failed to retrieve embedding provider '%s'. Please try again later.", providerID),
		))
		return
	}

	writeOK(w, provider, s.logger)
}

func (s *Server) handleUpdateEmbeddingProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	providerID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Embedding provider update request received")

	if providerID == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleUpdateEmbeddingProvider",
			"user_id", userID,
		).Error("Provider ID is required")
		apiErr := errors.NewProviderValidationError(
			"Provider ID is required in the URL path",
			[]errors.ValidationError{errors.NewRequiredFieldError("id")},
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	req, ok := s.decodeAndValidateUpdateEmbeddingProviderRequest(w, r, userID, providerID)
	if !ok {
		return
	}

	// Capture the provider before the update so we can tell whether its embedding
	// identity (model / provider type / endpoint) changed and a re-embed is needed.
	// Best-effort: if we can't read it, skip the re-embed check rather than fail.
	oldProvider, oldErr := s.container.EmbeddingProviderService().GetEmbeddingProvider(r.Context(), teamID, providerID)
	if oldErr != nil {
		oldProvider = nil
	}

	provider, err := s.container.EmbeddingProviderService().UpdateEmbeddingProvider(r.Context(), teamID, providerID, req)
	if err != nil {
		s.handleUpdateEmbeddingProviderError(w, r, userID, providerID, err)
		return
	}

	s.reembedTeamIfProviderIdentityChanged(teamID, oldProvider, provider)

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Embedding provider updated successfully")

	s.writeEmbeddingProviderResponse(w, provider)
}

// decodeAndValidateUpdateEmbeddingProviderRequest decodes the update body and
// enforces the instruction-prefix length cap, writing the appropriate error and
// returning ok=false on failure. Extracted from handleUpdateEmbeddingProvider to
// keep it within the function-length limit.
func (s *Server) decodeAndValidateUpdateEmbeddingProviderRequest(
	w http.ResponseWriter, r *http.Request, userID, providerID string,
) (models.UpdateEmbeddingProviderRequest, bool) {
	var req models.UpdateEmbeddingProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logEmbeddingProviderError(
			"handleUpdateEmbeddingProvider", userID, providerID, err,
			"Failed to decode request body",
		)
		errors.WriteJSONError(w, r, errors.NewBadRequestError(
			"Invalid request body. Please ensure the JSON is well-formed.",
		))
		return req, false
	}

	if ve := appendPrefixLengthErrors(nil, req.QueryPrefix, req.DocumentPrefix); len(ve) > 0 {
		errors.WriteJSONError(w, r, errors.NewProviderValidationError(
			"Embedding provider validation failed. Please check the prefix fields.", ve,
		))
		return req, false
	}

	return req, true
}

func (s *Server) handleUpdateEmbeddingProviderError(
	w http.ResponseWriter,
	r *http.Request,
	userID, providerID string,
	err error,
) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to update embedding provider")

	if stderrors.Is(err, services.ErrProviderNotFound) {
		errors.WriteJSONError(w, r, errors.NewProviderNotFoundError(providerID))
		return
	}
	errors.WriteJSONError(w, r, errors.NewProviderUpdateFailedError(
		"Unable to update embedding provider. Please check your configuration and try again.",
	))
}

func (s *Server) handleDeleteEmbeddingProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	providerID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleDeleteEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Embedding provider deletion request received")

	if providerID == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleDeleteEmbeddingProvider",
			"user_id", userID,
		).Error("Provider ID is required")
		apiErr := errors.NewProviderValidationError(
			"Provider ID is required in the URL path",
			[]errors.ValidationError{errors.NewRequiredFieldError("id")},
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	err := s.container.EmbeddingProviderService().DeleteEmbeddingProvider(r.Context(), teamID, providerID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleDeleteEmbeddingProvider",
			"user_id", userID,
			"provider_id", providerID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to delete embedding provider")
		if stderrors.Is(err, services.ErrProviderNotFound) {
			errors.WriteJSONError(w, r, errors.NewProviderNotFoundError(providerID))
			return
		}
		if stderrors.Is(err, services.ErrLastProviderDelete) {
			errors.WriteJSONError(w, r, errors.NewProviderLastDeleteBlockedError())
			return
		}
		errors.WriteJSONError(w, r, errors.NewProviderDeleteFailedError(
			"Unable to delete embedding provider. Please try again later.",
		))
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleDeleteEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Embedding provider deleted successfully")

	writeNoContent(w)
}

// handleReprocessEmbeddingProvider handles POST .../embedding-providers/{id}/reprocess.
// It re-drives embedding generation for the team's entities that are still missing
// an embedding, through the concurrency-bounded path (#142), and returns 202
// immediately — generation runs in the background and is idempotent
// (delete-then-insert per entity). The provider {id} is validated and authorized
// (team middleware), but the work is team-scoped: a provider is per-team, so
// reprocess enqueues the team's entity set, generated via the team's active
// provider. A per-team in-flight guard makes repeat calls safe (no double
// fan-out).
func (s *Server) handleReprocessEmbeddingProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	providerID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleReprocessEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Embedding provider reprocess request received")

	if providerID == "" {
		apiErr := errors.NewProviderValidationError(
			"Provider ID is required in the URL path",
			[]errors.ValidationError{errors.NewRequiredFieldError("id")},
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	// Confirm the provider exists and belongs to the team before enqueuing, so a
	// bad id returns 404 rather than silently starting a team-wide re-embed.
	if _, err := s.container.EmbeddingProviderService().GetEmbeddingProvider(r.Context(), teamID, providerID); err != nil {
		s.logEmbeddingProviderError(
			"handleReprocessEmbeddingProvider", userID, providerID, err,
			"Failed to load provider for reprocess",
		)
		if stderrors.Is(err, services.ErrProviderNotFound) {
			errors.WriteJSONError(w, r, errors.NewProviderNotFoundError(providerID))
			return
		}
		errors.WriteJSONError(w, r, errors.NewDatabaseError(
			fmt.Sprintf("Failed to retrieve embedding provider '%s'. Please try again later.", providerID),
		))
		return
	}

	s.enqueueTeamReembed(teamID, false)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":  "accepted",
		"message": "Reprocessing missing embeddings for this team in the background.",
	}, s.logger)
}

func (s *Server) handleValidateEmbeddingProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleValidateEmbeddingProvider",
		"user_id", userID,
	).Info("Embedding provider validation request received")

	var req models.ValidateEmbeddingProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logEmbeddingProviderError("handleValidateEmbeddingProvider", userID, "", err, "Failed to decode request body")
		apiErr := errors.NewBadRequestError(
			"Invalid request body. Please ensure the JSON is well-formed.",
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	if !s.validateEmbeddingProviderRequest(w, r, userID, &req) {
		return
	}

	response, err := s.container.EmbeddingProviderService().ValidateEmbeddingProvider(r.Context(), req)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleValidateEmbeddingProvider",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to validate embedding provider")
		// Service errors (network issues, etc.) are internal errors - don't expose raw error
		errors.WriteJSONError(w, r, errors.NewInternalError(
			"Provider validation failed due to a service error. Please try again later.",
		))
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleValidateEmbeddingProvider",
		"user_id", userID,
		"is_valid", response.IsValid,
		"message", response.Message,
	).Info("Embedding provider validation completed")

	writeOK(w, response, s.logger)
}

func (s *Server) validateEmbeddingProviderRequest(
	w http.ResponseWriter,
	r *http.Request,
	userID string,
	req *models.ValidateEmbeddingProviderRequest,
) bool {
	var validationErrors []errors.ValidationError

	if req.ProviderType == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleValidateEmbeddingProvider",
			"user_id", userID,
		).Error("Provider type is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("provider_type"))
	}

	if req.BaseURL == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleValidateEmbeddingProvider",
			"user_id", userID,
		).Error("Base URL is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("base_url"))
	}

	if req.Model == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleValidateEmbeddingProvider",
			"user_id", userID,
		).Error("Model is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("model"))
	}

	if len(validationErrors) > 0 {
		apiErr := errors.NewProviderValidationError(
			"Provider validation request is missing required fields",
			validationErrors,
		)
		errors.WriteJSONError(w, r, apiErr)
		return false
	}

	return true
}
