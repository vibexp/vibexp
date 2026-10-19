package services

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Knowledge Pulse: a polled, merged feed of the team's reads,
// writes, links and feed posts, plus a graph snapshot and today's counts.
//
// Membership is enforced by the tenancy middleware, as for the other team
// analytics reads; there is no role check, because the pulse shows nothing a
// member cannot already open.

// Pulse tuning. The window and caps come from the PRD's contract.
const (
	PulseDefaultLimit    = 200
	PulseMaxLimit        = 500
	PulseInitialWindow   = 15 * time.Minute
	PulseSnapshotNodeCap = 150
	PulseActivityWindow  = 24 * time.Hour
)

// Event kinds, actor types and sources as published in the spec.
const (
	PulseKindRead     = "read"
	PulseKindWrite    = "write"
	PulseKindLink     = "link"
	PulseKindFeedPost = "feed_post"

	PulseActorUser  = "user"
	PulseActorAgent = "agent"

	PulseSourceWeb = "web"
	PulseSourceCLI = "cli"
	PulseSourceMCP = "mcp"
	PulseSourceAPI = "api"

	PulseResourceFeedItem = "feed_item"
)

// KnowledgePulseServiceInterface builds the team's knowledge pulse.
type KnowledgePulseServiceInterface interface {
	GetPulse(ctx context.Context, teamID string, query KnowledgePulseQuery) (*KnowledgePulseResult, error)
}

// KnowledgePulseQuery is one poll. A nil Since means "first load": the last
// PulseInitialWindow of events plus the snapshot.
type KnowledgePulseQuery struct {
	Since           *time.Time
	Limit           int
	IncludeSnapshot bool
}

// PulseActor is a resolved person or agent.
type PulseActor struct {
	Type   string
	ID     string
	Name   string
	Source string
}

// PulseEvent is one merged event. Action is set on writes; RelationType and
// To on links.
type PulseEvent struct {
	ID           string
	Kind         string
	At           time.Time
	Actor        PulseActor
	Resource     models.PulseResource
	Action       string
	RelationType string
	To           *models.PulseResource
}

// PulseSnapshot is the graph the ring is drawn from.
type PulseSnapshot struct {
	Nodes  []models.PulseNode
	Edges  []models.PulseEdge
	Actors []PulseActor
}

// PulseStats are today's counts.
type PulseStats struct {
	Reads            int
	Writes           int
	Links            int
	CompoundingScore int
}

// KnowledgePulseResult is the service's answer to one poll.
type KnowledgePulseResult struct {
	Cursor   time.Time
	Events   []PulseEvent
	Snapshot *PulseSnapshot
	Stats    PulseStats
}

// KnowledgePulseService is the default KnowledgePulseServiceInterface.
type KnowledgePulseService struct {
	repo   repositories.KnowledgePulseRepository
	logger *slog.Logger
	now    func() time.Time
}

var _ KnowledgePulseServiceInterface = (*KnowledgePulseService)(nil)

// NewKnowledgePulseService creates a KnowledgePulseService.
func NewKnowledgePulseService(repo repositories.KnowledgePulseRepository, logger *slog.Logger) *KnowledgePulseService {
	return &KnowledgePulseService{repo: repo, logger: logger, now: time.Now}
}

// ClampPulseLimit applies the default and the cap to a requested limit.
func ClampPulseLimit(limit int) int {
	switch {
	case limit <= 0:
		return PulseDefaultLimit
	case limit > PulseMaxLimit:
		return PulseMaxLimit
	default:
		return limit
	}
}

// GetPulse reads every source after the cursor, merges them and computes the
// cursor, today's stats and (on first load or on request) the snapshot.
func (s *KnowledgePulseService) GetPulse(
	ctx context.Context, teamID string, query KnowledgePulseQuery,
) (*KnowledgePulseResult, error) {
	now := s.now().UTC()
	since := now.Add(-PulseInitialWindow)
	includeSnapshot := true
	if query.Since != nil {
		since = query.Since.UTC()
		includeSnapshot = query.IncludeSnapshot
	}
	limit := ClampPulseLimit(query.Limit)

	events, err := s.listSince(ctx, teamID, since, limit)
	if err != nil {
		return nil, err
	}
	// A first load wants the NEWEST events, but the sources return the oldest
	// after the cursor. When the window holds more than a page, narrow it until
	// the page reaches (close to) now, so the ring starts live instead of
	// replaying a backlog poll by poll.
	for window := PulseInitialWindow; query.Since == nil && len(events) >= limit && window > pulseMinInitialWindow; {
		window /= 4
		since = now.Add(-window)
		if events, err = s.listSince(ctx, teamID, since, limit); err != nil {
			return nil, err
		}
	}

	result := &KnowledgePulseResult{Cursor: since, Events: events}
	if len(events) > 0 {
		result.Cursor = events[len(events)-1].At
	}

	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	day, err := s.repo.GetDayStats(ctx, teamID, dayStart)
	if err != nil {
		return nil, fmt.Errorf("get pulse day stats: %w", err)
	}
	result.Stats = PulseStatsFromDay(day)

	if includeSnapshot {
		raw, err := s.repo.GetSnapshot(ctx, teamID, now.Add(-PulseActivityWindow), PulseSnapshotNodeCap)
		if err != nil {
			return nil, fmt.Errorf("get pulse snapshot: %w", err)
		}
		result.Snapshot = buildPulseSnapshot(raw)
	}
	return result, nil
}

// pulseMinInitialWindow is how far a busy team's first load narrows its
// window at most (see GetPulse).
const pulseMinInitialWindow = 30 * time.Second

// listSince reads every source after since and merges them into one page.
func (s *KnowledgePulseService) listSince(
	ctx context.Context, teamID string, since time.Time, limit int,
) ([]PulseEvent, error) {
	reads, err := s.repo.ListReadsSince(ctx, teamID, since, limit)
	if err != nil {
		return nil, fmt.Errorf("list pulse reads: %w", err)
	}
	writes, err := s.repo.ListWritesSince(ctx, teamID, since, limit)
	if err != nil {
		return nil, fmt.Errorf("list pulse writes: %w", err)
	}
	links, err := s.repo.ListLinksSince(ctx, teamID, since, limit)
	if err != nil {
		return nil, fmt.Errorf("list pulse links: %w", err)
	}
	posts, err := s.repo.ListFeedPostsSince(ctx, teamID, since, limit)
	if err != nil {
		return nil, fmt.Errorf("list pulse feed posts: %w", err)
	}
	return MergePulseEvents(reads, writes, links, posts, limit), nil
}

// PulseStatsFromDay turns raw counts into stats_today. The compounding score
// is new links, plus supersedes links (counted again: consolidation is the
// strongest compounding signal), plus updates to already-linked resources.
func PulseStatsFromDay(day models.PulseDayStats) PulseStats {
	return PulseStats{
		Reads:            day.Reads,
		Writes:           day.Writes,
		Links:            day.Links,
		CompoundingScore: day.Links + day.Supersedes + day.LinkedUpdates,
	}
}

// MergePulseEvents converts every source row to a PulseEvent, sorts the lot by
// time (oldest first, ties broken by id for a stable order) and keeps the
// first limit. Keeping the OLDEST is what makes the cursor safe: each source
// returned its oldest rows after the cursor, so nothing older than the last
// kept event can be missing, and the next poll resumes exactly there.
func MergePulseEvents(
	reads []models.PulseReadEvent,
	writes []models.PulseWriteEvent,
	links []models.PulseLinkEvent,
	posts []models.PulseFeedEvent,
	limit int,
) []PulseEvent {
	events := make([]PulseEvent, 0, len(reads)+len(writes)+len(links)+len(posts))
	for _, r := range reads {
		events = append(events, PulseEvent{
			ID: PulseKindRead + ":" + r.ID, Kind: PulseKindRead, At: r.At,
			Actor: ResolvePulseActor(r.Actor), Resource: r.Resource,
		})
	}
	for _, w := range writes {
		events = append(events, PulseEvent{
			ID: PulseKindWrite + ":" + w.ID, Kind: PulseKindWrite, At: w.At,
			Actor: ResolvePulseActor(w.Actor), Resource: w.Resource, Action: w.Action,
		})
	}
	for _, l := range links {
		to := l.To
		events = append(events, PulseEvent{
			ID: PulseKindLink + ":" + l.ID, Kind: PulseKindLink, At: l.At,
			Actor: linkActor(l), Resource: l.From, RelationType: l.RelationType, To: &to,
		})
	}
	for _, p := range posts {
		events = append(events, PulseEvent{
			ID: PulseKindFeedPost + ":" + p.ID, Kind: PulseKindFeedPost, At: p.At,
			Actor: feedActor(p),
			Resource: models.PulseResource{
				Type: PulseResourceFeedItem, ID: p.ID, Title: p.Title, ProjectID: p.ProjectID,
			},
		})
	}
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].At.Equal(events[j].At) {
			return events[i].At.Before(events[j].At)
		}
		return events[i].ID < events[j].ID
	})
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}
	return events
}

// ResolvePulseActor decides who caused a read or write. A recorded assistant
// name wins; a web event is the person; anything else is an agent named from
// the user agent, then the API key's name, then the channel itself.
func ResolvePulseActor(a models.PulseActorRaw) PulseActor {
	source := normalizePulseSource(a.Source)
	if a.AssistantName != "" {
		return pulseAgent(canonicalAgentName(a.AssistantName, true), source)
	}
	if source == PulseSourceWeb {
		return pulseUser(a.UserID, a.UserName, source)
	}
	name := canonicalAgentName(a.UserAgent, false)
	if name == "" {
		name = strings.TrimSpace(a.APIKeyName)
	}
	if name == "" {
		name = pulseSourceLabel(source)
	}
	return pulseAgent(name, source)
}

// linkActor: relations record only who and whether the AI proposed them.
func linkActor(l models.PulseLinkEvent) PulseActor {
	if l.Origin == "ai" {
		return pulseAgent("AI agent", PulseSourceMCP)
	}
	return pulseUser(l.CreatedByID, l.CreatedByName, PulseSourceWeb)
}

// feedActor: feed posts are written by agents, which name themselves.
func feedActor(p models.PulseFeedEvent) PulseActor {
	if name := canonicalAgentName(p.AssistantName, true); name != "" {
		return pulseAgent(name, PulseSourceMCP)
	}
	return pulseUser(p.UserID, p.UserName, PulseSourceWeb)
}

func pulseUser(id, name, source string) PulseActor {
	if name == "" {
		name = "A teammate"
	}
	if id == "" {
		id = "user:unknown"
	}
	return PulseActor{Type: PulseActorUser, ID: id, Name: name, Source: source}
}

// pulseAgent keys an agent by its name, so one tool used by several people is
// one actor on the outer ring.
func pulseAgent(name, source string) PulseActor {
	key := strings.Join(strings.Fields(strings.ToLower(name)), "-")
	return PulseActor{Type: PulseActorAgent, ID: "agent:" + key, Name: name, Source: source}
}

func normalizePulseSource(source string) string {
	switch source {
	case PulseSourceWeb, PulseSourceCLI, PulseSourceMCP, PulseSourceAPI:
		return source
	default:
		return PulseSourceAPI
	}
}

func pulseSourceLabel(source string) string {
	switch source {
	case PulseSourceMCP:
		return "MCP client"
	case PulseSourceCLI:
		return "VibeXP CLI"
	default:
		return "API client"
	}
}

// knownAgents maps a lower-cased token found in an assistant name or user
// agent to a display name. Order matters: the first match wins, so the more
// specific tokens come first.
var knownAgents = []struct{ token, name string }{
	{"claude-code", "Claude Code"},
	{"claude code", "Claude Code"},
	{"claudecode", "Claude Code"},
	{"cursor", "Cursor"},
	{"codex", "Codex"},
	{"copilot", "GitHub Copilot"},
	{"vscode", "VS Code"},
	{"visual studio code", "VS Code"},
	{"windsurf", "Windsurf"},
	{"gemini", "Gemini CLI"},
	{"vibexp-cli", "VibeXP CLI"},
	{"vibexp/", "VibeXP CLI"},
	{"claude", "Claude"},
}

// canonicalAgentName maps raw to a known agent name. An unrecognised value is
// kept as-is when keepUnknown (an assistant name is chosen by the agent), and
// dropped otherwise (a user agent like "Go-http-client/1.1" names nobody).
func canonicalAgentName(raw string, keepUnknown bool) string {
	raw = strings.TrimSpace(raw)
	lower := strings.ToLower(raw)
	for _, k := range knownAgents {
		if strings.Contains(lower, k.token) {
			return k.name
		}
	}
	if keepUnknown {
		return raw
	}
	return ""
}

func buildPulseSnapshot(raw *models.PulseSnapshotRaw) *PulseSnapshot {
	snap := &PulseSnapshot{
		Nodes:  raw.Nodes,
		Edges:  raw.Edges,
		Actors: make([]PulseActor, 0, len(raw.Actors)),
	}
	seen := make(map[string]bool, len(raw.Actors))
	for _, a := range raw.Actors {
		actor := ResolvePulseActor(a)
		if seen[actor.ID] {
			continue
		}
		seen[actor.ID] = true
		snap.Actors = append(snap.Actors, actor)
	}
	sort.Slice(snap.Actors, func(i, j int) bool { return snap.Actors[i].Name < snap.Actors[j].Name })
	return snap
}
