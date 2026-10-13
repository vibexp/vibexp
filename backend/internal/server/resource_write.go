package server

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
)

// annotateWriteSource labels a REST write with the channel it arrived through
// (#1392), for the handler to record once the write has succeeded.
//
// The channel is derived exactly as the read side derives it, from how the
// request was authenticated and routed: a session cookie is the web app, an API
// key with the CLI's user agent is the CLI, anything else is the API. Nothing
// the client puts in the body can change it. A REST write never carries an
// assistant name; only an MCP client reports one.
//
// The middleware only annotates the context. It records nothing, because it
// cannot know whether the write succeeded or which resource it produced.
func (s *Server) annotateWriteSource(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authType, _ := r.Context().Value(contextkeys.AuthType).(string)
		source := resourceaccess.DeriveSource(authType, r.URL.Path, r.UserAgent())
		next.ServeHTTP(w, r.WithContext(resourceaccess.WithWriteOrigin(r.Context(), source, "")))
	})
}

// withMCPWriteOrigin labels an MCP tool call's writes as arriving over MCP,
// under the name the client reported for itself when it initialized the
// session.
//
// The source is fixed here rather than derived: the MCP mount does not run the
// REST write-source middleware, and DeriveSource classifies an OAuth bearer
// token as "api", so deriving it would count every OAuth MCP client as an API
// consumer. The read side hardcodes SourceMCP for the same reason (see
// recordMCPResourceAccess).
func withMCPWriteOrigin(ctx context.Context, req *mcp.CallToolRequest) context.Context {
	var session *mcp.ServerSession
	if req != nil {
		session = req.Session
	}
	return resourceaccess.WithWriteOrigin(ctx, resourceaccess.SourceMCP, mcpClientName(session))
}

// mcpClientName returns the clientInfo.name the MCP client sent in its
// initialize request, or "" when there is none: no session, a session that has
// not initialized, or a client that sent no clientInfo. The name is whatever the
// client chose to report, so it is a label and never an identity.
func mcpClientName(session *mcp.ServerSession) string {
	if session == nil {
		return ""
	}
	return clientNameFromInitialize(session.InitializeParams())
}

func clientNameFromInitialize(params *mcp.InitializeParams) string {
	if params == nil || params.ClientInfo == nil {
		return ""
	}
	return params.ClientInfo.Name
}

// writtenResource identifies the resource a successful write produced. The
// team is the one the request was authorized against, not a value read back off
// the stored row.
type writtenResource struct {
	resourceType string
	teamID       string
	projectID    string
	id           string
}

func writtenPrompt(teamID string, p *models.Prompt) writtenResource {
	return writtenResource{resourceType: resourceTypePrompt, teamID: teamID, projectID: p.ProjectID, id: p.ID}
}

func writtenArtifact(teamID string, a *models.Artifact) writtenResource {
	return writtenResource{resourceType: resourceTypeArtifact, teamID: teamID, projectID: a.ProjectID, id: a.ID}
}

func writtenBlueprint(teamID string, b *models.Blueprint) writtenResource {
	return writtenResource{resourceType: resourceTypeBlueprint, teamID: teamID, projectID: b.ProjectID, id: b.ID}
}

func writtenMemory(teamID string, m *models.Memory) writtenResource {
	return writtenResource{resourceType: resourceTypeMemory, teamID: teamID, projectID: m.ProjectID, id: m.ID}
}

// recordResourceWrite records a write event for a create or update that has
// already succeeded. Callers invoke it only on the success path, so a failed or
// unauthorized write records nothing.
//
// It records nothing either when the context carries no write origin (an
// internal caller) or when the write service is not wired (unit tests).
// Recording is fire-and-forget and can never fail the write it describes.
func (s *Server) recordResourceWrite(ctx context.Context, userID, action string, res writtenResource) {
	origin := resourceaccess.WriteOriginFromContext(ctx)
	if origin == nil {
		return
	}

	svc := s.container.ResourceWriteService()
	if svc == nil {
		return
	}

	event := &models.ResourceWriteEvent{
		TeamID:        res.teamID,
		ResourceType:  res.resourceType,
		ResourceID:    res.id,
		Action:        action,
		Source:        origin.Source,
		AssistantName: origin.AssistantName,
	}
	if res.projectID != "" {
		event.ProjectID = &res.projectID
	}
	if userID != "" {
		event.UserID = &userID
	}
	if apiKeyID, ok := ctx.Value(contextkeys.APIKeyID).(string); ok && apiKeyID != "" {
		event.APIKeyID = &apiKeyID
	}

	svc.RecordWrite(event)
}
