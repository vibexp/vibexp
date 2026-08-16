package server

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"unicode/utf8"

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

// Response and log messages shared across the embedding provider handlers.
const ()

// appendPrefixLengthErrors appends a max-length validation error for each
// instruction prefix that exceeds maxEmbeddingPrefixLen. A nil pointer (field
// omitted) is skipped. Shared by the create and update handlers so both enforce
// the same cap. Length is counted in characters (runes), matching the OpenAPI
// maxLength and the frontend validation — not bytes — so a multi-byte prefix is
// accepted up to the same documented limit.
func appendPrefixLengthErrors(
	ve []errors.ValidationError, queryPrefix, documentPrefix *string,
) []errors.ValidationError {
	if queryPrefix != nil && utf8.RuneCountInString(*queryPrefix) > maxEmbeddingPrefixLen {
		ve = append(ve, errors.NewMaxLengthError("query_prefix", maxEmbeddingPrefixLen))
	}
	if documentPrefix != nil && utf8.RuneCountInString(*documentPrefix) > maxEmbeddingPrefixLen {
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
		"service", serverLogServiceName,
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
		result, err := s.container.EmbeddingBackfillService().Backfill(
			context.Background(),
			services.EmbeddingBackfillRequest{All: true, TeamID: teamID, MissingOnly: !wipe},
		)
		if err != nil {
			logger.With("error", fmt.Sprintf("%+v", err)).
				Error("Background team re-embed failed")
			return
		}
		// The 202 response carries no counters, so this log line is the only place
		// the reprocess/create path's outcome is attributable to the team that
		// triggered it. The counts are PUBLISHES, not embeddings written — the
		// service logs an embedding-coverage snapshot separately (#755).
		logger.With(
			"total_seen", result.TotalSeen,
			"total_published", result.TotalPublished,
			"total_failed", result.TotalFailed,
		).Info("Background team re-embed published its events (generation is async)")
	}()
}

func (s *Server) logEmbeddingProviderError(
	handler, userID, providerID string,
	err error,
	msg string,
) {
	fields := []any{
		"service", serverLogServiceName,
		"handler", handler,
		"user_id", userID,
		"error", fmt.Sprintf("%+v", err),
	}
	if providerID != "" {
		fields = append(fields, "provider_id", providerID)
	}
	s.logger.With(fields...).Error(msg)
}

func (s *Server) handleGetEmbeddingCoverage(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleGetEmbeddingCoverage",
		"user_id", userID,
	).Info("Embedding coverage request received")

	coverage, err := s.container.EmbeddingStatusService().GetCoverage(r.Context(), teamID)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
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
		"service", serverLogServiceName,
		"handler", "handleReprocessEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Embedding provider reprocess request received")

	// A team-wide re-embed spends the team's provider budget, so it is gated
	// like the settings that cause it (#464).
	if !s.requireProviderManagePermission(w, r, userID, teamID) {
		return
	}

	if providerID == "" {
		apiErr := errors.NewProviderValidationError(
			msgProviderIDRequiredInPath,
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

// handleClearEmbeddings handles DELETE .../settings/embedding-providers/embeddings.
// It permanently deletes every stored embedding for the team (a destructive
// truncate) and returns 200 with the number of rows removed. Unlike reprocess it
// deletes directly via the repository and deliberately does NOT re-enqueue
// generation — the team's content stays unembedded until a later reprocess or an
// identity-changing provider update re-embeds it. The work is team-scoped
// (team middleware validates {team_id}); only the authenticated team's rows are
// touched.
func (s *Server) handleClearEmbeddings(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleClearEmbeddings",
		"user_id", userID,
		"team_id", teamID,
	).Info("Clear embeddings request received")

	// Destructive: this deletes every embedding the team has. Owner/admin only (#464).
	if !s.requireProviderManagePermission(w, r, userID, teamID) {
		return
	}

	deleted, err := s.container.EmbeddingRepository().DeleteByTeam(r.Context(), teamID)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleClearEmbeddings",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to clear team embeddings")
		errors.WriteJSONError(w, r, errors.NewDatabaseError(
			"Failed to clear embeddings. Please try again later.",
		))
		return
	}

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleClearEmbeddings",
		"user_id", userID,
		"team_id", teamID,
		"deleted", deleted,
	).Info("Cleared team embeddings")

	writeOK(w, models.ClearEmbeddingsResponse{DeletedCount: deleted}, s.logger)
}
