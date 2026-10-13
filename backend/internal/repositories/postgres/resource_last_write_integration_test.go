//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// The last write of a resource, resolved by the list and detail reads of the
// four resource repositories against real Postgres (#1418).
//
// What only a real database can check: that the LATERAL join picks the newest
// event of THIS resource (not the newest of its team, not an event of another
// resource type that happens to share the id), that a resource with no event
// reads NULL rather than a default channel, and that the list and the detail
// read of one resource agree.

// lastWriteReader is the list and detail read of one resource type, behind one
// shape so the four types run through the same assertions.
type lastWriteReader struct {
	resourceType string
	insert       func(t *testing.T, f writeEventFixture, name string) string
	// list returns the last write of every resource of the team, by id.
	list func(t *testing.T, f writeEventFixture) map[string]models.LastWrite
	// detail returns the last write the single-resource read reports.
	detail func(t *testing.T, f writeEventFixture, id string) models.LastWrite
}

func lastWriteReaders() []lastWriteReader {
	ctx := context.Background()
	return []lastWriteReader{
		{
			resourceType: models.RelationResourceTypePrompt,
			insert: func(t *testing.T, f writeEventFixture, name string) string {
				return insertTestPrompt(t, f.userID, f.teamID, f.projectID, name, "body", "published")
			},
			list: func(t *testing.T, f writeEventFixture) map[string]models.LastWrite {
				rows, total, err := NewPromptRepository(integrationDB).List(ctx, f.userID,
					repositories.PromptFilters{TeamID: f.teamID, Page: 1, Limit: 100})
				require.NoError(t, err)
				require.Len(t, rows, total, "the lateral join must not change the row count")
				out := make(map[string]models.LastWrite, len(rows))
				for i := range rows {
					out[rows[i].ID] = rows[i].LastWrite
				}
				return out
			},
			detail: func(t *testing.T, f writeEventFixture, id string) models.LastWrite {
				got, err := NewPromptRepository(integrationDB).GetByID(ctx, f.userID, f.teamID, id)
				require.NoError(t, err)
				return got.LastWrite
			},
		},
		{
			resourceType: models.RelationResourceTypeArtifact,
			insert: func(t *testing.T, f writeEventFixture, name string) string {
				return insertTestArtifact(t, f.userID, f.teamID, f.projectID, name, "content", "active")
			},
			list: func(t *testing.T, f writeEventFixture) map[string]models.LastWrite {
				rows, total, err := NewArtifactRepository(integrationDB).List(ctx, f.userID,
					repositories.ArtifactFilters{TeamID: f.teamID, Page: 1, Limit: 100})
				require.NoError(t, err)
				require.Len(t, rows, total, "the lateral join must not change the row count")
				out := make(map[string]models.LastWrite, len(rows))
				for i := range rows {
					out[rows[i].ID] = rows[i].LastWrite
				}
				return out
			},
			detail: func(t *testing.T, f writeEventFixture, id string) models.LastWrite {
				got, err := NewArtifactRepository(integrationDB).GetByID(ctx, f.userID, f.teamID, id)
				require.NoError(t, err)
				return got.LastWrite
			},
		},
		{
			resourceType: models.RelationResourceTypeBlueprint,
			// Not insertTestBlueprintRow: it pins the path, and a project
			// holds one blueprint per path.
			insert: func(t *testing.T, f writeEventFixture, name string) string {
				id := uuid.New().String()
				_, err := integrationDB.ExecContext(ctx,
					"INSERT INTO blueprints (id, user_id, team_id, project_id, slug, title, content, path) "+
						"VALUES ($1, $2, $3, $4, $5, $6, 'content', $7)",
					id, f.userID, f.teamID, f.projectID, "bp-"+id[:8], name, ".claude/"+name+".md")
				require.NoError(t, err)
				return id
			},
			list: func(t *testing.T, f writeEventFixture) map[string]models.LastWrite {
				rows, total, err := NewBlueprintRepository(integrationDB).List(ctx, f.userID,
					repositories.BlueprintFilters{TeamID: f.teamID, Page: 1, Limit: 100})
				require.NoError(t, err)
				require.Len(t, rows, total, "the lateral join must not change the row count")
				out := make(map[string]models.LastWrite, len(rows))
				for i := range rows {
					out[rows[i].ID] = rows[i].LastWrite
				}
				return out
			},
			detail: func(t *testing.T, f writeEventFixture, id string) models.LastWrite {
				got, err := NewBlueprintRepository(integrationDB).GetByID(ctx, f.userID, f.teamID, id)
				require.NoError(t, err)
				return got.LastWrite
			},
		},
		{
			resourceType: models.RelationResourceTypeMemory,
			insert: func(t *testing.T, f writeEventFixture, name string) string {
				return insertTestMemory(t, f.userID, f.teamID, f.projectID, name)
			},
			list: func(t *testing.T, f writeEventFixture) map[string]models.LastWrite {
				rows, total, err := NewMemoryRepository(integrationDB).List(ctx, f.userID,
					repositories.MemoryFilters{TeamID: f.teamID, Page: 1, Limit: 100})
				require.NoError(t, err)
				require.Len(t, rows, total, "the lateral join must not change the row count")
				out := make(map[string]models.LastWrite, len(rows))
				for i := range rows {
					out[rows[i].ID] = rows[i].LastWrite
				}
				return out
			},
			detail: func(t *testing.T, f writeEventFixture, id string) models.LastWrite {
				got, err := NewMemoryRepository(integrationDB).GetByID(ctx, f.userID, f.teamID, id)
				require.NoError(t, err)
				return got.LastWrite
			},
		},
	}
}

// insertWriteEventAt stores one write event with an explicit age, so the order
// of a resource's events is stated by the test rather than left to the clock
// between two inserts. assistant is "" for a write that reported no name.
func insertWriteEventAt(t *testing.T, f writeEventFixture, e writeEventSpec, assistant string, age time.Duration) {
	t.Helper()
	var assistantName *string
	if assistant != "" {
		assistantName = &assistant
	}
	_, err := integrationDB.ExecContext(context.Background(),
		`INSERT INTO resource_write_events
			(team_id, project_id, user_id, resource_type, resource_id, action, source, assistant_name, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now() - make_interval(secs => $9))`,
		e.teamID, e.projectID, f.userID, e.resourceType, e.resourceID, e.action, e.source, assistantName, age.Seconds())
	require.NoError(t, err)
}

// lastWriteOf builds the expected value; source "" is "no recorded write".
func lastWriteOf(source, assistant string) models.LastWrite {
	var lw models.LastWrite
	if source == "" {
		return lw
	}
	lw.LastWriteSource = &source
	if assistant != "" {
		lw.LastWriteAssistantName = &assistant
	}
	return lw
}

func TestIntegrationLastWrite_ListAndDetailResolveTheLatestEvent(t *testing.T) {
	for _, reader := range lastWriteReaders() {
		t.Run(reader.resourceType, func(t *testing.T) {
			f := newWriteEventFixture(t)
			event := func(id, action, source, assistant string, age time.Duration) {
				insertWriteEventAt(t, f, writeEventSpec{
					teamID: f.teamID, projectID: f.projectID,
					resourceType: reader.resourceType, resourceID: id, action: action, source: source,
				}, assistant, age)
			}
			create, update := models.WriteActionCreate, models.WriteActionUpdate

			// One resource per channel, each written once.
			web := reader.insert(t, f, "web")
			event(web, create, "web", "", time.Hour)
			cli := reader.insert(t, f, "cli")
			event(cli, create, "cli", "", time.Hour)
			mcp := reader.insert(t, f, "mcp")
			event(mcp, create, "mcp", "claude-code", time.Hour)
			api := reader.insert(t, f, "api")
			event(api, create, "api", "", time.Hour)

			// No event at all: written before write channels were recorded.
			none := reader.insert(t, f, "none")

			// Created through one channel, then updated through another: the
			// update's channel wins, and it brings its own assistant name.
			webThenMCP := reader.insert(t, f, "web-then-mcp")
			event(webThenMCP, create, "web", "", 3*time.Hour)
			event(webThenMCP, update, "mcp", "cursor", time.Hour)

			// The reverse: the earlier MCP write's assistant name must not
			// survive a later write that reported none.
			mcpThenWeb := reader.insert(t, f, "mcp-then-web")
			event(mcpThenWeb, create, "mcp", "claude-code", 3*time.Hour)
			event(mcpThenWeb, update, "cli", "", 2*time.Hour)
			event(mcpThenWeb, update, "web", "", time.Hour)

			// A legacy resource edited after recording began: no create event,
			// and its one update is still its latest write.
			legacyUpdated := reader.insert(t, f, "legacy-updated")
			event(legacyUpdated, update, "api", "", time.Hour)

			// The newest event of the TEAM belongs to another resource, and an
			// event of another resource type shares this resource's id. Neither
			// may leak into `none` or `web`.
			otherType := models.RelationResourceTypePrompt
			if reader.resourceType == otherType {
				otherType = models.RelationResourceTypeArtifact
			}
			insertWriteEventAt(t, f, writeEventSpec{
				teamID: f.teamID, projectID: f.projectID,
				resourceType: otherType, resourceID: none, action: update, source: "mcp",
			}, "intruder", time.Minute)
			insertWriteEventAt(t, f, writeEventSpec{
				teamID: f.teamID, projectID: f.projectID,
				resourceType: otherType, resourceID: web, action: update, source: "mcp",
			}, "intruder", time.Minute)

			want := map[string]models.LastWrite{
				web:           lastWriteOf("web", ""),
				cli:           lastWriteOf("cli", ""),
				mcp:           lastWriteOf("mcp", "claude-code"),
				api:           lastWriteOf("api", ""),
				none:          lastWriteOf("", ""),
				webThenMCP:    lastWriteOf("mcp", "cursor"),
				mcpThenWeb:    lastWriteOf("web", ""),
				legacyUpdated: lastWriteOf("api", ""),
			}

			assert.Equal(t, want, reader.list(t, f), "list")
			for id, lw := range want {
				assert.Equal(t, lw, reader.detail(t, f, id), "detail of %s", id)
			}
		})
	}
}

// The latest-event lookup is served by the index migration 026 adds for it,
// index-only: one descent per listed row, never a scan of the team's event log.
// Sequential scans are disabled for the statement because a table this small
// would be scanned whatever indexes exist; the assertion is that the planner
// CAN answer the lookup from the index alone, which is what holds at scale.
func TestIntegrationLastWrite_LookupIsServedByItsIndex(t *testing.T) {
	f := newWriteEventFixture(t)
	promptID := insertTestPrompt(t, f.userID, f.teamID, f.projectID, "p", "body", "published")
	for i := 0; i < 5; i++ {
		insertWriteEventAt(t, f, writeEventSpec{
			teamID: f.teamID, projectID: f.projectID,
			resourceType: "prompt", resourceID: promptID, action: models.WriteActionUpdate, source: "web",
		}, "", time.Duration(i+1)*time.Minute)
	}

	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off")
	require.NoError(t, err)

	rows, err := tx.QueryContext(ctx,
		"EXPLAIN SELECT p.id, "+resourceLastWriteProjection+" FROM prompts p LEFT JOIN "+
			resourceLastWriteLateral(models.RelationResourceTypePrompt, "p")+" WHERE p.team_id = $1", f.teamID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var plan strings.Builder
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan.WriteString(line + "\n")
	}
	require.NoError(t, rows.Err())

	assert.Contains(t, plan.String(), "Index Only Scan using idx_resource_write_events_resource_latest", plan.String())
	assert.NotContains(t, plan.String(), "Seq Scan on resource_write_events", plan.String())
}

// insertUnsourcedVersion stores a version snapshot with no source: what an
// update leaves behind when it changed the content and recorded no channel.
func insertUnsourcedVersion(t *testing.T, teamID, resourceType, resourceID string) {
	t.Helper()
	require.NoError(t, NewContentVersionRepository(integrationDB).Create(context.Background(),
		&models.ContentVersion{TeamID: teamID, ResourceType: resourceType, ResourceID: resourceID, Content: "prior"}))
}

func TestIntegrationResourceWriteEvents_UnsourcedUpdateCount(t *testing.T) {
	f := newWriteEventFixture(t)
	ctx := context.Background()
	repo := NewResourceWriteEventRepository(integrationDB)
	since := time.Now().UTC().Add(-time.Hour)

	prompt := insertTestPrompt(t, f.userID, f.teamID, f.projectID, "p", "body", "published")
	artifact := insertTestArtifact(t, f.userID, f.teamID, f.projectID, "a", "content", "active")
	blueprint := insertTestBlueprintRow(t, f.userID, f.teamID, f.projectID)
	memory := insertTestMemory(t, f.userID, f.teamID, f.projectID, "m")
	siblingPrompt := insertTestPrompt(t, f.userID, f.teamID, f.siblingID, "sp", "body", "published")
	otherPrompt := insertTestPrompt(t, f.userID, f.otherTeamID, f.otherProject, "op", "body", "published")

	// Main project: 2 + 1 + 1 + 1 = 5 unsourced content changes.
	insertUnsourcedVersion(t, f.teamID, "prompt", prompt)
	insertUnsourcedVersion(t, f.teamID, "prompt", prompt)
	insertUnsourcedVersion(t, f.teamID, "artifact", artifact)
	insertUnsourcedVersion(t, f.teamID, "blueprint", blueprint)
	insertUnsourcedVersion(t, f.teamID, "memory", memory)
	// Sibling project: 1. Other team: 1, which must never leak.
	insertUnsourcedVersion(t, f.teamID, "prompt", siblingPrompt)
	insertUnsourcedVersion(t, f.otherTeamID, "prompt", otherPrompt)

	// A snapshot that carries a channel belongs to a recorded update: it is
	// counted by its write event, never here.
	source := "mcp"
	require.NoError(t, NewContentVersionRepository(integrationDB).Create(ctx, &models.ContentVersion{
		TeamID: f.teamID, ResourceType: "prompt", ResourceID: prompt, Content: "prior", Source: &source,
	}))

	team, err := repo.GetTeamUnsourcedUpdateCount(ctx, f.teamID, since)
	require.NoError(t, err)
	assert.Equal(t, 6, team)

	project, err := repo.GetProjectUnsourcedUpdateCount(ctx, f.teamID, f.projectSlug, since)
	require.NoError(t, err)
	assert.Equal(t, 5, project)

	sibling, err := repo.GetProjectUnsourcedUpdateCount(ctx, f.teamID, f.siblingSlug, since)
	require.NoError(t, err)
	assert.Equal(t, 1, sibling)

	missing, err := repo.GetProjectUnsourcedUpdateCount(ctx, f.teamID, "no-such-project", since)
	require.NoError(t, err)
	assert.Zero(t, missing)

	// A snapshot from before the window is not counted.
	_, err = integrationDB.ExecContext(ctx,
		"UPDATE content_versions SET created_at = now() - interval '3 hours' "+
			"WHERE team_id = $1 AND resource_type = 'memory'", f.teamID)
	require.NoError(t, err)
	team, err = repo.GetTeamUnsourcedUpdateCount(ctx, f.teamID, since)
	require.NoError(t, err)
	assert.Equal(t, 5, team)
	project, err = repo.GetProjectUnsourcedUpdateCount(ctx, f.teamID, f.projectSlug, since)
	require.NoError(t, err)
	assert.Equal(t, 4, project)
}

// The identity the metrics endpoint promises, against real rows: recorded
// events + creations with no create event + unsourced content changes is every
// write of the window the database has a record of, with nothing counted
// twice. The expected total is counted here independently, straight off the
// three tables, rather than re-derived from the queries under test.
func TestIntegrationResourceWriteEvents_WritesSumToTheWindowTotal(t *testing.T) {
	f := newWriteEventFixture(t)
	seedMixedWrites(t, f)
	ctx := context.Background()
	repo := NewResourceWriteEventRepository(integrationDB)
	since := time.Now().UTC().Add(-time.Hour)

	// Unrecorded updates on top of the mixed fixture: two on a legacy prompt,
	// one on a memory that does have a create event.
	var legacyPrompt, recordedMemory string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT id FROM prompts WHERE team_id = $1 AND name = 'p3-legacy'", f.teamID).Scan(&legacyPrompt))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT id FROM memories WHERE team_id = $1 AND text = 'm1'", f.teamID).Scan(&recordedMemory))
	insertUnsourcedVersion(t, f.teamID, "prompt", legacyPrompt)
	insertUnsourcedVersion(t, f.teamID, "prompt", legacyPrompt)
	insertUnsourcedVersion(t, f.teamID, "memory", recordedMemory)

	created, err := repo.GetTeamCreationCountsBySource(ctx, f.teamID, since)
	require.NoError(t, err)
	writes, err := repo.GetTeamWriteCountsBySource(ctx, f.teamID, since)
	require.NoError(t, err)
	unsourced, err := repo.GetTeamUnsourcedUpdateCount(ctx, f.teamID, since)
	require.NoError(t, err)

	unknownCreations := 0
	for _, row := range created {
		if row.Source == "unknown" {
			unknownCreations += row.Count
		}
	}
	recorded := sumSources(writesMap(writes))
	// seedMixedWrites: 11 events for this team, and 4 resources with no create
	// event (p3-legacy, a2-legacy, m3-legacy, sibling sm-legacy).
	assert.Equal(t, 11, recorded)
	assert.Equal(t, 4, unknownCreations)
	assert.Equal(t, 3, unsourced)

	var wantTotal int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM resource_write_events WHERE team_id = $1)
			+ (SELECT COUNT(*) FROM content_versions WHERE team_id = $1 AND source IS NULL)
			+ (SELECT COUNT(*) FROM (
				SELECT id, 'prompt' AS kind FROM prompts WHERE team_id = $1
				UNION ALL SELECT id, 'artifact' FROM artifacts WHERE team_id = $1
				UNION ALL SELECT id, 'blueprint' FROM blueprints WHERE team_id = $1
				UNION ALL SELECT id, 'memory' FROM memories WHERE team_id = $1
			) r WHERE NOT EXISTS (
				SELECT 1 FROM resource_write_events e
				WHERE e.resource_type = r.kind AND e.resource_id = r.id AND e.action = 'create'))`,
		f.teamID).Scan(&wantTotal))
	assert.Equal(t, 18, wantTotal)
	assert.Equal(t, wantTotal, recorded+unknownCreations+unsourced)
}
