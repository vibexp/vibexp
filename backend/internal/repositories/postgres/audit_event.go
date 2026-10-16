package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// AuditEventRepository implements repositories.AuditEventRepository on the
// audit_events table (#1511).
type AuditEventRepository struct {
	db *database.DB
}

var _ repositories.AuditEventRepository = (*AuditEventRepository)(nil)

// NewAuditEventRepository creates a new PostgreSQL audit event repository.
func NewAuditEventRepository(db *database.DB) *AuditEventRepository {
	return &AuditEventRepository{db: db}
}

// emptyAuditMetadata is what an event with no metadata stores: the column is
// NOT NULL and the readers expect an object.
var emptyAuditMetadata = []byte("{}")

// auditEventColumns is the column list of every read, in the order
// scanAuditEventDest scans it.
var auditEventColumns = []string{
	"id", "created_at", "actor_user_id", "actor_email", "actor_name", "actor_type",
	"team_id", "team_name", "project_id", "project_name", "operation",
	"resource_type", "resource_id", "resource_name", "source", "assistant_name",
	"api_key_id", "api_key_name", "outcome", "ip", "user_agent", "request_id", "metadata",
}

// The ID and the timestamp fall back to the column defaults when the event
// leaves them unset. Binding them (rather than leaving them out) lets a caller
// that has a time of its own, such as a test fixture, store it.
const insertAuditEventQuery = `
	INSERT INTO audit_events (
		id, created_at, actor_user_id, actor_email, actor_name, actor_type,
		team_id, team_name, project_id, project_name, operation,
		resource_type, resource_id, resource_name, source, assistant_name,
		api_key_id, api_key_name, outcome, ip, user_agent, request_id, metadata
	) VALUES (
		COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), COALESCE($2::timestamptz, now()),
		$3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23
	)
	RETURNING id, created_at`

// purgeAuditEventsBatchQuery deletes one batch. The LIMIT lives in a subquery
// because Postgres DELETE has none; created_at uses idx_audit_events_created.
const purgeAuditEventsBatchQuery = `
	DELETE FROM audit_events
	WHERE id IN (SELECT id FROM audit_events WHERE created_at < $1 LIMIT $2)`

// Insert stores one audit event and writes back its id and created_at.
func (r *AuditEventRepository) Insert(ctx context.Context, e *models.AuditEvent) error {
	if e == nil {
		return errors.New("audit event is nil")
	}

	var createdAt *time.Time
	if !e.CreatedAt.IsZero() {
		createdAt = &e.CreatedAt
	}

	err := r.db.QueryRowContext(ctx, insertAuditEventQuery,
		e.ID, createdAt, e.ActorUserID, e.ActorEmail, e.ActorName, string(e.ActorType),
		e.TeamID, e.TeamName, e.ProjectID, e.ProjectName, string(e.Operation),
		e.ResourceType, e.ResourceID, e.ResourceName, string(e.Source), e.AssistantName,
		e.APIKeyID, e.APIKeyName, string(e.Outcome), e.IP, e.UserAgent, e.RequestID,
		auditMetadataParam(e.Metadata),
	).Scan(&e.ID, &e.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert audit event: %w", err)
	}
	return nil
}

// auditMetadataParam binds metadata as jsonb, mapping an absent document (nil,
// empty or JSON null) to an empty object.
func auditMetadataParam(doc json.RawMessage) []byte {
	trimmed := strings.TrimSpace(string(doc))
	if trimmed == "" || trimmed == "null" {
		return emptyAuditMetadata
	}
	return []byte(doc)
}

// List returns one newest-first page of the events matching f, and the cursor
// of the next page (nil when the log is exhausted).
//
// It fetches one row more than the page to learn whether a next page exists,
// so the last page never hands back a cursor leading to an empty page. The id
// breaks created_at ties, so rows sharing a timestamp are neither repeated nor
// skipped across pages.
func (r *AuditEventRepository) List(
	ctx context.Context, f models.AuditEventFilter,
) ([]models.AuditEvent, *models.AuditCursor, error) {
	limit := clampAuditEventLimit(f.Limit)

	query, args, err := psql.Select(auditEventColumns...).
		From("audit_events").
		Where(buildAuditEventWhere(f)).
		OrderBy("created_at DESC", "id DESC").
		Limit(uint64(limit) + 1). // #nosec G115 -- limit is clamped to 1..AuditEventListMaxLimit
		ToSql()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build audit event list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list audit events: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close audit event rows", "error", closeErr)
		}
	}()

	events := make([]models.AuditEvent, 0, limit)
	for rows.Next() {
		var e models.AuditEvent
		if err := rows.Scan(scanAuditEventDest(&e)...); err != nil {
			return nil, nil, fmt.Errorf("failed to scan audit event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed to iterate audit events: %w", err)
	}

	if len(events) <= limit {
		return events, nil, nil
	}
	events = events[:limit]
	last := events[limit-1]
	return events, &models.AuditCursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
}

// clampAuditEventLimit maps a requested page size into
// [1, AuditEventListMaxLimit], with non-positive meaning the default.
func clampAuditEventLimit(limit int) int {
	if limit <= 0 {
		return repositories.AuditEventListDefaultLimit
	}
	return min(limit, repositories.AuditEventListMaxLimit)
}

// Count returns how many events match f's filters, ignoring f.Limit and
// f.After.
func (r *AuditEventRepository) Count(ctx context.Context, f models.AuditEventFilter) (int, error) {
	f.After = nil
	query, args, err := psql.Select("count(*)").
		From("audit_events").
		Where(buildAuditEventWhere(f)).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build audit event count query: %w", err)
	}
	var n int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("failed to count audit events: %w", err)
	}
	return n, nil
}

// buildAuditEventWhere turns a filter into the AND of its set fields.
func buildAuditEventWhere(f models.AuditEventFilter) squirrel.And {
	where := squirrel.And{}

	for _, eq := range []struct {
		col string
		val *string
	}{
		{"actor_user_id", f.ActorUserID},
		{"team_id", f.TeamID},
		{"project_id", f.ProjectID},
		{"resource_type", f.ResourceType},
	} {
		if eq.val != nil {
			where = append(where, squirrel.Eq{eq.col: *eq.val})
		}
	}

	if len(f.Operations) > 0 {
		where = append(where, squirrel.Eq{"operation": auditStrings(f.Operations)})
	}
	if len(f.Sources) > 0 {
		where = append(where, squirrel.Eq{"source": auditStrings(f.Sources)})
	}
	if len(f.Outcomes) > 0 {
		where = append(where, squirrel.Eq{"outcome": auditStrings(f.Outcomes)})
	}

	if f.From != nil {
		where = append(where, squirrel.GtOrEq{"created_at": *f.From})
	}
	if f.To != nil {
		where = append(where, squirrel.Lt{"created_at": *f.To})
	}

	if f.Query != nil && *f.Query != "" {
		term := "%" + escapeLikePattern(*f.Query) + "%"
		where = append(where, squirrel.Expr(
			`(resource_name ILIKE ? ESCAPE '\' OR actor_email ILIKE ? ESCAPE '\' OR ip ILIKE ? ESCAPE '\')`,
			term, term, term,
		))
	}

	if f.After != nil {
		where = append(where, squirrel.Expr(
			"(created_at, id) < (?::timestamptz, ?::uuid)", f.After.CreatedAt, f.After.ID,
		))
	}

	return where
}

// auditStrings converts a slice of one of the audit string enums to []string,
// which is what lib/pq binds for an IN list.
func auditStrings[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}

// likePatternEscaper escapes the LIKE metacharacters of a user-supplied term so
// it matches literally under ESCAPE '\'. The backslash goes first, or the
// escapes added for % and _ would themselves be escaped.
var likePatternEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// escapeLikePattern returns s with its LIKE wildcards escaped.
func escapeLikePattern(s string) string {
	return likePatternEscaper.Replace(s)
}

// scanAuditEventDest returns the scan destinations for auditEventColumns.
func scanAuditEventDest(e *models.AuditEvent) []any {
	return []any{
		&e.ID, &e.CreatedAt, &e.ActorUserID, &e.ActorEmail, &e.ActorName, &e.ActorType,
		&e.TeamID, &e.TeamName, &e.ProjectID, &e.ProjectName, &e.Operation,
		&e.ResourceType, &e.ResourceID, &e.ResourceName, &e.Source, &e.AssistantName,
		&e.APIKeyID, &e.APIKeyName, &e.Outcome, &e.IP, &e.UserAgent, &e.RequestID, (*[]byte)(&e.Metadata),
	}
}

// PurgeOlderThan deletes every event older than cutoff, batchSize rows per
// statement, and returns the total deleted. Small batches keep each DELETE's
// lock and WAL footprint bounded on a large backlog; it stops at the first
// batch that deletes fewer rows than batchSize.
func (r *AuditEventRepository) PurgeOlderThan(
	ctx context.Context, cutoff time.Time, batchSize int,
) (int64, error) {
	if batchSize <= 0 {
		return 0, fmt.Errorf("audit event purge batch size must be positive, got %d", batchSize)
	}

	var total int64
	for {
		res, err := r.db.ExecContext(ctx, purgeAuditEventsBatchQuery, cutoff, batchSize)
		if err != nil {
			return total, fmt.Errorf("failed to purge audit events: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("failed to read purged audit event count: %w", err)
		}
		total += n
		if n < int64(batchSize) {
			return total, nil
		}
	}
}
