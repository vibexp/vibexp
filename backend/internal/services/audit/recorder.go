// Package audit records the instance audit log (#1511, epic #1510): who did
// what, when, from where and through which channel.
//
// Features call Recorder.Record from the service layer. Recording is
// best-effort and asynchronous: it reads the request-scoped facts from the
// context on the caller's goroutine, then looks up display names and writes
// the row on the shared events worker pool. An audit failure is logged and
// counted (vx_audit_events_dropped_total) and never reaches the caller, so
// auditing can neither slow nor fail the action it describes.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	"github.com/vibexp/vibexp/pkg/events"
)

// Event is what a feature reports about one audited operation. The recorder
// adds everything it can read from the request context: the actor, the
// channel, the API key, the client IP and user agent, and the request id.
type Event struct {
	Operation models.AuditOperation
	// Outcome defaults to success.
	Outcome models.AuditOutcome
	// Actor overrides the actor read from the context. Set it when the context
	// has no user: a failed sign-in (the attempted email, nil UserID) or a
	// system job.
	Actor *Actor
	// Source overrides the channel derived from the context.
	Source models.AuditSource

	// The names are optional: a missing team or project name is looked up off
	// the request path. Pass ResourceName on a delete, since it cannot be
	// looked up afterwards (resource names are never looked up).
	TeamID, TeamName, ProjectID, ProjectName string
	ResourceType, ResourceID, ResourceName   string

	// Metadata holds the changed fields (old → new for settings and roles).
	// The caller redacts secrets; resource content never belongs here. It is
	// serialized when Record is called, so the caller may reuse the map
	// afterwards.
	Metadata map[string]any
}

// Actor names who performed an operation when the context cannot say.
type Actor struct {
	UserID *string
	Email  string
	Name   string
	// Type defaults to user when UserID is set, otherwise to anonymous for an
	// HTTP request and system outside one.
	Type models.AuditActorType
}

// Recorder records audit events.
//
// Contract: Record must not block or do I/O on the caller's goroutine. It is
// called on request paths (sign-in, every resource write) and must never slow
// them. AsyncRecorder guarantees this: Record only snapshots the context and
// the event (no I/O; the name lookups, the capture setting and the insert all
// run in persist, on the worker pool), and events.WorkerPool is built with
// ants.WithNonblocking(true), so Submit never waits for a free worker (it falls
// back to a dedicated goroutine). Callers still go through SafeRecord, which
// recovers a panicking implementation.
type Recorder interface {
	// Record captures ctx-derived fields synchronously and persists asynchronously.
	// Never blocks on I/O, never returns an error, never panics the caller.
	Record(ctx context.Context, e Event)
}

// DropCounter counts audit events that could not be recorded in full, by
// reason. *metrics.Metrics implements it.
type DropCounter interface {
	RecordAuditEventDropped(ctx context.Context, reason string)
}

// Drop reasons, the reason attribute of vx_audit_events_dropped_total.
const (
	// DropReasonInvalid is an event with an operation, outcome, source or
	// actor type outside the vocabulary. It is not stored.
	DropReasonInvalid = "invalid"
	// DropReasonLookup is a failed display-name lookup. The event is still
	// stored, without that name.
	DropReasonLookup = "lookup"
	// DropReasonMetadata is metadata that could not be serialized. The event
	// is still stored, with a marker in place of the metadata.
	DropReasonMetadata = "metadata"
	// DropReasonPersist is a failed insert. The event is lost.
	DropReasonPersist = "persist"
	// DropReasonPanic is a panic while recording. The event is lost.
	DropReasonPanic = "panic"
)

// Field caps, in bytes, applied on rune boundaries.
const (
	maxActorEmailBytes   = 320
	maxResourceNameBytes = 512
	// MaxUserAgentBytes caps a stored user agent. The user-agent middleware
	// applies the same cap before the value reaches the context.
	MaxUserAgentBytes = 512
	// maxRequestIDBytes caps the request id, which RequestIDMiddleware takes
	// verbatim from a client's X-Request-ID header when one is sent.
	maxRequestIDBytes = 128
	// maxMetadataBytes caps the serialized metadata; anything larger is
	// replaced by truncatedMetadata.
	maxMetadataBytes = 16 * 1024
)

// persistTimeout bounds the lookups and the insert of one event.
const persistTimeout = 5 * time.Second

// authTypeCookie is the contextkeys.AuthType of a cookie-authenticated request
// (see internal/server/middleware.go).
const authTypeCookie = "cookie"

var (
	truncatedMetadata      = json.RawMessage(`{"truncated":true}`)
	unserializableMetadata = json.RawMessage(`{"unserializable":true}`)
)

// taskSubmitter is the part of *events.WorkerPool the recorder uses.
type taskSubmitter interface {
	Submit(task func())
}

// RecorderDeps are the collaborators of the recorder. The lookup repositories
// resolve display names off the request path.
type RecorderDeps struct {
	Repo     repositories.AuditEventRepository
	Users    repositories.UserRepository
	APIKeys  repositories.APIKeyRepository
	Teams    repositories.TeamRepository
	Projects repositories.ProjectRepository
	Capture  ClientInfoCapturer
	Pool     *events.WorkerPool
	Drops    DropCounter
	Logger   *slog.Logger
}

// AsyncRecorder implements Recorder on the events worker pool.
type AsyncRecorder struct {
	repo      repositories.AuditEventRepository
	users     repositories.UserRepository
	apiKeys   repositories.APIKeyRepository
	teams     repositories.TeamRepository
	projects  repositories.ProjectRepository
	capture   ClientInfoCapturer
	submitter taskSubmitter
	drops     DropCounter
	logger    *slog.Logger
}

var _ Recorder = (*AsyncRecorder)(nil)

// NewRecorder creates the audit recorder.
func NewRecorder(deps RecorderDeps) *AsyncRecorder {
	return newRecorder(deps, deps.Pool)
}

// newRecorder lets tests run the persistence task synchronously.
func newRecorder(deps RecorderDeps, submitter taskSubmitter) *AsyncRecorder {
	capture := deps.Capture
	if capture == nil {
		capture = AlwaysCapture{}
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &AsyncRecorder{
		repo:      deps.Repo,
		users:     deps.Users,
		apiKeys:   deps.APIKeys,
		teams:     deps.Teams,
		projects:  deps.Projects,
		capture:   capture,
		submitter: submitter,
		drops:     deps.Drops,
		logger:    logger,
	}
}

// pending is an event captured on the caller's goroutine, waiting for its
// names to be resolved and its row written.
type pending struct {
	row models.AuditEvent
	// lookupActor is set when the actor's name or email still needs filling.
	lookupActor bool
	// lookupTeam and lookupProject are set when the caller passed an id
	// without its name.
	lookupTeam    bool
	lookupProject bool
}

// Record captures the request-scoped facts now and persists the event on the
// worker pool. It returns immediately and never panics the caller.
func (r *AsyncRecorder) Record(ctx context.Context, e Event) {
	defer func() {
		if rec := recover(); rec != nil {
			r.drop(DropReasonPanic, "recovered from panic while capturing an audit event", e.Operation, "panic", rec)
		}
	}()

	p, ok := r.snapshot(ctx, e)
	if !ok {
		return
	}
	r.submitter.Submit(func() { r.persist(p) })
}

// snapshot builds the event from e and ctx. It does no I/O.
func (r *AsyncRecorder) snapshot(ctx context.Context, e Event) (*pending, bool) {
	outcome := e.Outcome
	if outcome == "" {
		outcome = models.AuditOutcomeSuccess
	}
	isRequest := contextkeys.GetRequestID(ctx) != ""
	userAgent := contextString(ctx, contextkeys.UserAgent)

	source, assistant := e.Source, (*string)(nil)
	if source == "" {
		source, assistant = deriveSource(ctx, isRequest, userAgent)
	}

	actorUserID, actorEmail, actorName, actorType := actorFromContext(ctx, isRequest)
	if e.Actor != nil {
		actorUserID, actorEmail, actorName, actorType = resolveActor(*e.Actor, isRequest)
	}

	if !e.Operation.IsValid() || !outcome.IsValid() || !source.IsValid() || !actorType.IsValid() {
		r.drop(DropReasonInvalid, "dropped an audit event outside the vocabulary", e.Operation,
			"outcome", outcome, "source", source, "actor_type", actorType)
		return nil, false
	}

	p := &pending{
		row: models.AuditEvent{
			ActorUserID:   actorUserID,
			ActorEmail:    TruncateBytes(actorEmail, maxActorEmailBytes),
			ActorName:     actorName,
			ActorType:     actorType,
			TeamID:        optional(e.TeamID),
			TeamName:      optional(e.TeamName),
			ProjectID:     optional(e.ProjectID),
			ProjectName:   optional(e.ProjectName),
			Operation:     e.Operation,
			ResourceType:  optional(e.ResourceType),
			ResourceID:    optional(e.ResourceID),
			ResourceName:  optional(TruncateBytes(e.ResourceName, maxResourceNameBytes)),
			Source:        source,
			AssistantName: assistant,
			APIKeyID:      optional(contextString(ctx, contextkeys.APIKeyID)),
			Outcome:       outcome,
			IP:            optional(contextString(ctx, contextkeys.ClientIP)),
			UserAgent:     optional(TruncateBytes(userAgent, MaxUserAgentBytes)),
			RequestID:     optional(TruncateBytes(contextkeys.GetRequestID(ctx), maxRequestIDBytes)),
			Metadata:      r.marshalMetadata(e),
		},
		lookupActor:   actorUserID != nil && (actorEmail == "" || actorName == ""),
		lookupTeam:    e.TeamID != "" && e.TeamName == "",
		lookupProject: e.ProjectID != "" && e.ProjectName == "",
	}
	return p, true
}

// deriveSource returns the channel of the request in ctx. A write origin set
// by the server wins (it also carries the MCP client's name); otherwise the
// channel follows from how the request was authenticated. Outside a request
// the source is system.
func deriveSource(ctx context.Context, isRequest bool, userAgent string) (models.AuditSource, *string) {
	if origin := resourceaccess.WriteOriginFromContext(ctx); origin != nil {
		return models.AuditSource(origin.Source), origin.AssistantName
	}
	if !isRequest {
		return models.AuditSourceSystem, nil
	}
	authType := contextString(ctx, contextkeys.AuthType)
	if authType == "" {
		// An unauthenticated request (a sign-in attempt) comes from the web
		// app's sign-in pages.
		return models.AuditSourceWeb, nil
	}
	if authType == authTypeCookie {
		return models.AuditSourceWeb, nil
	}
	// The request path is not in the context, so an API-key request is cli
	// (by user agent) or api. Only the MCP create/update tools set a write
	// origin (withMCPWriteOrigin); any other MCP call site must pass Source
	// mcp or wrap its context the same way, or it is recorded as api.
	return models.AuditSource(resourceaccess.DeriveSource(authType, "", userAgent)), nil
}

// actorFromContext returns the authenticated user of ctx as the actor.
func actorFromContext(
	ctx context.Context, isRequest bool,
) (userID *string, email, name string, actorType models.AuditActorType) {
	userID = optional(contextString(ctx, contextkeys.UserID))
	return userID, contextString(ctx, contextkeys.UserEmail), "", defaultActorType(userID, isRequest)
}

// resolveActor applies a caller-supplied actor, defaulting its type.
func resolveActor(a Actor, isRequest bool) (*string, string, string, models.AuditActorType) {
	userID := a.UserID
	if userID != nil && *userID == "" {
		userID = nil
	}
	actorType := a.Type
	if actorType == "" {
		actorType = defaultActorType(userID, isRequest)
	}
	return userID, a.Email, a.Name, actorType
}

func defaultActorType(userID *string, isRequest bool) models.AuditActorType {
	switch {
	case userID != nil:
		return models.AuditActorUser
	case isRequest:
		return models.AuditActorAnonymous
	default:
		return models.AuditActorSystem
	}
}

// marshalMetadata serializes the metadata now, so the caller's map is never
// read from another goroutine.
func (r *AsyncRecorder) marshalMetadata(e Event) json.RawMessage {
	if len(e.Metadata) == 0 {
		return json.RawMessage(`{}`)
	}
	doc, err := json.Marshal(e.Metadata)
	if err != nil {
		r.drop(DropReasonMetadata, "audit event metadata could not be serialized", e.Operation, "error", err)
		return unserializableMetadata
	}
	if len(doc) > maxMetadataBytes {
		return truncatedMetadata
	}
	return doc
}

// persist resolves the missing names and writes the row. It runs on the
// worker pool, on a fresh time-bounded context, and is panic-safe.
func (r *AsyncRecorder) persist(p *pending) {
	defer func() {
		if rec := recover(); rec != nil {
			r.drop(DropReasonPanic, "recovered from panic while recording an audit event", p.row.Operation, "panic", rec)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), persistTimeout)
	defer cancel()

	r.resolveNames(ctx, p)

	if !r.capture.CaptureClientInfo(ctx) {
		p.row.IP = nil
		p.row.UserAgent = nil
	}

	if err := r.repo.Insert(ctx, &p.row); err != nil {
		r.drop(DropReasonPersist, "failed to record audit event", p.row.Operation, "error", err)
	}
}

// resolveNames fills the actor, API key, team and project names the event
// lacks. A failed lookup leaves that name empty and is counted once per event;
// an id that no longer resolves (a deleted team) is not a failure.
func (r *AsyncRecorder) resolveNames(ctx context.Context, p *pending) {
	failed := false
	fail := func(what string, err error) {
		failed = true
		r.logger.With("error", err, "operation", p.row.Operation, "lookup", what).
			Warn("audit event name lookup failed")
	}

	if err := r.resolveActor(ctx, p); err != nil {
		fail("actor", err)
	}
	if err := r.resolveAPIKey(ctx, p); err != nil {
		fail("api_key", err)
	}
	if err := r.resolveTeam(ctx, p); err != nil {
		fail("team", err)
	}
	if err := r.resolveProject(ctx, p); err != nil {
		fail("project", err)
	}

	if failed {
		r.countDrop(ctx, DropReasonLookup)
	}
}

func (r *AsyncRecorder) resolveActor(ctx context.Context, p *pending) error {
	if !p.lookupActor {
		return nil
	}
	user, err := r.users.GetByID(ctx, *p.row.ActorUserID)
	if errors.Is(err, repositories.ErrUserNotFound) || (err == nil && user == nil) {
		return nil // a deleted user keeps the snapshot the event already has
	}
	if err != nil {
		return err
	}
	if p.row.ActorEmail == "" {
		p.row.ActorEmail = TruncateBytes(user.Email, maxActorEmailBytes)
	}
	if p.row.ActorName == "" {
		p.row.ActorName = user.Name
	}
	return nil
}

// resolveAPIKey names the key the request authenticated with. Key names are
// looked up through their owner, who is the actor.
func (r *AsyncRecorder) resolveAPIKey(ctx context.Context, p *pending) error {
	if p.row.APIKeyID == nil || p.row.ActorUserID == nil {
		return nil
	}
	names, err := r.apiKeys.GetNamesByIDs(ctx, *p.row.ActorUserID, []string{*p.row.APIKeyID})
	if err != nil {
		return err
	}
	p.row.APIKeyName = optional(names[*p.row.APIKeyID])
	return nil
}

func (r *AsyncRecorder) resolveTeam(ctx context.Context, p *pending) error {
	if !p.lookupTeam {
		return nil
	}
	names, err := r.teams.GetNamesByIDs(ctx, []string{*p.row.TeamID})
	if err != nil {
		return err
	}
	p.row.TeamName = optional(names[*p.row.TeamID])
	return nil
}

// resolveProject names the project with an unscoped lookup: the actor of an
// admin, system or denial event need not be a member of the project's team.
func (r *AsyncRecorder) resolveProject(ctx context.Context, p *pending) error {
	if !p.lookupProject {
		return nil
	}
	name, err := r.projects.GetNameByIDForAudit(ctx, *p.row.ProjectID)
	if err != nil {
		return err
	}
	p.row.ProjectName = optional(name)
	return nil
}

// drop logs a recording failure at Warn and counts it.
func (r *AsyncRecorder) drop(reason, msg string, op models.AuditOperation, fields ...any) {
	r.logger.With(fields...).With("operation", op, "reason", reason).Warn(msg)
	r.countDrop(context.Background(), reason)
}

func (r *AsyncRecorder) countDrop(ctx context.Context, reason string) {
	if r.drops != nil {
		r.drops.RecordAuditEventDropped(ctx, reason)
	}
}

// contextString reads a string context value, "" when absent or not a string.
func contextString(ctx context.Context, key contextkeys.ContextKey) string {
	s, _ := ctx.Value(key).(string)
	return s
}

// optional maps "" to nil, which the table stores as NULL.
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// TruncateBytes cuts s to at most limit bytes without splitting a rune. The
// user-agent middleware shares it, so both caps cut the same way.
func TruncateBytes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
