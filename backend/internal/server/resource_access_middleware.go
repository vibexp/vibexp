package server

import (
	"context"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
)

// Resource type values recorded on access events. These must match the values
// stored by the resource-access epic exactly.
const (
	resourceTypePrompt    = "prompt"
	resourceTypeArtifact  = "artifact"
	resourceTypeBlueprint = "blueprint"
	resourceTypeMemory    = "memory"
	resourceTypeProject   = "project"
	resourceTypeAgent     = "agent"
)

// recordResourceAccess returns middleware that records a resource detail-access
// event for a successful read.
//
// Contract: the middleware injects a mutable resource-id holder before calling
// the handler; the handler fills it in via contextkeys.SetAccessedResourceID
// once it has resolved the entity's UUID. After the handler returns, an event is
// recorded only when ALL of these hold:
//   - the response status is 2xx,
//   - a resolved resource UUID is present (absent ⇒ skip, which avoids a
//     slug→UUID double lookup on slug-keyed routes that errored), and
//   - the resource-access service is wired (it may be nil in unit tests).
//
// Recording is fire-and-forget and never blocks or fails the read path.
func (s *Server) recordResourceAccess(resourceType string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := contextkeys.ContextWithAccessedResourceID(r.Context())
			r = r.WithContext(ctx)

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				// WriteHeader was never called explicitly.
				status = http.StatusOK
			}
			if status < 200 || status >= 300 {
				return
			}

			resourceID, ok := contextkeys.GetAccessedResourceID(ctx)
			if !ok {
				return
			}

			svc := s.container.ResourceAccessService()
			if svc == nil {
				return
			}

			svc.RecordAccess(s.buildResourceAccessEvent(r, resourceType, resourceID))
		})
	}
}

// buildResourceAccessEvent assembles a freshly-allocated access event from the
// request's authenticated context. Identity fields come from middleware-set
// context values, never from the request body.
func (s *Server) buildResourceAccessEvent(
	r *http.Request,
	resourceType, resourceID string,
) *models.ResourceAccessEvent {
	event := &models.ResourceAccessEvent{
		TeamID:       chi.URLParam(r, "team_id"),
		ResourceType: resourceType,
		ResourceID:   resourceID,
	}

	authType, _ := r.Context().Value(contextkeys.AuthType).(string)
	event.Source = resourceaccess.DeriveSource(authType, r.URL.Path, r.UserAgent())

	if userID, ok := r.Context().Value(contextkeys.UserID).(string); ok && userID != "" {
		event.UserID = &userID
	}
	if apiKeyID, ok := r.Context().Value(contextkeys.APIKeyID).(string); ok && apiKeyID != "" {
		event.APIKeyID = &apiKeyID
	}
	if ua := r.UserAgent(); ua != "" {
		event.UserAgent = &ua
	}
	// source_ip is an INET column. clientIP resolves under the trusted-proxy
	// rule (#465), so a forged X-Forwarded-For no longer lands here on a
	// directly-exposed instance. The parse check stays as a belt-and-braces
	// guard: a non-IP value would fail the INSERT and silently drop the event
	// (the column is nullable, so leaving it nil is safe).
	if ip := clientIP(r); net.ParseIP(ip) != nil {
		event.SourceIP = &ip
	}

	return event
}

// recordMCPResourceAccess records a detail-access event for an MCP get-tool.
// MCP tools bypass HTTP, so source is always SourceMCP and UserAgent/SourceIP are
// unavailable. Recording is fire-and-forget and skipped when the service is nil.
func (s *Server) recordMCPResourceAccess(
	ctx context.Context,
	teamID, userID, resourceType, resourceID string,
) {
	svc := s.container.ResourceAccessService()
	if svc == nil {
		return
	}

	event := &models.ResourceAccessEvent{
		TeamID:       teamID,
		UserID:       &userID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Source:       resourceaccess.SourceMCP,
	}
	if apiKeyID, ok := ctx.Value(contextkeys.APIKeyID).(string); ok && apiKeyID != "" {
		event.APIKeyID = &apiKeyID
	}

	svc.RecordAccess(event)
}
