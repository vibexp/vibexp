package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	siteinfogen "github.com/vibexp/vibexp/internal/server/gen/siteinfo"
)

// Public instance site info (#1355, epic #1352): the company branding an
// instance admin configured, readable without authentication because the
// sign-in page shows it. The same values are inlined into /config.js (see
// handleConfigJS) so the first paint is already branded.
//
// Both surfaces are world-readable. toPublicSiteInfo is the one place that
// decides what they carry: the eight non-secret values and nothing else.

// siteInfoStrictServer implements siteinfogen.StrictServerInterface.
type siteInfoStrictServer struct {
	s *Server
}

var _ siteinfogen.StrictServerInterface = (*siteInfoStrictServer)(nil)

// setupSiteInfoRoutes mounts the generated SiteInfo strict-server handler. The
// operation is unauthenticated, and sits behind the general per-IP API limit:
// the web app fetches it on load, so the strict auth limit would be wrong.
func (s *Server) setupSiteInfoRoutes() {
	strict := siteinfogen.NewStrictHandlerWithOptions(
		&siteInfoStrictServer{s: s},
		nil,
		siteinfogen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  s.siteInfoBindErrorHandler,
			ResponseErrorHandlerFunc: s.siteInfoResponseErrorHandler,
		},
	)
	s.router.Group(func(r chi.Router) {
		rateLimitByIP(r, s.config.RateLimit.APIPerMinute, s.config.IsLocalDevelopment())
		siteinfogen.HandlerWithOptions(strict, siteinfogen.ChiServerOptions{
			BaseRouter:       r,
			ErrorHandlerFunc: s.siteInfoBindErrorHandler,
		})
	})
}

// GetSiteInfo handles GET /api/v1/site-info. It never fails: publicSiteInfo
// answers "nothing configured" when the values cannot be read.
func (h *siteInfoStrictServer) GetSiteInfo(
	ctx context.Context, _ siteinfogen.GetSiteInfoRequestObject,
) (siteinfogen.GetSiteInfoResponseObject, error) {
	return siteinfogen.GetSiteInfo200JSONResponse(h.s.publicSiteInfo(ctx)), nil
}

// publicSiteInfo resolves the instance's company branding in its public shape.
//
// It FAILS OPEN at every step, because both callers sit in front of sign-in
// and /config.js must render for the app to load at all:
//
//   - the service's Resolve already answers the empty value set on a read
//     error;
//   - a server with no container or no site info service answers the same;
//   - so does a read that panics, which is what a server built without a
//     database does (the SPA and /config.js are served with no database in
//     tests and tooling). The panic is logged, never swallowed silently.
func (s *Server) publicSiteInfo(ctx context.Context) (out siteinfogen.SiteInfoResponse) {
	defer func() {
		if recovered := recover(); recovered != nil {
			s.logger.With("panic", recovered).
				Error("site info read panicked; treating it as not configured")
			out = toPublicSiteInfo(models.InstanceSiteInfoValues{})
		}
	}()
	if s.container == nil {
		return toPublicSiteInfo(models.InstanceSiteInfoValues{})
	}
	svc := s.container.InstanceSiteInfoService()
	if svc == nil {
		return toPublicSiteInfo(models.InstanceSiteInfoValues{})
	}
	return toPublicSiteInfo(svc.Resolve(ctx))
}

// toPublicSiteInfo renders the values for the public surfaces: an unset value
// (the empty string) is null.
func toPublicSiteInfo(v models.InstanceSiteInfoValues) siteinfogen.SiteInfoResponse {
	return siteinfogen.SiteInfoResponse{
		CompanyName:  nilIfEmpty(v.CompanyName),
		CompanyEmail: nilIfEmpty(v.CompanyEmail),
		Website:      nilIfEmpty(v.Website),
		SupportUrl:   nilIfEmpty(v.SupportURL),
		TermsUrl:     nilIfEmpty(v.TermsURL),
		PrivacyUrl:   nilIfEmpty(v.PrivacyURL),
		LogoUrl:      nilIfEmpty(v.LogoURL),
		LogoDarkUrl:  nilIfEmpty(v.LogoDarkURL),
	}
}

func nilIfEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// siteInfoBindErrorHandler writes request-binding errors. The one operation
// takes no parameters or body, so this is defensive.
func (s *Server) siteInfoBindErrorHandler(w http.ResponseWriter, r *http.Request, _ error) {
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("Invalid request"))
}

// siteInfoResponseErrorHandler writes errors returned by the strict handler.
// GetSiteInfo returns none, so this covers only a failure to encode the
// response.
func (s *Server) siteInfoResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}
	s.logger.With("error", err).Error("Site info strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Internal server error"))
}
