package server

import (
	"context"
	"errors"
	"net/http"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
)

// adminMsgInternalError is the generic problem detail for unexpected failures
// in the admin strict handlers.
const adminMsgInternalError = "Internal server error"

// adminStrictServer implements the generated Admin StrictServerInterface. The
// /api/v1/admin surface is guarded by instanceAdminMiddleware, so every request
// reaching these methods is already an authenticated instance admin.
type adminStrictServer struct {
	s *Server
}

var _ admingen.StrictServerInterface = (*adminStrictServer)(nil)

// GetAdminStats returns instance-wide entity counts plus the running app version.
func (a *adminStrictServer) GetAdminStats(
	ctx context.Context, _ admingen.GetAdminStatsRequestObject,
) (admingen.GetAdminStatsResponseObject, error) {
	counts, err := a.s.container.AdminService().GetInstanceCounts(ctx)
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName,
			"handler", "GetAdminStats",
			"error", err,
		).Error("Failed to get instance counts")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}

	version := a.s.config.Server.ServiceVersion
	if version == "" {
		version = "dev"
	}

	return admingen.GetAdminStats200JSONResponse(admingen.AdminStatsResponse{
		Counts: admingen.AdminInstanceCounts{
			Users:     counts.Users,
			Teams:     counts.Teams,
			Prompts:   counts.Prompts,
			Artifacts: counts.Artifacts,
			Memories:  counts.Memories,
		},
		Version: version,
	}), nil
}

// adminBindErrorHandler translates parameter-binding failures from the generated
// layer into this domain's RFC 9457 400 responses.
func (s *Server) adminBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(err.Error()))
}

// adminResponseErrorHandler writes errors returned by the strict handler
// implementations. *apierrors.APIError carries the intended RFC 9457 error;
// anything else is defensive and maps to a generic 500.
func (s *Server) adminResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With("error", err).Error("Admin strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(adminMsgInternalError))
}
