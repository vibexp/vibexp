package server

import (
	"fmt"
	"net/http"
	"time"

	apierrors "github.com/vibexp/vibexp/internal/errors"
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
			s.logger.With(
				"error", fmt.Sprintf("%+v", err),
				"from", fromStr,
			).Error("Invalid 'from' date format")
			apierrors.WriteJSONError(w, r, apierrors.NewDateValidationError("from", fromStr))
			return
		}

		parsedTo, err := time.Parse("2006-01-02", toStr)
		if err != nil {
			s.logger.With(
				"error", fmt.Sprintf("%+v", err),
				"to", toStr,
			).Error("Invalid 'to' date format")
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
		s.logger.With("error", fmt.Sprintf("%+v", err)).Error("Failed to get usage and growth data")
		apierrors.WriteJSONError(w, r, apierrors.NewDatabaseError(
			"Failed to retrieve usage and growth data. Please try again later.",
		))
		return
	}

	writeOK(w, response, s.logger)
}
