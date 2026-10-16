package server

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/audit"
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
	// lastWrite points at the written model's LastWrite, so recording the
	// write can stamp it on the response body (see recordResourceWrite).
	lastWrite *models.LastWrite
}

func writtenPrompt(teamID string, p *models.Prompt) writtenResource {
	return writtenResource{
		resourceType: resourceTypePrompt, teamID: teamID, projectID: p.ProjectID, id: p.ID, lastWrite: &p.LastWrite,
	}
}

func writtenArtifact(teamID string, a *models.Artifact) writtenResource {
	return writtenResource{
		resourceType: resourceTypeArtifact, teamID: teamID, projectID: a.ProjectID, id: a.ID, lastWrite: &a.LastWrite,
	}
}

func writtenBlueprint(teamID string, b *models.Blueprint) writtenResource {
	return writtenResource{
		resourceType: resourceTypeBlueprint, teamID: teamID, projectID: b.ProjectID, id: b.ID, lastWrite: &b.LastWrite,
	}
}

func writtenMemory(teamID string, m *models.Memory) writtenResource {
	return writtenResource{
		resourceType: resourceTypeMemory, teamID: teamID, projectID: m.ProjectID, id: m.ID, lastWrite: &m.LastWrite,
	}
}

// recordResourceWrite records a write event for a create or update that has
// already succeeded. Callers invoke it only on the success path, so a failed or
// unauthorized write records nothing.
//
// It records nothing either when the context carries no write origin (an
// internal caller) or when the write service is not wired (unit tests).
// Recording is fire-and-forget and can never fail the write it describes.
//
// It also stamps the origin on the written model as its last write (#1418).
// The event is persisted off the request path, so a model the service read
// back after the write still reports the PREVIOUS write's channel, or none for
// a create. This write is by definition the latest, and its channel is known
// here, so the response says so without waiting for the event to land.
func (s *Server) recordResourceWrite(ctx context.Context, userID, action string, res writtenResource) {
	origin := resourceaccess.WriteOriginFromContext(ctx)
	if origin == nil {
		return
	}
	if res.lastWrite != nil {
		*res.lastWrite = models.LastWriteFromOrigin(origin)
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

// recordImportedBlueprintWrites records one write event per blueprint a GitHub
// import created or refreshed (#1418). The import writes through the blueprint
// repository rather than the blueprint service, so nothing else records it, and
// before this every imported blueprint counted as an unknown creation for good.
//
// The channel is the import request's own: the import is a write the caller
// asked for through the web app, the CLI or the API, like any other. A
// blueprint the import left alone (up to date, conflicting, skipped or failed)
// was not written and records nothing.
func (s *Server) recordImportedBlueprintWrites(
	ctx context.Context, userID, teamID string, report *models.BlueprintImportReport,
) {
	if report == nil {
		return
	}
	written := func(blueprintID string) writtenResource {
		return writtenResource{
			resourceType: resourceTypeBlueprint, teamID: teamID, projectID: report.ProjectID, id: blueprintID,
		}
	}
	audited := func(verb, blueprintID, title string) auditedWrite {
		w := auditRecord(teamID, resourceTypeBlueprint, verb, blueprintID,
			&auditState{projectID: report.ProjectID, name: title})
		w.metadata[auditMetaVia] = auditViaGitHubImport
		return w
	}
	for i := range report.SuccessfulItems {
		item := &report.SuccessfulItems[i]
		s.recordResourceWrite(ctx, userID, models.WriteActionCreate, written(item.BlueprintID))
		s.recordAuditWrite(ctx, audited(auditVerbCreate, item.BlueprintID, item.Title))
	}
	for i := range report.UpdatedItems {
		item := &report.UpdatedItems[i]
		s.recordResourceWrite(ctx, userID, models.WriteActionUpdate, written(item.BlueprintID))
		s.recordAuditWrite(ctx, audited(auditVerbUpdate, item.BlueprintID, item.Title))
	}
}

// auditedWrite is one successful write as the audit log records it (#1513).
// The names are snapshots: pass the resource's name as it is after a create
// or update, and as it was before a delete, which cannot be looked up later.
type auditedWrite struct {
	op           models.AuditOperation
	teamID       string
	projectID    string
	projectName  string
	resourceType string
	resourceID   string
	resourceName string
	// metadata holds changed-field names and old/new values of small scalar
	// fields only. Resource content never goes here (epic #1510, decision 4).
	metadata map[string]any
}

// auditRecorder returns the audit recorder, or nil when none is wired (unit
// tests on a mock container). Callers that must do extra work for the audit
// log, such as reading a resource before updating it, check it first so that
// work is skipped when nothing would record it.
func (s *Server) auditRecorder() audit.Recorder {
	if s.container == nil {
		return nil
	}
	return s.container.AuditRecorder()
}

// recordAuditWrite records a resource write in the audit log. Like
// recordResourceWrite it is called only on the success path, so a failed or
// denied write records nothing here (denials are recorded by the
// authorization service). The channel, MCP client name, API key, IP and user
// agent come from ctx, which is why the routes and tools that call it carry a
// write origin. Recording is asynchronous and can never fail the write.
func (s *Server) recordAuditWrite(ctx context.Context, w auditedWrite) {
	s.recordAudit(ctx, audit.Event{
		Operation:    w.op,
		TeamID:       w.teamID,
		ProjectID:    w.projectID,
		ProjectName:  w.projectName,
		ResourceType: w.resourceType,
		ResourceID:   w.resourceID,
		ResourceName: w.resourceName,
		Metadata:     w.metadata,
	})
}

// annotateWriteSourceOnWrites is annotateWriteSource for a route group that
// also serves reads: it labels only the requests that can write (anything but
// GET, HEAD and OPTIONS). It is mounted on the strict-server prompt, artifact
// and blueprint groups, whose only writes are lock and unlock, so those are
// attributed to their channel in the audit log while the reads stay as they
// were.
func (s *Server) annotateWriteSourceOnWrites(next http.Handler) http.Handler {
	annotated := s.annotateWriteSource(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
		default:
			annotated.ServeHTTP(w, r)
		}
	})
}

// toGenWriteSource converts a resource's stored last-write channel to a
// generated strict-server enum, keeping nil (no recorded write) as nil so the
// field serializes as null. T is a domain package's ResourceWriteSource; the
// four domains each generate their own copy of the one spec enum.
func toGenWriteSource[T ~string](source *string) *T {
	if source == nil {
		return nil
	}
	out := T(*source)
	return &out
}
