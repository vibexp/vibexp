package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ErrInvalidSettingsAudit is returned when an entry does not identify what was
// copied, where to, or onto which surface. It is a programming error rather
// than user input — every field it guards is supplied by the copy service, not
// by a request body — so it is deliberately not mapped to a status code.
var ErrInvalidSettingsAudit = errors.New("invalid team settings audit entry")

// TeamSettingsAuditRecord is one settings-copy event as the copy services
// (#829/#830/#831) describe it.
//
// It is a separate type from models.TeamSettingsAudit rather than the model
// itself because the caller supplies only the facts of the copy: id and
// created_at are the database's to assign, and accepting a model would invite a
// caller to set them.
//
// SourceResourceID and CreatedResourceID are optional and both are absent for a
// custom-types copy, which produces many rows from many rows in one action —
// their ids belong in Detail. ActorUserID is optional so a copy performed by
// something other than a signed-in user still records the event rather than
// failing; every current caller has one.
type TeamSettingsAuditRecord struct {
	TeamID            string
	ActorUserID       string
	Surface           string
	SourceTeamID      string
	SourceResourceID  string
	CreatedResourceID string
	Detail            map[string]interface{}
}

// TeamSettingsAuditServiceInterface is the team settings audit log (epic #827,
// decision 8): an unauthorized write path and an authorized read path.
//
// Record carries no permission check of its own, and that is deliberate: an
// entry is written as part of an already-authorized copy, which has checked
// authz.TeamUpdate on BOTH the source and the destination team. Re-checking
// there would only be able to re-derive that answer, while making the audit
// write fail for an actor who was allowed to perform the copy — an audit the
// authorized path can silently skip is worse than no audit. ListAudit is where
// the permission check belongs, and it makes one.
type TeamSettingsAuditServiceInterface interface {
	// Record appends one settings-copy event and returns the stored entry.
	//
	// It returns an ErrInvalidSettingsAudit-wrapped error for an entry with no
	// destination team, no source team, or an unrecognized surface, and it
	// propagates a storage failure unchanged. Callers must NOT swallow that
	// error: the entry is the compensating control for a copy that moved a
	// credential's use into a different set of members, so a copy whose audit
	// did not land must not report success.
	Record(ctx context.Context, record TeamSettingsAuditRecord) (*models.TeamSettingsAudit, error)

	// ListAudit returns one page of the team's settings audit log, newest
	// first, with the actor and source-team display names resolved.
	//
	// Requires authz.TeamSettingsUpdate — the same owner/admin gate as the
	// other team settings surfaces, and deliberately NOT the any-member read
	// the neighbouring freshness audit allows: this log names who brought a
	// credential-bearing configuration in and from where, which is exactly the
	// question the epic's compensating control exists to answer for the team's
	// owners. Returns ErrPermissionDenied when the caller's role does not
	// grant it.
	//
	// page and limit are clamped UP to their minimums only (1 and the spec's
	// default of 20). Enforcing the maximums is the caller's, because rejecting
	// an out-of-range request is a 400 the HTTP layer owns — the handler does
	// it in settingsAuditPaging.
	ListAudit(
		ctx context.Context, userID, teamID string, page, limit int,
	) (*models.TeamSettingsAuditPage, error)
}

// TeamSettingsAuditService implements TeamSettingsAuditServiceInterface.
type TeamSettingsAuditService struct {
	repo   repositories.TeamSettingsAuditRepository
	authz  AuthorizationServiceInterface
	users  repositories.UserRepository
	teams  repositories.TeamRepository
	logger *slog.Logger
}

var _ TeamSettingsAuditServiceInterface = (*TeamSettingsAuditService)(nil)

// NewTeamSettingsAuditService creates a new TeamSettingsAuditService.
func NewTeamSettingsAuditService(
	repo repositories.TeamSettingsAuditRepository,
	authzService AuthorizationServiceInterface,
	users repositories.UserRepository,
	teams repositories.TeamRepository,
	logger *slog.Logger,
) *TeamSettingsAuditService {
	return &TeamSettingsAuditService{
		repo:   repo,
		authz:  authzService,
		users:  users,
		teams:  teams,
		logger: logger,
	}
}

// Record appends one settings-copy event.
func (s *TeamSettingsAuditService) Record(
	ctx context.Context, record TeamSettingsAuditRecord,
) (*models.TeamSettingsAudit, error) {
	if record.TeamID == "" {
		return nil, fmt.Errorf("%w: destination team is required", ErrInvalidSettingsAudit)
	}
	// The source team is required even though the column has no foreign key:
	// "this configuration came from somewhere else" is the entire claim the
	// entry makes, and an entry without it records only that something changed.
	if record.SourceTeamID == "" {
		return nil, fmt.Errorf("%w: source team is required", ErrInvalidSettingsAudit)
	}
	if !slices.Contains(models.SettingsAuditSurfaces, record.Surface) {
		return nil, fmt.Errorf("%w: surface %q is not a copyable settings surface",
			ErrInvalidSettingsAudit, record.Surface)
	}

	detail, err := marshalAuditDetail(record.Detail)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSettingsAudit, err)
	}

	entry := &models.TeamSettingsAudit{
		TeamID:            record.TeamID,
		ActorUserID:       optionalID(record.ActorUserID),
		Surface:           record.Surface,
		SourceTeamID:      optionalID(record.SourceTeamID),
		SourceResourceID:  optionalID(record.SourceResourceID),
		CreatedResourceID: optionalID(record.CreatedResourceID),
		Detail:            detail,
	}

	if err := s.repo.Append(ctx, entry); err != nil {
		return nil, fmt.Errorf("failed to record team settings audit entry: %w", err)
	}

	s.logger.Info("Recorded team settings copy",
		"team_id", entry.TeamID, "source_team_id", record.SourceTeamID,
		"surface", entry.Surface, "actor_user_id", record.ActorUserID,
	)
	return entry, nil
}

// marshalAuditDetail renders the per-surface context as jsonb. A nil or empty
// map yields nil, which the repository turns into an empty object — so a
// caller with nothing to add never has to spell one out.
func marshalAuditDetail(detail map[string]interface{}) (json.RawMessage, error) {
	if len(detail) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		return nil, fmt.Errorf("detail is not serializable: %w", err)
	}
	return encoded, nil
}

// optionalID maps an empty id to nil, so an absent optional reference is stored
// as SQL NULL rather than as an empty string that would fail the column's uuid
// type.
func optionalID(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

// defaultSettingsAuditPageSize matches the spec's default for the `limit` query
// parameter.
const defaultSettingsAuditPageSize = 20

// ListAudit returns one page of the team's settings audit log, newest first.
func (s *TeamSettingsAuditService) ListAudit(
	ctx context.Context, userID, teamID string, page, limit int,
) (*models.TeamSettingsAuditPage, error) {
	if err := s.authz.Can(ctx, userID, teamID, authz.TeamSettingsUpdate); err != nil {
		return nil, err
	}

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = defaultSettingsAuditPageSize
	}

	entries, total, err := s.repo.ListByTeam(ctx, teamID, limit, (page-1)*limit)
	if err != nil {
		return nil, fmt.Errorf("TeamSettingsAuditService.ListAudit: %w", err)
	}

	return &models.TeamSettingsAuditPage{
		Entries:    s.resolveNames(ctx, teamID, entries),
		TotalCount: total,
		Page:       page,
		PerPage:    limit,
	}, nil
}

// resolveNames attaches the actor and source-team display names to a page of
// entries with two batched lookups, so rendering a page costs two queries
// rather than two per row.
//
// A failed lookup is logged and DEGRADED to no names rather than failing the
// read: the ids on every entry are the audit record, the names are only there
// to make it legible, and a team investigating what arrived in their settings
// is worse served by an error page than by a page of ids.
func (s *TeamSettingsAuditService) resolveNames(
	ctx context.Context, teamID string, entries []*models.TeamSettingsAudit,
) []*models.TeamSettingsAuditEntryView {
	actorIDs := make([]string, 0, len(entries))
	teamIDs := make([]string, 0, len(entries))
	seenActors := make(map[string]struct{}, len(entries))
	seenTeams := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		collectID(entry.ActorUserID, seenActors, &actorIDs)
		collectID(entry.SourceTeamID, seenTeams, &teamIDs)
	}

	actorNames, err := s.users.GetNamesByIDs(ctx, actorIDs)
	if err != nil {
		s.logger.Warn("Failed to resolve settings audit actor names",
			"team_id", teamID, "error", err)
		actorNames = nil
	}
	teamNames, err := s.teams.GetNamesByIDs(ctx, teamIDs)
	if err != nil {
		s.logger.Warn("Failed to resolve settings audit source team names",
			"team_id", teamID, "error", err)
		teamNames = nil
	}

	views := make([]*models.TeamSettingsAuditEntryView, 0, len(entries))
	for _, entry := range entries {
		views = append(views, &models.TeamSettingsAuditEntryView{
			Entry:          entry,
			ActorName:      lookupName(actorNames, entry.ActorUserID),
			SourceTeamName: lookupName(teamNames, entry.SourceTeamID),
		})
	}
	return views
}

// collectID appends a non-nil, not-yet-seen id to ids, so one lookup covers a
// page in which the same actor or source team appears on many rows.
func collectID(id *string, seen map[string]struct{}, ids *[]string) {
	if id == nil || *id == "" {
		return
	}
	if _, ok := seen[*id]; ok {
		return
	}
	seen[*id] = struct{}{}
	*ids = append(*ids, *id)
}

// lookupName resolves one id against a name map. It returns nil for a nil id
// and for an id the map does not carry — the referenced row has been deleted,
// which for this log is an expected outcome and not an error.
func lookupName(names map[string]string, id *string) *string {
	if id == nil || names == nil {
		return nil
	}
	name, ok := names[*id]
	if !ok {
		return nil
	}
	return &name
}
