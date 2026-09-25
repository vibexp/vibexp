package services

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// readExportCSV runs an export's WriteCSV and parses the result back with a
// strict reader (every record must have the header's field count).
func readExportCSV(t *testing.T, exp AdminExport) (records [][]string, rows int, raw string) {
	t.Helper()
	var buf bytes.Buffer
	rows, err := exp.WriteCSV(context.Background(), &buf)
	require.NoError(t, err)
	records, err = csv.NewReader(strings.NewReader(buf.String())).ReadAll()
	require.NoError(t, err)
	return records, rows, buf.String()
}

func adminExportUser(id, email, name string) models.AdminUserListItem {
	return models.AdminUserListItem{
		ID: id, Email: email, Name: name, Status: "active",
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("CEST", 2*3600)),
	}
}

// mockStreamUsers wires CountUsers/StreamUsers on repo to yield users.
func mockStreamUsers(repo *repomocks.MockAdminRepository, total int, users []models.AdminUserListItem) {
	repo.On("CountUsers", mock.Anything, mock.Anything).Return(total, nil)
	repo.On("StreamUsers", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(func(_ context.Context, _ repositories.AdminUserFilters, limit int,
			fn func(models.AdminUserListItem) error,
		) error {
			for i, u := range users {
				if i == limit {
					break
				}
				if err := fn(u); err != nil {
					return err
				}
			}
			return nil
		})
}

func TestAdminService_ExportUsers_HeaderColumnsAndValues(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	idp := "google"
	last := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	u := adminExportUser("u-1", "ada@example.com", "Ada")
	u.IDPProvider = &idp
	u.TeamCount, u.ProjectCount = 2, 3
	u.ResourceCounts = models.AdminResourceCounts{
		Prompts: 4, Memories: 5, Artifacts: 6, Blueprints: 7, Agents: 8,
		Feeds: 9, FeedItems: 10, Comments: 11, Attachments: 12, Total: 72,
	}
	u.LastResourceCreatedAt = &last
	filters := repositories.AdminUserFilters{SortBy: "email"}
	repo.On("CountUsers", mock.Anything, filters).Return(1, nil)
	repo.On("StreamUsers", mock.Anything, filters, adminExportRowCap, mock.Anything).
		Return(func(_ context.Context, _ repositories.AdminUserFilters, _ int,
			fn func(models.AdminUserListItem) error,
		) error {
			return fn(u)
		})

	exp, err := (&AdminService{adminRepo: repo}).ExportUsers(context.Background(), filters)
	require.NoError(t, err)
	assert.Equal(t, 1, exp.TotalCount)
	assert.False(t, exp.Truncated)

	records, rows, raw := readExportCSV(t, exp)
	assert.Equal(t, 1, rows)
	assert.Contains(t, raw, "\r\n", "RFC 4180 line endings")
	assert.Equal(t, []string{
		"id", "email", "name", "idp_provider", "status", "created_at", "team_count", "project_count",
		"prompt_count", "memory_count", "artifact_count", "blueprint_count", "agent_count", "feed_count",
		"feed_item_count", "comment_count", "attachment_count", "total_resource_count",
		"last_resource_created_at",
	}, records[0])
	assert.Equal(t, []string{
		"u-1", "ada@example.com", "Ada", "google", "active", "2026-01-02T01:04:05Z", "2", "3",
		"4", "5", "6", "7", "8", "9", "10", "11", "12", "72", "2026-09-01T12:00:00Z",
	}, records[1])
}

func TestAdminService_ExportUsers_NilOptionalCellsAreEmpty(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	mockStreamUsers(repo, 1, []models.AdminUserListItem{adminExportUser("u-1", "a@example.com", "A")})

	exp, err := (&AdminService{adminRepo: repo}).ExportUsers(context.Background(), repositories.AdminUserFilters{})
	require.NoError(t, err)
	records, _, _ := readExportCSV(t, exp)
	assert.Empty(t, records[1][3], "nil idp_provider")
	assert.Empty(t, records[1][18], "nil last_resource_created_at")
}

func TestAdminService_ExportTeams_HeaderColumnsAndValues(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	team := models.AdminTeamListItem{
		ID: "t-1", Name: "Acme", Slug: "acme", IsPersonal: true,
		Owner:       models.AdminTeamOwner{ID: "o-1", Email: "owner@example.com", Name: "Owner"},
		MemberCount: 1, OwnerCount: 2, AdminCount: 3, ProjectCount: 4,
		ResourceCounts: models.AdminResourceCounts{
			Prompts: 5, Memories: 6, Artifacts: 7, Blueprints: 8, Agents: 9,
			Feeds: 10, FeedItems: 11, Comments: 12, Attachments: 13, Total: 81,
		},
		Configuration: models.AdminTeamConfiguration{
			EmbeddingConfigured: true, AISummaryEnabled: true, GitHubConfigured: true, FreshnessEnabled: true,
		},
		CreatedAt: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC),
	}
	repo.On("CountTeams", mock.Anything, repositories.AdminTeamFilters{}).Return(1, nil)
	repo.On("StreamTeams", mock.Anything, repositories.AdminTeamFilters{}, adminExportRowCap, mock.Anything).
		Return(func(_ context.Context, _ repositories.AdminTeamFilters, _ int,
			fn func(models.AdminTeamListItem) error,
		) error {
			return fn(team)
		})

	exp, err := (&AdminService{adminRepo: repo}).ExportTeams(context.Background(), repositories.AdminTeamFilters{})
	require.NoError(t, err)
	records, rows, _ := readExportCSV(t, exp)
	assert.Equal(t, 1, rows)
	assert.Equal(t, []string{
		"id", "name", "slug", "is_personal", "owner_id", "owner_email", "owner_name",
		"member_count", "owner_count", "admin_count", "project_count",
		"prompt_count", "memory_count", "artifact_count", "blueprint_count", "agent_count", "feed_count",
		"feed_item_count", "comment_count", "attachment_count", "total_resource_count",
		"embedding_configured", "llm_configured", "ai_summary_enabled", "email_configured",
		"github_configured", "search_settings_customized", "freshness_enabled", "created_at",
	}, records[0])
	assert.Equal(t, []string{
		"t-1", "Acme", "acme", "true", "o-1", "owner@example.com", "Owner",
		"1", "2", "3", "4",
		"5", "6", "7", "8", "9", "10", "11", "12", "13", "81",
		"true", "false", "true", "false", "true", "false", "true", "2026-03-04T05:06:07Z",
	}, records[1])
}

func TestAdminService_ExportProjects_HeaderColumnsAndValues(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	project := models.AdminProjectListItem{
		ID: "p-1", Name: "Platform", Slug: "platform",
		Team:  models.AdminProjectTeam{ID: "t-1", Name: "Acme", Slug: "acme"},
		Owner: models.AdminTeamOwner{ID: "o-1", Email: "owner@example.com", Name: "Owner"},
		ResourceCounts: models.AdminProjectResourceCounts{
			Prompts: 1, Memories: 2, Artifacts: 3, Blueprints: 4, FeedItems: 5, Total: 15,
		},
		CreatedAt: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC),
	}
	repo.On("CountProjects", mock.Anything, repositories.AdminProjectFilters{}).Return(1, nil)
	repo.On("StreamProjects", mock.Anything, repositories.AdminProjectFilters{}, adminExportRowCap, mock.Anything).
		Return(func(_ context.Context, _ repositories.AdminProjectFilters, _ int,
			fn func(models.AdminProjectListItem) error,
		) error {
			return fn(project)
		})

	exp, err := (&AdminService{adminRepo: repo}).ExportProjects(
		context.Background(), repositories.AdminProjectFilters{})
	require.NoError(t, err)
	records, rows, _ := readExportCSV(t, exp)
	assert.Equal(t, 1, rows)
	assert.Equal(t, []string{
		"id", "name", "slug", "team_id", "team_name", "team_slug", "owner_id", "owner_email", "owner_name",
		"prompt_count", "memory_count", "artifact_count", "blueprint_count", "feed_item_count",
		"total_resource_count", "last_resource_created_at", "created_at", "updated_at",
	}, records[0])
	assert.Equal(t, []string{
		"p-1", "Platform", "platform", "t-1", "Acme", "acme", "o-1", "owner@example.com", "Owner",
		"1", "2", "3", "4", "5", "15", "", "2026-03-04T05:06:07Z", "2026-04-05T06:07:08Z",
	}, records[1])
}

// TestAdminService_Export_FormulaGuard pins the CSV-injection guard: every
// trigger character is defused on free-text cells, and counts are untouched.
func TestAdminService_Export_FormulaGuard(t *testing.T) {
	for _, trigger := range []string{"=", "+", "-", "@", "\t", "\r"} {
		t.Run(strings.ReplaceAll(strings.ReplaceAll(trigger, "\t", `\t`), "\r", `\r`), func(t *testing.T) {
			repo := repomocks.NewMockAdminRepository(t)
			idp := trigger + "idp"
			u := adminExportUser("u-1", trigger+"mail@example.com", trigger+`HYPERLINK("http://x")`)
			u.IDPProvider = &idp
			mockStreamUsers(repo, 1, []models.AdminUserListItem{u})

			exp, err := (&AdminService{adminRepo: repo}).ExportUsers(
				context.Background(), repositories.AdminUserFilters{})
			require.NoError(t, err)
			records, _, _ := readExportCSV(t, exp)
			// encoding/csv with UseCRLF drops a bare \r inside a field, so the
			// quote prefix is what remains in front of that trigger.
			want := "'" + strings.TrimPrefix(trigger, "\r")
			assert.Equal(t, want+"mail@example.com", records[1][1])
			assert.Equal(t, want+`HYPERLINK("http://x")`, records[1][2])
			assert.Equal(t, want+"idp", records[1][3])
		})
	}

	t.Run("project and team free text", func(t *testing.T) {
		repo := repomocks.NewMockAdminRepository(t)
		p := models.AdminProjectListItem{
			ID: "p-1", Name: "=cmd", Slug: "-slug",
			Team:  models.AdminProjectTeam{ID: "t-1", Name: "+team", Slug: "@team"},
			Owner: models.AdminTeamOwner{ID: "o-1", Email: "=o@example.com", Name: "-owner"},
		}
		repo.On("CountProjects", mock.Anything, mock.Anything).Return(1, nil)
		repo.On("StreamProjects", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(func(_ context.Context, _ repositories.AdminProjectFilters, _ int,
				fn func(models.AdminProjectListItem) error,
			) error {
				return fn(p)
			})
		exp, err := (&AdminService{adminRepo: repo}).ExportProjects(
			context.Background(), repositories.AdminProjectFilters{})
		require.NoError(t, err)
		records, _, _ := readExportCSV(t, exp)
		assert.Equal(t, []string{"p-1", "'=cmd", "'-slug", "t-1", "'+team", "'@team", "o-1", "'=o@example.com", "'-owner"},
			records[1][:9])
		assert.Equal(t, "0", records[1][9], "counts are never prefixed")
	})
}

func TestCSVSafe(t *testing.T) {
	assert.Empty(t, csvSafe(""))
	assert.Equal(t, "plain", csvSafe("plain"))
	assert.Equal(t, "a=b", csvSafe("a=b"), "only a LEADING trigger is defused")
	assert.Equal(t, "'-1", csvSafe("-1"))
}

// TestAdminService_Export_QuotingRoundTrips proves embedded quotes, commas and
// newlines survive encoding/csv's quoting.
func TestAdminService_Export_QuotingRoundTrips(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	name := "Ada \"The Countess\", of\nLovelace"
	mockStreamUsers(repo, 1, []models.AdminUserListItem{adminExportUser("u-1", "ada@example.com", name)})

	exp, err := (&AdminService{adminRepo: repo}).ExportUsers(context.Background(), repositories.AdminUserFilters{})
	require.NoError(t, err)
	records, _, _ := readExportCSV(t, exp)
	assert.Equal(t, name, records[1][2])
}

func TestAdminService_Export_Cap(t *testing.T) {
	users := []models.AdminUserListItem{
		adminExportUser("u-1", "a@example.com", "A"),
		adminExportUser("u-2", "b@example.com", "B"),
		adminExportUser("u-3", "c@example.com", "C"),
		adminExportUser("u-4", "d@example.com", "D"),
	}

	t.Run("total above cap appends the marker row", func(t *testing.T) {
		repo := repomocks.NewMockAdminRepository(t)
		mockStreamUsers(repo, 4, users)

		exp, err := (&AdminService{adminRepo: repo, exportCap: 3}).ExportUsers(
			context.Background(), repositories.AdminUserFilters{})
		require.NoError(t, err)
		assert.True(t, exp.Truncated)
		assert.Equal(t, 4, exp.TotalCount)

		records, rows, _ := readExportCSV(t, exp)
		assert.Equal(t, 3, rows)
		require.Len(t, records, 1+3+1)
		marker := records[4]
		assert.Equal(t, "# TRUNCATED: exported 3 of 4 rows; narrow the filters", marker[0])
		for _, cell := range marker[1:] {
			assert.Empty(t, cell)
		}
	})

	t.Run("total equal to cap has no marker", func(t *testing.T) {
		repo := repomocks.NewMockAdminRepository(t)
		mockStreamUsers(repo, 3, users[:3])

		exp, err := (&AdminService{adminRepo: repo, exportCap: 3}).ExportUsers(
			context.Background(), repositories.AdminUserFilters{})
		require.NoError(t, err)
		assert.False(t, exp.Truncated)

		records, rows, _ := readExportCSV(t, exp)
		assert.Equal(t, 3, rows)
		assert.Len(t, records, 1+3)
	})

	t.Run("rows deleted after the count get no marker", func(t *testing.T) {
		repo := repomocks.NewMockAdminRepository(t)
		mockStreamUsers(repo, 4, users[:2])

		exp, err := (&AdminService{adminRepo: repo, exportCap: 3}).ExportUsers(
			context.Background(), repositories.AdminUserFilters{})
		require.NoError(t, err)
		records, rows, _ := readExportCSV(t, exp)
		assert.Equal(t, 2, rows)
		assert.Len(t, records, 1+2)
	})

	t.Run("default cap", func(t *testing.T) {
		assert.Equal(t, 50000, (&AdminService{}).exportRowCap())
	})
}

func TestAdminService_Export_Errors(t *testing.T) {
	t.Run("count error", func(t *testing.T) {
		repo := repomocks.NewMockAdminRepository(t)
		repo.On("CountTeams", mock.Anything, mock.Anything).Return(0, errors.New("db down"))
		_, err := (&AdminService{adminRepo: repo}).ExportTeams(context.Background(), repositories.AdminTeamFilters{})
		require.Error(t, err)
	})

	t.Run("stream error propagates", func(t *testing.T) {
		repo := repomocks.NewMockAdminRepository(t)
		repo.On("CountProjects", mock.Anything, mock.Anything).Return(5, nil)
		repo.On("StreamProjects", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(errors.New("connection reset"))
		exp, err := (&AdminService{adminRepo: repo}).ExportProjects(
			context.Background(), repositories.AdminProjectFilters{})
		require.NoError(t, err)
		_, err = exp.WriteCSV(context.Background(), &bytes.Buffer{})
		require.EqualError(t, err, "connection reset")
	})

	t.Run("writer error stops the stream", func(t *testing.T) {
		repo := repomocks.NewMockAdminRepository(t)
		mockStreamUsers(repo, 1, []models.AdminUserListItem{adminExportUser("u-1", "a@example.com", "A")})
		exp, err := (&AdminService{adminRepo: repo}).ExportUsers(context.Background(), repositories.AdminUserFilters{})
		require.NoError(t, err)
		_, err = exp.WriteCSV(context.Background(), failingWriter{})
		require.Error(t, err)
	})
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed pipe") }
