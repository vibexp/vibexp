//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// The write-event log and its two aggregates against real Postgres (#1392).
//
// The property the creation metrics promise is arithmetic, and only a real
// database can check it: for each resource type, the per-channel breakdown must
// sum to the total the per-day series reports for the same window. Both
// queries are run here over one fixture and compared, including under a
// non-UTC session, where the naive memories.created_at column is the branch
// that breaks first.

// writeEventFixture is one team with two projects and a second, unrelated team.
type writeEventFixture struct {
	userID       string
	teamID       string
	projectID    string
	projectSlug  string
	siblingID    string
	siblingSlug  string
	otherTeamID  string
	otherProject string
}

func newWriteEventFixture(t *testing.T) writeEventFixture {
	t.Helper()
	resetIntegrationTables(t)

	f := writeEventFixture{userID: insertTestUser(t)}
	f.teamID = insertTestTeam(t, f.userID)
	f.projectID = insertTestProject(t, f.userID, f.teamID)
	f.siblingID = insertTestProject(t, f.userID, f.teamID)
	f.otherTeamID = insertTestTeam(t, f.userID)
	f.otherProject = insertTestProject(t, f.userID, f.otherTeamID)

	ctx := context.Background()
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT slug FROM projects WHERE id = $1", f.projectID).Scan(&f.projectSlug))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT slug FROM projects WHERE id = $1", f.siblingID).Scan(&f.siblingSlug))
	return f
}

func insertTestBlueprintRow(t *testing.T, userID, teamID, projectID string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO blueprints (id, user_id, team_id, project_id, slug, title, content, path) "+
			"VALUES ($1, $2, $3, $4, $5, 'b', 'content', 'CLAUDE.md')",
		id, userID, teamID, projectID, "bp-"+id[:8])
	require.NoError(t, err)
	return id
}

// recordWrite stores one write event through the repository under test.
func recordWrite(t *testing.T, f writeEventFixture, teamID, projectID, resourceType, resourceID, action, source string) {
	t.Helper()
	require.NoError(t, NewResourceWriteEventRepository(integrationDB).Create(context.Background(),
		&models.ResourceWriteEvent{
			TeamID:       teamID,
			ProjectID:    &projectID,
			UserID:       &f.userID,
			ResourceType: resourceType,
			ResourceID:   resourceID,
			Action:       action,
			Source:       source,
		}))
}

// seedMixedWrites fills the main project with resources created through every
// channel plus some with no create event at all (as every resource written
// before migration 022 is), and puts decoys in the sibling project and the
// other team.
//
// Main project, by type and channel:
//
//	prompts:    web 1, mcp 1, unknown 1          = 3
//	artifacts:  cli 1, unknown 1                 = 2
//	blueprints: api 1                            = 1
//	memories:   mcp 2, unknown 1                 = 3
//
// Sibling project: prompts mcp 1, memories unknown 1.
func seedMixedWrites(t *testing.T, f writeEventFixture) {
	t.Helper()
	create := models.WriteActionCreate
	update := models.WriteActionUpdate

	p1 := insertTestPrompt(t, f.userID, f.teamID, f.projectID, "p1", "body", "published")
	p2 := insertTestPrompt(t, f.userID, f.teamID, f.projectID, "p2", "body", "published")
	insertTestPrompt(t, f.userID, f.teamID, f.projectID, "p3-legacy", "body", "published")
	recordWrite(t, f, f.teamID, f.projectID, "prompt", p1, create, "web")
	recordWrite(t, f, f.teamID, f.projectID, "prompt", p2, create, "mcp")
	// Updates must never be counted as creations, whatever their channel.
	recordWrite(t, f, f.teamID, f.projectID, "prompt", p1, update, "mcp")
	recordWrite(t, f, f.teamID, f.projectID, "prompt", p1, update, "mcp")
	recordWrite(t, f, f.teamID, f.projectID, "prompt", p2, update, "cli")

	a1 := insertTestArtifact(t, f.userID, f.teamID, f.projectID, "a1", "content", "active")
	legacyArtifact := insertTestArtifact(t, f.userID, f.teamID, f.projectID, "a2-legacy", "content", "active")
	recordWrite(t, f, f.teamID, f.projectID, "artifact", a1, create, "cli")
	// A legacy resource edited after the migration: it has an update event and
	// still no create event, so its creation stays unknown.
	recordWrite(t, f, f.teamID, f.projectID, "artifact", legacyArtifact, update, "web")

	b1 := insertTestBlueprintRow(t, f.userID, f.teamID, f.projectID)
	recordWrite(t, f, f.teamID, f.projectID, "blueprint", b1, create, "api")

	m1 := insertTestMemory(t, f.userID, f.teamID, f.projectID, "m1")
	m2 := insertTestMemory(t, f.userID, f.teamID, f.projectID, "m2")
	insertTestMemory(t, f.userID, f.teamID, f.projectID, "m3-legacy")
	recordWrite(t, f, f.teamID, f.projectID, "memory", m1, create, "mcp")
	recordWrite(t, f, f.teamID, f.projectID, "memory", m2, create, "mcp")

	// Sibling project of the same team.
	sp := insertTestPrompt(t, f.userID, f.teamID, f.siblingID, "sp", "body", "published")
	recordWrite(t, f, f.teamID, f.siblingID, "prompt", sp, create, "mcp")
	insertTestMemory(t, f.userID, f.teamID, f.siblingID, "sm-legacy")

	// Another team entirely: must never leak into this team's numbers.
	op := insertTestPrompt(t, f.userID, f.otherTeamID, f.otherProject, "op", "body", "published")
	recordWrite(t, f, f.otherTeamID, f.otherProject, "prompt", op, create, "web")
	recordWrite(t, f, f.otherTeamID, f.otherProject, "prompt", op, update, "web")
}

// bySourceMap flattens the repository rows into resource type -> source -> count.
func bySourceMap(rows []models.ResourceCreationSourceCount) map[string]map[string]int {
	out := map[string]map[string]int{}
	for _, row := range rows {
		if out[row.ResourceType] == nil {
			out[row.ResourceType] = map[string]int{}
		}
		out[row.ResourceType][row.Source] += row.Count
	}
	return out
}

func sumSources(m map[string]int) int {
	total := 0
	for _, n := range m {
		total += n
	}
	return total
}

func writesMap(rows []models.ResourceWriteSourceCount) map[string]int {
	out := map[string]int{}
	for _, row := range rows {
		out[row.Source] += row.Count
	}
	return out
}

func TestIntegrationResourceWriteEvent_CreateRoundTrips(t *testing.T) {
	f := newWriteEventFixture(t)
	memoryID := insertTestMemory(t, f.userID, f.teamID, f.projectID, "m")
	assistant := "claude-code"

	event := &models.ResourceWriteEvent{
		TeamID:        f.teamID,
		ProjectID:     &f.projectID,
		UserID:        &f.userID,
		ResourceType:  "memory",
		ResourceID:    memoryID,
		Action:        models.WriteActionCreate,
		Source:        "mcp",
		AssistantName: &assistant,
	}
	require.NoError(t, NewResourceWriteEventRepository(integrationDB).Create(context.Background(), event))

	assert.NotEmpty(t, event.ID)
	assert.WithinDuration(t, time.Now(), event.CreatedAt, time.Minute)

	var source, action string
	var storedAssistant, apiKeyID *string
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT source, action, assistant_name, api_key_id::text FROM resource_write_events WHERE id = $1", event.ID,
	).Scan(&source, &action, &storedAssistant, &apiKeyID))
	assert.Equal(t, "mcp", source)
	assert.Equal(t, "create", action)
	require.NotNil(t, storedAssistant)
	assert.Equal(t, "claude-code", *storedAssistant)
	assert.Nil(t, apiKeyID, "an absent api key is stored as NULL")
}

// A resource has at most one create event. This is what lets the metrics LEFT
// JOIN on it without fanning a resource out into two rows.
func TestIntegrationResourceWriteEvent_OneCreateEventPerResource(t *testing.T) {
	f := newWriteEventFixture(t)
	promptID := insertTestPrompt(t, f.userID, f.teamID, f.projectID, "p", "body", "published")
	repo := NewResourceWriteEventRepository(integrationDB)
	event := func(action string) *models.ResourceWriteEvent {
		return &models.ResourceWriteEvent{
			TeamID: f.teamID, ResourceType: "prompt", ResourceID: promptID, Action: action, Source: "web",
		}
	}

	require.NoError(t, repo.Create(context.Background(), event(models.WriteActionCreate)))

	err := repo.Create(context.Background(), event(models.WriteActionCreate))
	var pqErr *pq.Error
	require.ErrorAs(t, err, &pqErr)
	assert.Equal(t, pq.ErrorCode("23505"), pqErr.Code, "a second create event must violate the unique index")

	// Updates are unlimited.
	require.NoError(t, repo.Create(context.Background(), event(models.WriteActionUpdate)))
	require.NoError(t, repo.Create(context.Background(), event(models.WriteActionUpdate)))
}

func TestIntegrationResourceWriteEvents_TeamCreationBySource(t *testing.T) {
	f := newWriteEventFixture(t)
	seedMixedWrites(t, f)
	since := time.Now().UTC().Add(-time.Hour)
	ctx := context.Background()

	rows, err := NewResourceWriteEventRepository(integrationDB).GetTeamCreationCountsBySource(ctx, f.teamID, since)
	require.NoError(t, err)

	got := bySourceMap(rows)
	assert.Equal(t, map[string]map[string]int{
		"prompts":    {"web": 1, "mcp": 2, "unknown": 1}, // main project + the sibling's mcp prompt
		"artifacts":  {"cli": 1, "unknown": 1},
		"blueprints": {"api": 1},
		"memories":   {"mcp": 2, "unknown": 2}, // main project + the sibling's legacy memory
	}, got)

	// The promise: per type, the channels sum to the per-day series' total over
	// the same window. The series query is the existing one, untouched.
	series, err := NewTeamRepository(integrationDB).GetTeamResourceCreationMetrics(ctx, f.teamID, since)
	require.NoError(t, err)
	totals := map[string]int{}
	for _, row := range series {
		totals[row.ResourceType] += row.Count
	}
	for _, resourceType := range []string{"prompts", "artifacts", "blueprints", "memories"} {
		assert.Equal(t, totals[resourceType], sumSources(got[resourceType]),
			"%s: the per-channel breakdown must sum to the series total", resourceType)
	}
	// The series also counts projects; the breakdown does not.
	assert.Equal(t, 2, totals["projects"])
	assert.NotContains(t, got, "projects")
}

func TestIntegrationResourceWriteEvents_ProjectCreationBySource(t *testing.T) {
	f := newWriteEventFixture(t)
	seedMixedWrites(t, f)
	since := time.Now().UTC().Add(-time.Hour)
	ctx := context.Background()
	repo := NewResourceWriteEventRepository(integrationDB)

	rows, err := repo.GetProjectCreationCountsBySource(ctx, f.teamID, f.projectSlug, since)
	require.NoError(t, err)

	got := bySourceMap(rows)
	assert.Equal(t, map[string]map[string]int{
		"prompts":    {"web": 1, "mcp": 1, "unknown": 1},
		"artifacts":  {"cli": 1, "unknown": 1},
		"blueprints": {"api": 1},
		"memories":   {"mcp": 2, "unknown": 1},
	}, got)

	series, err := NewProjectRepository(integrationDB).
		GetProjectResourceCreationMetrics(ctx, f.teamID, f.userID, f.projectSlug, since)
	require.NoError(t, err)
	totals := map[string]int{}
	for _, row := range series {
		totals[row.ResourceType] += row.Count
	}
	for _, resourceType := range []string{"prompts", "artifacts", "blueprints", "memories"} {
		assert.Equal(t, totals[resourceType], sumSources(got[resourceType]),
			"%s: the per-channel breakdown must sum to the series total", resourceType)
	}

	sibling, err := repo.GetProjectCreationCountsBySource(ctx, f.teamID, f.siblingSlug, since)
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string]int{
		"prompts":  {"mcp": 1},
		"memories": {"unknown": 1},
	}, bySourceMap(sibling))

	// A slug is resolved inside its own team: the same slug asked of another
	// team, and a slug nobody has, both match nothing.
	wrongTeam, err := repo.GetProjectCreationCountsBySource(ctx, f.otherTeamID, f.projectSlug, since)
	require.NoError(t, err)
	assert.Empty(t, wrongTeam)
	missing, err := repo.GetProjectCreationCountsBySource(ctx, f.teamID, "no-such-project", since)
	require.NoError(t, err)
	assert.Empty(t, missing)
}

// Resources created before the window are left out of the breakdown exactly as
// they are left out of the series, whether or not they have a create event.
func TestIntegrationResourceWriteEvents_CreationBySourceHonoursWindow(t *testing.T) {
	f := newWriteEventFixture(t)
	old := time.Now().UTC().AddDate(0, 0, -40)
	ctx := context.Background()

	oldPrompt := insertTestPrompt(t, f.userID, f.teamID, f.projectID, "old", "body", "published")
	_, err := integrationDB.ExecContext(ctx, "UPDATE prompts SET created_at = $1 WHERE id = $2", old, oldPrompt)
	require.NoError(t, err)
	recordWrite(t, f, f.teamID, f.projectID, "prompt", oldPrompt, models.WriteActionCreate, "web")

	newPrompt := insertTestPrompt(t, f.userID, f.teamID, f.projectID, "new", "body", "published")
	recordWrite(t, f, f.teamID, f.projectID, "prompt", newPrompt, models.WriteActionCreate, "mcp")

	rows, err := NewResourceWriteEventRepository(integrationDB).
		GetTeamCreationCountsBySource(ctx, f.teamID, time.Now().UTC().AddDate(0, 0, -30))
	require.NoError(t, err)

	assert.Equal(t, map[string]map[string]int{"prompts": {"mcp": 1}}, bySourceMap(rows))
}

// Same data, two session time zones, identical answer -- with rows placed
// exactly on the window edge, where a session-dependent comparison of the naive
// memories.created_at column drops a row (#773).
func TestIntegrationResourceWriteEvents_CreationBySourceIgnoresSessionTimezone(t *testing.T) {
	f := newWriteEventFixture(t)
	ctx := context.Background()
	edge := time.Date(2026, 3, 1, 23, 30, 0, 0, time.UTC)

	promptID := insertTestPrompt(t, f.userID, f.teamID, f.projectID, "edge", "body", "published")
	_, err := integrationDB.ExecContext(ctx, "UPDATE prompts SET created_at = $1 WHERE id = $2", edge, promptID)
	require.NoError(t, err)
	recordWrite(t, f, f.teamID, f.projectID, "prompt", promptID, models.WriteActionCreate, "web")

	// INSERTed with an explicit naive value: an UPDATE would fire the memories
	// trigger, and the column is naive, so it is written as it is stored.
	memoryID := uuid.New().String()
	_, err = integrationDB.ExecContext(ctx,
		`INSERT INTO memories (id, user_id, team_id, project_id, text, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 'edge', $5::timestamp, $5::timestamp)`,
		memoryID, f.userID, f.teamID, f.projectID, edge.Format("2006-01-02 15:04:05"))
	require.NoError(t, err)
	recordWrite(t, f, f.teamID, f.projectID, "memory", memoryID, models.WriteActionCreate, "mcp")

	// The window starts EXACTLY at the seeded instant: a wider margin would
	// swallow the session offset and the test would see nothing.
	since := edge
	want := map[string]map[string]int{"prompts": {"web": 1}, "memories": {"mcp": 1}}
	shiftedRepo := NewResourceWriteEventRepository(openSessionTZDB(t, tzTestZone))

	utc, err := NewResourceWriteEventRepository(integrationDB).GetTeamCreationCountsBySource(ctx, f.teamID, since)
	require.NoError(t, err)
	assert.Equal(t, want, bySourceMap(utc))

	shifted, err := shiftedRepo.GetTeamCreationCountsBySource(ctx, f.teamID, since)
	require.NoError(t, err)
	assert.Equal(t, want, bySourceMap(shifted), "the team breakdown must not follow the session time zone")

	shiftedProject, err := shiftedRepo.GetProjectCreationCountsBySource(ctx, f.teamID, f.projectSlug, since)
	require.NoError(t, err)
	assert.Equal(t, want, bySourceMap(shiftedProject), "the project breakdown must not follow the session time zone")
}

func TestIntegrationResourceWriteEvents_WriteCountsBySource(t *testing.T) {
	f := newWriteEventFixture(t)
	seedMixedWrites(t, f)
	ctx := context.Background()
	repo := NewResourceWriteEventRepository(integrationDB)
	since := time.Now().UTC().Add(-time.Hour)

	team, err := repo.GetTeamWriteCountsBySource(ctx, f.teamID, since)
	require.NoError(t, err)
	// Creates and updates alike, across both projects of the team:
	//   web: p1 create, legacy artifact update            = 2
	//   mcp: p2 create, 2 p1 updates, m1, m2, sibling sp  = 6
	//   cli: p2 update, a1 create                         = 2
	//   api: b1 create                                    = 1
	assert.Equal(t, map[string]int{"web": 2, "mcp": 6, "cli": 2, "api": 1}, writesMap(team))

	project, err := repo.GetProjectWriteCountsBySource(ctx, f.teamID, f.projectSlug, since)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"web": 2, "mcp": 5, "cli": 2, "api": 1}, writesMap(project))

	sibling, err := repo.GetProjectWriteCountsBySource(ctx, f.teamID, f.siblingSlug, since)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"mcp": 1}, writesMap(sibling))

	// An event from before the window is not counted.
	_, err = integrationDB.ExecContext(ctx,
		"UPDATE resource_write_events SET created_at = now() - interval '3 hours' WHERE team_id = $1 AND source = 'api'",
		f.teamID)
	require.NoError(t, err)
	team, err = repo.GetTeamWriteCountsBySource(ctx, f.teamID, since)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"web": 2, "mcp": 6, "cli": 2}, writesMap(team))

	missing, err := repo.GetProjectWriteCountsBySource(ctx, f.teamID, "no-such-project", since)
	require.NoError(t, err)
	assert.Empty(t, missing)
}

// A version written with a channel reads it back; one written without reads
// NULL, never a default channel.
func TestIntegrationContentVersion_SourceRoundTrips(t *testing.T) {
	f := newWriteEventFixture(t)
	artifactID := insertTestArtifact(t, f.userID, f.teamID, f.projectID, "a", "content", "active")
	repo := NewContentVersionRepository(integrationDB)
	ctx := context.Background()
	source, assistant := "mcp", "claude-code"

	require.NoError(t, repo.Create(ctx, &models.ContentVersion{
		TeamID: f.teamID, ResourceType: "artifact", ResourceID: artifactID, Content: "v1",
	}))
	require.NoError(t, repo.Create(ctx, &models.ContentVersion{
		TeamID: f.teamID, ResourceType: "artifact", ResourceID: artifactID, Content: "v2",
		Source: &source, AssistantName: &assistant,
	}))

	versions, err := repo.ListByResource(ctx, f.teamID, "artifact", artifactID)
	require.NoError(t, err)
	require.Len(t, versions, 2)

	require.NotNil(t, versions[0].Source)
	assert.Equal(t, "mcp", *versions[0].Source)
	require.NotNil(t, versions[0].AssistantName)
	assert.Equal(t, "claude-code", *versions[0].AssistantName)

	assert.Nil(t, versions[1].Source)
	assert.Nil(t, versions[1].AssistantName)

	one, err := repo.GetByVersionNumber(ctx, f.teamID, "artifact", artifactID, 2)
	require.NoError(t, err)
	require.NotNil(t, one.Source)
	assert.Equal(t, "mcp", *one.Source)
}
