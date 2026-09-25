package services

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// adminExportRowCap bounds one admin CSV export (#1149). The filtered set's
// total is counted first, so a truncated export is signalled up front (response
// headers) and by a trailing marker row.
const adminExportRowCap = 50000

// adminCSVFormulaTriggers are the leading characters a spreadsheet may evaluate
// as a formula (OWASP "CSV injection").
const adminCSVFormulaTriggers = "=+-@\t\r"

// AdminExport is a prepared admin CSV export: the filtered set has been
// counted, and WriteCSV streams it.
type AdminExport struct {
	// TotalCount is the size of the filtered set before the cap.
	TotalCount int
	// Truncated reports TotalCount > the row cap.
	Truncated bool
	// WriteCSV writes the header row, every row up to the cap and, when
	// truncated, the marker row, returning the number of data rows written. It
	// does not buffer the set: rows are written as the repository yields them.
	WriteCSV func(ctx context.Context, w io.Writer) (rows int, err error)
}

// adminCSVColumn is one export column: its header and how a row renders it.
// Declaring both together keeps headers and values from misaligning.
type adminCSVColumn[T any] struct {
	header string
	value  func(T) string
}

// adminCSVText is a free-text column: its cells go through csvSafe.
func adminCSVText[T any](header string, value func(T) string) adminCSVColumn[T] {
	return adminCSVColumn[T]{header: header, value: func(item T) string { return csvSafe(value(item)) }}
}

// adminCSVInt is a count column; counts are server-computed and never guarded.
func adminCSVInt[T any](header string, value func(T) int64) adminCSVColumn[T] {
	return adminCSVColumn[T]{header: header, value: func(item T) string { return strconv.FormatInt(value(item), 10) }}
}

// adminCSVBool is a boolean column rendered as true/false.
func adminCSVBool[T any](header string, value func(T) bool) adminCSVColumn[T] {
	return adminCSVColumn[T]{header: header, value: func(item T) string { return strconv.FormatBool(value(item)) }}
}

// adminCSVTime is a timestamp column rendered as RFC 3339 UTC; nil is empty.
func adminCSVTime[T any](header string, value func(T) *time.Time) adminCSVColumn[T] {
	return adminCSVColumn[T]{header: header, value: func(item T) string { return formatCSVTime(value(item)) }}
}

// adminCSVID is a server-generated identifier or enum column, written verbatim.
func adminCSVID[T any](header string, value func(T) string) adminCSVColumn[T] {
	return adminCSVColumn[T]{header: header, value: value}
}

// csvSafe defuses spreadsheet formula injection: a cell starting with one of
// adminCSVFormulaTriggers is prefixed with a single quote so it is shown as text.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune(adminCSVFormulaTriggers, rune(s[0])) {
		return "'" + s
	}
	return s
}

// formatCSVTime renders t as RFC 3339 in UTC, or "" for nil.
func formatCSVTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// adminResourceCountColumns are the per-type authored-resource count columns
// shared by the user and team exports.
func adminResourceCountColumns[T any](counts func(T) models.AdminResourceCounts) []adminCSVColumn[T] {
	return []adminCSVColumn[T]{
		adminCSVInt("prompt_count", func(t T) int64 { return counts(t).Prompts }),
		adminCSVInt("memory_count", func(t T) int64 { return counts(t).Memories }),
		adminCSVInt("artifact_count", func(t T) int64 { return counts(t).Artifacts }),
		adminCSVInt("blueprint_count", func(t T) int64 { return counts(t).Blueprints }),
		adminCSVInt("agent_count", func(t T) int64 { return counts(t).Agents }),
		adminCSVInt("feed_count", func(t T) int64 { return counts(t).Feeds }),
		adminCSVInt("feed_item_count", func(t T) int64 { return counts(t).FeedItems }),
		adminCSVInt("comment_count", func(t T) int64 { return counts(t).Comments }),
		adminCSVInt("attachment_count", func(t T) int64 { return counts(t).Attachments }),
		adminCSVInt("total_resource_count", func(t T) int64 { return counts(t).Total }),
	}
}

// adminUserExportColumns mirrors the admin user list columns.
func adminUserExportColumns() []adminCSVColumn[models.AdminUserListItem] {
	type u = models.AdminUserListItem
	cols := []adminCSVColumn[u]{
		adminCSVID("id", func(x u) string { return x.ID }),
		adminCSVText("email", func(x u) string { return x.Email }),
		adminCSVText("name", func(x u) string { return x.Name }),
		adminCSVText("idp_provider", func(x u) string {
			if x.IDPProvider == nil {
				return ""
			}
			return *x.IDPProvider
		}),
		adminCSVID("status", func(x u) string { return x.Status }),
		adminCSVTime("created_at", func(x u) *time.Time { return &x.CreatedAt }),
		adminCSVInt("team_count", func(x u) int64 { return x.TeamCount }),
		adminCSVInt("project_count", func(x u) int64 { return x.ProjectCount }),
	}
	cols = append(cols, adminResourceCountColumns(func(x u) models.AdminResourceCounts { return x.ResourceCounts })...)
	return append(cols, adminCSVTime("last_resource_created_at", func(x u) *time.Time { return x.LastResourceCreatedAt }))
}

// adminTeamExportColumns mirrors the admin team list columns, flattening the
// owner and the configuration flags.
func adminTeamExportColumns() []adminCSVColumn[models.AdminTeamListItem] {
	type t = models.AdminTeamListItem
	cols := []adminCSVColumn[t]{
		adminCSVID("id", func(x t) string { return x.ID }),
		adminCSVText("name", func(x t) string { return x.Name }),
		adminCSVText("slug", func(x t) string { return x.Slug }),
		adminCSVBool("is_personal", func(x t) bool { return x.IsPersonal }),
		adminCSVID("owner_id", func(x t) string { return x.Owner.ID }),
		adminCSVText("owner_email", func(x t) string { return x.Owner.Email }),
		adminCSVText("owner_name", func(x t) string { return x.Owner.Name }),
		adminCSVInt("member_count", func(x t) int64 { return x.MemberCount }),
		adminCSVInt("owner_count", func(x t) int64 { return x.OwnerCount }),
		adminCSVInt("admin_count", func(x t) int64 { return x.AdminCount }),
		adminCSVInt("project_count", func(x t) int64 { return x.ProjectCount }),
	}
	cols = append(cols, adminResourceCountColumns(func(x t) models.AdminResourceCounts { return x.ResourceCounts })...)
	return append(cols,
		adminCSVBool("embedding_configured", func(x t) bool { return x.Configuration.EmbeddingConfigured }),
		adminCSVBool("llm_configured", func(x t) bool { return x.Configuration.LLMConfigured }),
		adminCSVBool("ai_summary_enabled", func(x t) bool { return x.Configuration.AISummaryEnabled }),
		adminCSVBool("email_configured", func(x t) bool { return x.Configuration.EmailConfigured }),
		adminCSVBool("github_configured", func(x t) bool { return x.Configuration.GitHubConfigured }),
		adminCSVBool("search_settings_customized", func(x t) bool { return x.Configuration.SearchSettingsCustomized }),
		adminCSVBool("freshness_enabled", func(x t) bool { return x.Configuration.FreshnessEnabled }),
		adminCSVTime("created_at", func(x t) *time.Time { return &x.CreatedAt }),
	)
}

// adminProjectExportColumns mirrors the admin project list columns, flattening
// the team and the creator.
func adminProjectExportColumns() []adminCSVColumn[models.AdminProjectListItem] {
	type p = models.AdminProjectListItem
	return []adminCSVColumn[p]{
		adminCSVID("id", func(x p) string { return x.ID }),
		adminCSVText("name", func(x p) string { return x.Name }),
		adminCSVText("slug", func(x p) string { return x.Slug }),
		adminCSVID("team_id", func(x p) string { return x.Team.ID }),
		adminCSVText("team_name", func(x p) string { return x.Team.Name }),
		adminCSVText("team_slug", func(x p) string { return x.Team.Slug }),
		adminCSVID("owner_id", func(x p) string { return x.Owner.ID }),
		adminCSVText("owner_email", func(x p) string { return x.Owner.Email }),
		adminCSVText("owner_name", func(x p) string { return x.Owner.Name }),
		adminCSVInt("prompt_count", func(x p) int64 { return x.ResourceCounts.Prompts }),
		adminCSVInt("memory_count", func(x p) int64 { return x.ResourceCounts.Memories }),
		adminCSVInt("artifact_count", func(x p) int64 { return x.ResourceCounts.Artifacts }),
		adminCSVInt("blueprint_count", func(x p) int64 { return x.ResourceCounts.Blueprints }),
		adminCSVInt("feed_item_count", func(x p) int64 { return x.ResourceCounts.FeedItems }),
		adminCSVInt("total_resource_count", func(x p) int64 { return x.ResourceCounts.Total }),
		adminCSVTime("last_resource_created_at", func(x p) *time.Time { return x.LastResourceCreatedAt }),
		adminCSVTime("created_at", func(x p) *time.Time { return &x.CreatedAt }),
		adminCSVTime("updated_at", func(x p) *time.Time { return &x.UpdatedAt }),
	}
}

// exportRowCap is the effective row cap; tests shrink it through exportCap.
func (s *AdminService) exportRowCap() int {
	if s.exportCap > 0 {
		return s.exportCap
	}
	return adminExportRowCap
}

// newAdminExport builds the export for one listing: total is the filtered
// set's size, and stream yields its rows (up to limit) in the listing's order.
func newAdminExport[T any](
	total, rowCap int, cols []adminCSVColumn[T],
	stream func(ctx context.Context, limit int, fn func(T) error) error,
) AdminExport {
	truncated := total > rowCap
	return AdminExport{
		TotalCount: total,
		Truncated:  truncated,
		WriteCSV: func(ctx context.Context, w io.Writer) (int, error) {
			cw := csv.NewWriter(w)
			cw.UseCRLF = true

			record := make([]string, len(cols))
			for i, c := range cols {
				record[i] = c.header
			}
			if err := cw.Write(record); err != nil {
				return 0, err
			}

			rows := 0
			err := stream(ctx, rowCap, func(item T) error {
				for i, c := range cols {
					record[i] = c.value(item)
				}
				rows++
				return cw.Write(record)
			})
			if err != nil {
				return rows, err
			}

			// Rows inserted after the count can shift the streamed total; only a
			// stream that actually hit the cap gets the marker.
			if truncated && rows == rowCap {
				clear(record)
				record[0] = fmt.Sprintf("# TRUNCATED: exported %d of %d rows; narrow the filters", rows, total)
				if err := cw.Write(record); err != nil {
					return rows, err
				}
			}

			cw.Flush()
			return rows, cw.Error()
		},
	}
}

// ExportUsers counts the filtered user set and returns its CSV export.
func (s *AdminService) ExportUsers(
	ctx context.Context, filters repositories.AdminUserFilters,
) (AdminExport, error) {
	total, err := s.adminRepo.CountUsers(ctx, filters)
	if err != nil {
		return AdminExport{}, err
	}
	return newAdminExport(total, s.exportRowCap(), adminUserExportColumns(),
		func(ctx context.Context, limit int, fn func(models.AdminUserListItem) error) error {
			return s.adminRepo.StreamUsers(ctx, filters, limit, fn)
		}), nil
}

// ExportTeams counts the filtered team set and returns its CSV export.
func (s *AdminService) ExportTeams(
	ctx context.Context, filters repositories.AdminTeamFilters,
) (AdminExport, error) {
	total, err := s.adminRepo.CountTeams(ctx, filters)
	if err != nil {
		return AdminExport{}, err
	}
	return newAdminExport(total, s.exportRowCap(), adminTeamExportColumns(),
		func(ctx context.Context, limit int, fn func(models.AdminTeamListItem) error) error {
			return s.adminRepo.StreamTeams(ctx, filters, limit, fn)
		}), nil
}

// ExportProjects counts the filtered project set and returns its CSV export.
func (s *AdminService) ExportProjects(
	ctx context.Context, filters repositories.AdminProjectFilters,
) (AdminExport, error) {
	total, err := s.adminRepo.CountProjects(ctx, filters)
	if err != nil {
		return AdminExport{}, err
	}
	return newAdminExport(total, s.exportRowCap(), adminProjectExportColumns(),
		func(ctx context.Context, limit int, fn func(models.AdminProjectListItem) error) error {
			return s.adminRepo.StreamProjects(ctx, filters, limit, fn)
		}), nil
}
