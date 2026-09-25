//go:build integration

package postgres

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Integration coverage for the #1136 per-user access queries. Everything is
// scoped by the seeded user's id, so no table truncation is needed.

// accessDay is the day most fixture accesses happen on.
var accessDay = time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)

// insertAccessEvent records one resource access at an explicit time.
func insertAccessEvent(t *testing.T, teamID, userID, resourceType, resourceID, source string, at time.Time) {
	t.Helper()
	adminListExec(t, "INSERT INTO resource_access_events "+
		"(team_id, user_id, resource_type, resource_id, source, created_at) VALUES ($1, $2, $3, $4, $5, $6)",
		teamID, userID, resourceType, resourceID, source, at)
}

// adminAccessFixture is the seeded population.
type adminAccessFixture struct {
	user, teamA, teamB, projectA, projectB string
	prompt, memory, agent, deleted         string
}

// seedAdminAccessFixture seeds, for one user across two teams (one of which
// the user is NOT a member of):
//
//	prompt (team A)  4 accesses: 3 web on accessDay, 1 mcp the next day
//	memory (team B)  2 cli accesses
//	project B        2 web accesses (ties with the memory)
//	agent (team A)   1 api access
//	deleted artifact 1 web access (resource_id joins nothing)
//
// plus a teammate's 5 accesses to the prompt and one of the user's accesses
// just outside the window, both of which must be excluded.
func seedAdminAccessFixture(t *testing.T) adminAccessFixture {
	t.Helper()
	f := adminAccessFixture{
		user: insertAdminListUser(t, "acc"+uuid.New().String()[:8], "user"),
	}
	teammate := insertAdminListUser(t, "acc"+uuid.New().String()[:8], "teammate")
	f.teamA = insertTestTeam(t, f.user)
	f.teamB = insertTestTeam(t, teammate)
	f.projectA = insertTestProject(t, f.user, f.teamA)
	f.projectB = insertTestProject(t, teammate, f.teamB)

	f.prompt = insertTestPrompt(t, f.user, f.teamA, f.projectA, "secret title", "body", "published")
	f.memory = uuid.New().String()
	adminListExec(t, "INSERT INTO memories (id, user_id, team_id, project_id, text) VALUES ($1, $2, $3, $4, 'm')",
		f.memory, teammate, f.teamB, f.projectB)
	f.agent = insertTestAgent(t, f.user, f.teamA)
	f.deleted = uuid.New().String()

	for i := 0; i < 3; i++ {
		insertAccessEvent(t, f.teamA, f.user, "prompt", f.prompt, "web", accessDay)
	}
	insertAccessEvent(t, f.teamA, f.user, "prompt", f.prompt, "mcp", accessDay.AddDate(0, 0, 1))
	insertAccessEvent(t, f.teamB, f.user, "memory", f.memory, "cli", accessDay)
	insertAccessEvent(t, f.teamB, f.user, "memory", f.memory, "cli", accessDay)
	insertAccessEvent(t, f.teamB, f.user, "project", f.projectB, "web", accessDay)
	insertAccessEvent(t, f.teamB, f.user, "project", f.projectB, "web", accessDay)
	insertAccessEvent(t, f.teamA, f.user, "agent", f.agent, "api", accessDay)
	insertAccessEvent(t, f.teamA, f.user, "artifact", f.deleted, "web", accessDay)
	for i := 0; i < 5; i++ {
		insertAccessEvent(t, f.teamA, teammate, "prompt", f.prompt, "web", accessDay)
	}
	insertAccessEvent(t, f.teamA, f.user, "prompt", f.prompt, "web", accessDay.AddDate(0, 0, -10))
	return f
}

func TestAdminUserAccess_BySourceSeries(t *testing.T) {
	f := seedAdminAccessFixture(t)
	repo := NewAdminRepository(integrationDB)
	day := time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC)
	next := day.AddDate(0, 0, 1)

	got, err := repo.GetUserAccessBySourceSeries(context.Background(), f.user, day, day.AddDate(0, 0, 2), "day")
	require.NoError(t, err)
	assert.Equal(t, []models.AdminSourcePoint{
		{Bucket: day, Source: "api", Count: 1},
		{Bucket: day, Source: "cli", Count: 2},
		{Bucket: day, Source: "web", Count: 6},
		{Bucket: next, Source: "mcp", Count: 1},
	}, normalizeSourcePoints(got), "the teammate's accesses and the out-of-window one are excluded")

	// The window end is exclusive: stopping at `next` drops the mcp access.
	got, err = repo.GetUserAccessBySourceSeries(context.Background(), f.user, day, next, "week")
	require.NoError(t, err)
	week := time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC) // Monday
	for _, p := range normalizeSourcePoints(got) {
		assert.Equal(t, week, p.Bucket)
		assert.NotEqual(t, "mcp", p.Source)
	}
}

// normalizeSourcePoints puts buckets in UTC so assertions compare instants.
func normalizeSourcePoints(points []models.AdminSourcePoint) []models.AdminSourcePoint {
	out := make([]models.AdminSourcePoint, len(points))
	for i, p := range points {
		out[i] = models.AdminSourcePoint{Bucket: p.Bucket.UTC(), Source: p.Source, Count: p.Count}
	}
	return out
}

func TestAdminUserAccess_TopAccessedResources(t *testing.T) {
	f := seedAdminAccessFixture(t)
	repo := NewAdminRepository(integrationDB)
	from, to := accessDay.AddDate(0, 0, -1), accessDay.AddDate(0, 0, 2)

	got, err := repo.GetUserTopAccessedResources(context.Background(), f.user, from, to, 10)
	require.NoError(t, err)
	require.Len(t, got, 5)

	// Ranked by count, ties (memory vs project, agent vs artifact) broken by id.
	assert.Equal(t, f.prompt, got[0].ResourceID)
	assert.Equal(t, int64(4), got[0].AccessCount, "the teammate's accesses are not counted")
	assert.Equal(t, "prompt", got[0].ResourceType)
	assert.Equal(t, f.teamA, got[0].TeamID)
	require.NotNil(t, got[0].ProjectID)
	assert.Equal(t, f.projectA, *got[0].ProjectID)
	assert.NotNil(t, got[0].ProjectName)
	assert.False(t, got[0].ResourceDeleted)

	assertTieOrder := func(a, b models.AdminTopAccessedResource) {
		t.Helper()
		assert.Equal(t, a.AccessCount, b.AccessCount)
		assert.Less(t, a.ResourceID, b.ResourceID, "ties are broken by resource id")
	}
	assertTieOrder(got[1], got[2])
	assertTieOrder(got[3], got[4])

	byID := make(map[string]models.AdminTopAccessedResource, len(got))
	for _, r := range got {
		byID[r.ResourceID] = r
	}

	memory := byID[f.memory]
	assert.Equal(t, int64(2), memory.AccessCount)
	assert.Equal(t, f.teamB, memory.TeamID, "a team the user is not a member of still resolves")
	require.NotNil(t, memory.ProjectID)
	assert.Equal(t, f.projectB, *memory.ProjectID)

	project := byID[f.projectB]
	assert.Equal(t, "project", project.ResourceType)
	require.NotNil(t, project.ProjectID, "a project row resolves to the project itself")
	assert.Equal(t, f.projectB, *project.ProjectID)
	assert.False(t, project.ResourceDeleted)

	agent := byID[f.agent]
	assert.Nil(t, agent.ProjectID, "an agent has no project")
	assert.False(t, agent.ResourceDeleted)

	deleted := byID[f.deleted]
	assert.True(t, deleted.ResourceDeleted)
	assert.Nil(t, deleted.ProjectID)
	assert.Nil(t, deleted.ProjectName)

	// The limit applies to the ranked rows.
	got, err = repo.GetUserTopAccessedResources(context.Background(), f.user, from, to, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, f.prompt, got[0].ResourceID)

	// The window is honoured: only the out-of-window access remains before it.
	got, err = repo.GetUserTopAccessedResources(context.Background(), f.user,
		accessDay.AddDate(0, 0, -11), accessDay.AddDate(0, 0, -1), 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, int64(1), got[0].AccessCount)
}

// TestAdminUserAccess_RowCarriesNoContent pins the opaque DTO: the scanned
// struct has exactly these fields, none of which can hold a title, slug or body.
func TestAdminUserAccess_RowCarriesNoContent(t *testing.T) {
	typ := reflect.TypeOf(models.AdminTopAccessedResource{})
	fields := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		fields = append(fields, typ.Field(i).Name)
	}
	assert.Equal(t, []string{
		"ResourceType", "ResourceID", "TeamID", "TeamName",
		"ProjectID", "ProjectName", "ResourceDeleted", "AccessCount",
	}, fields)
}
