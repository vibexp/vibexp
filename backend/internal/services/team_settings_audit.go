package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"

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

// TeamSettingsAuditServiceInterface is the write path for the team settings
// audit log (epic #827, decision 8).
//
// It carries no permission check of its own, and that is deliberate: an entry
// is written as part of an already-authorized copy, which has checked
// authz.TeamUpdate on BOTH the source and the destination team. Re-checking
// here would only be able to re-derive that answer, while making the audit
// write fail for an actor who was allowed to perform the copy — an audit the
// authorized path can silently skip is worse than no audit. The read path
// (#832) is where a permission check belongs.
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
}

// TeamSettingsAuditService implements TeamSettingsAuditServiceInterface.
type TeamSettingsAuditService struct {
	repo   repositories.TeamSettingsAuditRepository
	logger *slog.Logger
}

var _ TeamSettingsAuditServiceInterface = (*TeamSettingsAuditService)(nil)

// NewTeamSettingsAuditService creates a new TeamSettingsAuditService.
func NewTeamSettingsAuditService(
	repo repositories.TeamSettingsAuditRepository,
	logger *slog.Logger,
) *TeamSettingsAuditService {
	return &TeamSettingsAuditService{repo: repo, logger: logger}
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
