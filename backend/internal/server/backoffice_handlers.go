package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/services"
)

// handleBackofficeUsageAndGrowth handles GET /bo/v1/reports/usage-and-growth
func (s *Server) handleBackofficeUsageAndGrowth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	var fromDate, toDate *time.Time

	if fromStr != "" && toStr != "" {
		parsedFrom, err := time.Parse("2006-01-02", fromStr)
		if err != nil {
			s.logger.WithFields(logrus.Fields{
				"error": fmt.Sprintf("%+v", err),
				"from":  fromStr,
			}).Error("Invalid 'from' date format")
			apierrors.WriteJSONError(w, r, apierrors.NewDateValidationError("from", fromStr))
			return
		}

		parsedTo, err := time.Parse("2006-01-02", toStr)
		if err != nil {
			s.logger.WithFields(logrus.Fields{
				"error": fmt.Sprintf("%+v", err),
				"to":    toStr,
			}).Error("Invalid 'to' date format")
			apierrors.WriteJSONError(w, r, apierrors.NewDateValidationError("to", toStr))
			return
		}

		if parsedTo.Before(parsedFrom) {
			apierrors.WriteJSONError(w, r, apierrors.NewDateRangeError())
			return
		}

		fromDate = &parsedFrom
		toDate = &parsedTo
	}

	// Call service layer
	response, err := s.container.BackofficeService().GetUsageAndGrowth(ctx, fromDate, toDate)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"error": fmt.Sprintf("%+v", err),
		}).Error("Failed to get usage and growth data")
		apierrors.WriteJSONError(w, r, apierrors.NewDatabaseError(
			"Failed to retrieve usage and growth data. Please try again later.",
		))
		return
	}

	writeOK(w, response, s.logger)
}

// embeddingsBackfillRequest is the request body for the backfill endpoint. The
// scope is explicit: exactly one of all or a non-empty entity_types must be set.
type embeddingsBackfillRequest struct {
	// All backfills every supported entity type. Mutually exclusive with EntityTypes.
	All bool `json:"all"`
	// EntityTypes restricts the run to specific types. Mutually exclusive with All.
	EntityTypes []string `json:"entity_types"`
	// MissingOnly restricts the run to entities lacking an embedding for the
	// currently configured model.
	MissingOnly bool `json:"missing_only"`
	// DryRun previews the counts without publishing any event.
	DryRun bool `json:"dry_run"`
}

// handleEmbeddingsBackfill handles POST /bo/v1/embeddings/backfill. It republishes
// the `.created` event for every embeddable entity so the embedding pipeline
// regenerates all vectors under the currently configured model — the operational
// step after a model/dimension swap. Guarded by backofficeAuthMiddleware.
func (s *Server) handleEmbeddingsBackfill(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	req, err := decodeEmbeddingsBackfillRequest(r)
	if err != nil {
		apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("Invalid request body"))
		return
	}

	result, err := s.container.EmbeddingBackfillService().Backfill(ctx, services.EmbeddingBackfillRequest{
		All:         req.All,
		EntityTypes: req.EntityTypes,
		MissingOnly: req.MissingOnly,
		DryRun:      req.DryRun,
	})
	if err != nil {
		if isBackfillBadRequest(err) {
			apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(err.Error()))
			return
		}
		s.logger.WithFields(logrus.Fields{
			"error": fmt.Sprintf("%+v", err),
		}).Error("Embedding backfill failed")
		apierrors.WriteJSONError(w, r, apierrors.NewInternalError(
			"Failed to run embedding backfill. Please try again later.",
		))
		return
	}

	writeOK(w, result, s.logger)
}

// isBackfillBadRequest reports whether a backfill error is a client-side scope or
// entity-type validation failure that maps to a 400 rather than a 500.
func isBackfillBadRequest(err error) bool {
	return errors.Is(err, services.ErrUnsupportedBackfillEntityType) ||
		errors.Is(err, services.ErrBackfillScopeRequired) ||
		errors.Is(err, services.ErrBackfillScopeAmbiguous)
}

// decodeEmbeddingsBackfillRequest decodes the request body. An empty body yields
// the zero-value request, which the service rejects as a missing scope.
func decodeEmbeddingsBackfillRequest(r *http.Request) (embeddingsBackfillRequest, error) {
	var req embeddingsBackfillRequest
	dec := json.NewDecoder(r.Body)
	// Reject unknown/typo'd fields so a misspelled "dry_run" can't silently fall
	// through to a full live backfill on this destructive operational endpoint.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			return req, nil
		}
		return req, err
	}
	return req, nil
}
