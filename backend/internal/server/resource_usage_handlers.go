package server

import (
	"net/http"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/services"
)

// ResourceUsageHandler handles resource usage endpoints
type ResourceUsageHandler struct {
	resourceUsageService services.ResourceUsageServiceInterface
	logger               *logrus.Logger
}

// NewResourceUsageHandler creates a new resource usage handler
func NewResourceUsageHandler(
	resourceUsageService services.ResourceUsageServiceInterface,
	logger *logrus.Logger,
) *ResourceUsageHandler {
	return &ResourceUsageHandler{
		resourceUsageService: resourceUsageService,
		logger:               logger,
	}
}

// GetResourceUsage handles GET /api/v1/resource-usage
func (h *ResourceUsageHandler) GetResourceUsage(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context
	userID, ok := r.Context().Value(contextKeyUserID).(string)
	if !ok || userID == "" {
		h.logger.Error("User ID missing from context")
		apiErr := errors.NewAuthRequiredError("Valid authentication token required")
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	// Get resource usage
	usage, err := h.resourceUsageService.GetResourceUsage(r.Context(), userID)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get resource usage")
		apiErr := errors.NewInternalError("Failed to get resource usage")
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	// Return response
	writeOK(w, usage, h.logger)
}
