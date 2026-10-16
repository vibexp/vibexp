package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// adminAuditExportPageSize is how many events one page of an audit export
// reads. The export follows the keyset cursor page by page instead of holding
// one long-running query open for the whole download.
const adminAuditExportPageSize = 500

// AdminAuditServiceInterface is the instance admin's read side of the audit
// log (#1516): the paged listing and the CSV export of the same filter.
// Instance-admin authorization is the route middleware's job.
type AdminAuditServiceInterface interface {
	// List returns one page of events matching f, newest first, and the
	// cursor of the next page (nil on the last one).
	List(ctx context.Context, f models.AuditEventFilter) ([]models.AuditEvent, *models.AuditCursor, error)
	// Export counts the events matching f and returns their CSV export,
	// newest first, capped like every admin export. f's Limit and After are
	// ignored.
	Export(ctx context.Context, f models.AuditEventFilter) (AdminExport, error)
}

// AdminAuditService implements AdminAuditServiceInterface.
type AdminAuditService struct {
	events repositories.AuditEventRepository
	// exportCap overrides adminExportRowCap when positive (tests).
	exportCap int
}

var _ AdminAuditServiceInterface = (*AdminAuditService)(nil)

// NewAdminAuditService creates a new AdminAuditService.
func NewAdminAuditService(events repositories.AuditEventRepository) *AdminAuditService {
	return &AdminAuditService{events: events}
}

// List implements AdminAuditServiceInterface.
func (s *AdminAuditService) List(
	ctx context.Context, f models.AuditEventFilter,
) ([]models.AuditEvent, *models.AuditCursor, error) {
	events, next, err := s.events.List(ctx, f)
	if err != nil {
		return nil, nil, fmt.Errorf("AdminAuditService.List: %w", err)
	}
	return events, next, nil
}

// Export implements AdminAuditServiceInterface.
func (s *AdminAuditService) Export(ctx context.Context, f models.AuditEventFilter) (AdminExport, error) {
	f.Limit, f.After = 0, nil
	total, err := s.events.Count(ctx, f)
	if err != nil {
		return AdminExport{}, fmt.Errorf("AdminAuditService.Export: %w", err)
	}
	rowCap := adminExportRowCap
	if s.exportCap > 0 {
		rowCap = s.exportCap
	}
	return newAdminExport(total, rowCap, adminAuditExportColumns(),
		func(ctx context.Context, limit int, fn func(models.AuditEvent) error) error {
			return s.streamAudit(ctx, f, limit, fn)
		}), nil
}

// streamAudit yields up to limit events matching f, newest first, reading
// them page by page along the keyset cursor.
func (s *AdminAuditService) streamAudit(
	ctx context.Context, f models.AuditEventFilter, limit int, fn func(models.AuditEvent) error,
) error {
	yielded := 0
	for yielded < limit {
		f.Limit = min(adminAuditExportPageSize, limit-yielded)
		events, next, err := s.events.List(ctx, f)
		if err != nil {
			return fmt.Errorf("list audit events for export: %w", err)
		}
		for _, e := range events {
			if err := fn(e); err != nil {
				return err
			}
			yielded++
		}
		if next == nil || len(events) == 0 {
			return nil
		}
		f.After = next
	}
	return nil
}

// adminAuditExportColumns are the audit export's columns, in the documented
// order. Every recorded string goes through the formula-injection guard,
// including the ids: audit_events stores them as plain text with no UUID
// constraint, so they are not trusted to be well-formed. Only the
// server-generated enums and the timestamp are written verbatim. The guard is
// a no-op on a real UUID, which starts with a hex digit.
func adminAuditExportColumns() []adminCSVColumn[models.AuditEvent] {
	type e = models.AuditEvent
	return []adminCSVColumn[e]{
		adminCSVTime("created_at", func(x e) *time.Time { return &x.CreatedAt }),
		adminCSVID("operation", func(x e) string { return string(x.Operation) }),
		adminCSVID("outcome", func(x e) string { return string(x.Outcome) }),
		adminCSVID("source", func(x e) string { return string(x.Source) }),
		adminCSVID("actor_type", func(x e) string { return string(x.ActorType) }),
		adminCSVText("actor_email", func(x e) string { return x.ActorEmail }),
		adminCSVText("actor_name", func(x e) string { return x.ActorName }),
		adminCSVText("actor_user_id", func(x e) string { return auditCell(x.ActorUserID) }),
		adminCSVText("team_name", func(x e) string { return auditCell(x.TeamName) }),
		adminCSVText("team_id", func(x e) string { return auditCell(x.TeamID) }),
		adminCSVText("project_name", func(x e) string { return auditCell(x.ProjectName) }),
		adminCSVText("project_id", func(x e) string { return auditCell(x.ProjectID) }),
		adminCSVText("resource_type", func(x e) string { return auditCell(x.ResourceType) }),
		adminCSVText("resource_name", func(x e) string { return auditCell(x.ResourceName) }),
		adminCSVText("resource_id", func(x e) string { return auditCell(x.ResourceID) }),
		adminCSVText("assistant_name", func(x e) string { return auditCell(x.AssistantName) }),
		adminCSVText("api_key_name", func(x e) string { return auditCell(x.APIKeyName) }),
		adminCSVText("ip", func(x e) string { return auditCell(x.IP) }),
		adminCSVText("user_agent", func(x e) string { return auditCell(x.UserAgent) }),
		adminCSVText("request_id", func(x e) string { return auditCell(x.RequestID) }),
		adminCSVText("metadata", func(x e) string { return compactAuditMetadata(x.Metadata) }),
	}
}

// auditCell renders an optional value verbatim, or "" for nil. Unlike
// optionalValue it does not trim: an audit export reproduces what was
// recorded, and the formula guard must see the value's real first character.
func auditCell(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// compactAuditMetadata renders the metadata object as compact JSON: "{}" when
// there is none, and the stored text as-is in the unlikely case it does not
// parse.
func compactAuditMetadata(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "{}"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}
