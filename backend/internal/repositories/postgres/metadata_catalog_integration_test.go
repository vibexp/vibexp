//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level suite for MetadataCatalogRepository against real Postgres
// (#519). The sqlmock tests assert the generated SQL; these assert what the
// database actually returns — in particular that the tenancy predicate really
// keeps another team's keys and values out of the catalog (the #517 bug class),
// and that the JSONB expansion handles the shapes metadata is stored in.

func resetMetadataCatalogTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, teams, projects, artifacts, memories, blueprints CASCADE")
	require.NoError(t, err)
}

// insertBlueprintWithMetadata seeds one blueprint carrying the given metadata
// JSON. metadataJSON is raw JSON text so a test can store shapes the Go model
// cannot express (numbers, booleans, arrays, nested objects).
func insertBlueprintWithMetadata(t *testing.T, userID, teamID, projectID, metadataJSON string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		`INSERT INTO blueprints (id, slug, user_id, team_id, project_id, content, title, path, metadata)
		 VALUES ($1, $2, $3, $4, $5, 'content', 'Title', $6, $7::jsonb)`,
		id, "bp-"+id[:8], userID, teamID, projectID, "bp-"+id[:8]+".md", metadataJSON)
	require.NoError(t, err)
	return id
}

type metadataCatalogFixture struct {
	repo      repositories.MetadataCatalogRepository
	userID    string
	teamID    string
	projectID string
}

// newMetadataCatalogFixture seeds one team owned by the caller plus a SECOND
// team, owned by a different user, holding metadata that must never surface.
func newMetadataCatalogFixture(t *testing.T) metadataCatalogFixture {
	t.Helper()
	resetMetadataCatalogTables(t)

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	projectID := insertTestProject(t, userID, teamID)

	otherUserID := insertTestUser(t)
	otherTeamID := insertTestTeam(t, otherUserID)
	otherProjectID := insertTestProject(t, otherUserID, otherTeamID)
	insertBlueprintWithMetadata(t, otherUserID, otherTeamID, otherProjectID,
		`{"foreign-key":"foreign-value","env":"foreign-env"}`)

	return metadataCatalogFixture{
		repo:      NewMetadataCatalogRepository(integrationDB),
		userID:    userID,
		teamID:    teamID,
		projectID: projectID,
	}
}

func (f metadataCatalogFixture) query() repositories.MetadataCatalogQuery {
	return repositories.MetadataCatalogQuery{
		UserID:       f.userID,
		TeamID:       f.teamID,
		ResourceType: repositories.MetadataResourceBlueprints,
		Limit:        100,
	}
}

func TestIntegrationMetadataCatalog_KeysNeverLeakAcrossTeams(t *testing.T) {
	f := newMetadataCatalogFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"prod","owner":"core"}`)

	result, err := f.repo.Keys(context.Background(), f.query())

	require.NoError(t, err)
	assert.Equal(t, []string{"env", "owner"}, result.Entries)
	assert.NotContains(t, result.Entries, "foreign-key")
}

func TestIntegrationMetadataCatalog_ValuesNeverLeakAcrossTeams(t *testing.T) {
	f := newMetadataCatalogFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"prod"}`)

	query := f.query()
	query.Key = "env"
	result, err := f.repo.Values(context.Background(), query)

	require.NoError(t, err)
	// The other team also has an `env` key, so this is the sharper assertion:
	// the key name matches, only the tenancy predicate keeps its value out.
	assert.Equal(t, []string{"prod"}, result.Entries)
	assert.NotContains(t, result.Entries, "foreign-env")
}

// TestIntegrationMetadataCatalog_NonMemberSeesNothing covers the read-access
// half of the predicate: a valid user asking about a team they do not belong
// to gets an empty catalog, not that team's keys.
func TestIntegrationMetadataCatalog_NonMemberSeesNothing(t *testing.T) {
	f := newMetadataCatalogFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"prod"}`)

	query := f.query()
	query.UserID = insertTestUser(t) // a real user, but not a member of the team

	result, err := f.repo.Keys(context.Background(), query)

	require.NoError(t, err)
	assert.Empty(t, result.Entries)
}

func TestIntegrationMetadataCatalog_ValuesFlattensArraysAndScalars(t *testing.T) {
	f := newMetadataCatalogFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"prod"}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":["staging","qa"]}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":3}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":true}`)

	query := f.query()
	query.Key = "env"
	result, err := f.repo.Values(context.Background(), query)

	require.NoError(t, err)
	// Array-valued metadata flattens, and non-string scalars render in their
	// text form, so every value is usable directly as filter input.
	assert.ElementsMatch(t, []string{"prod", "staging", "qa", "3", "true"}, result.Entries)
}

// TestIntegrationMetadataCatalog_ValuesSkipsObjectValuedKeys covers the shape
// with no meaningful value list: without the guard, jsonb_array_elements_text
// would emit the object's JSON text as if it were a filterable value.
func TestIntegrationMetadataCatalog_ValuesSkipsObjectValuedKeys(t *testing.T) {
	f := newMetadataCatalogFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":{"nested":"deep"}}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"prod"}`)

	query := f.query()
	query.Key = "env"
	result, err := f.repo.Values(context.Background(), query)

	require.NoError(t, err)
	assert.Equal(t, []string{"prod"}, result.Entries)
}

// TestIntegrationMetadataCatalog_ValuesSkipsRowsWithoutTheKey guards the
// jsonb_exists filter: without it, `metadata -> key` is NULL, the array
// coercion yields [null], and a spurious empty entry appears.
func TestIntegrationMetadataCatalog_ValuesSkipsRowsWithoutTheKey(t *testing.T) {
	f := newMetadataCatalogFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"other":"x"}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"prod"}`)

	query := f.query()
	query.Key = "env"
	result, err := f.repo.Values(context.Background(), query)

	require.NoError(t, err)
	assert.Equal(t, []string{"prod"}, result.Entries)
}

func TestIntegrationMetadataCatalog_KeysNarrowedByProject(t *testing.T) {
	f := newMetadataCatalogFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"in-project":"y"}`)

	otherProjectID := insertTestProject(t, f.userID, f.teamID)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, otherProjectID, `{"other-project":"y"}`)

	query := f.query()
	query.ProjectID = &f.projectID
	result, err := f.repo.Keys(context.Background(), query)

	require.NoError(t, err)
	assert.Equal(t, []string{"in-project"}, result.Entries)
}

func TestIntegrationMetadataCatalog_ValuesTypeahead(t *testing.T) {
	f := newMetadataCatalogFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"production"}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"staging"}`)

	query := f.query()
	query.Key = "env"
	search := "PROD" // case-insensitive by design
	query.Search = &search

	result, err := f.repo.Values(context.Background(), query)

	require.NoError(t, err)
	assert.Equal(t, []string{"production"}, result.Entries)
}

func TestIntegrationMetadataCatalog_KeysReportsTruncation(t *testing.T) {
	f := newMetadataCatalogFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"a":"1","b":"2","c":"3"}`)

	query := f.query()
	query.Limit = 2

	result, err := f.repo.Keys(context.Background(), query)

	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, result.Entries)
	assert.True(t, result.Truncated)
}

// TestIntegrationMetadataCatalog_AllResourceTypes proves the closed table map
// points at real, queryable tables — a typo would only show up here.
func TestIntegrationMetadataCatalog_AllResourceTypes(t *testing.T) {
	for _, resourceType := range []repositories.MetadataResourceType{
		repositories.MetadataResourceArtifacts,
		repositories.MetadataResourceBlueprints,
		repositories.MetadataResourceMemories,
	} {
		t.Run(string(resourceType), func(t *testing.T) {
			f := newMetadataCatalogFixture(t)

			query := f.query()
			query.ResourceType = resourceType
			_, err := f.repo.Keys(context.Background(), query)
			require.NoError(t, err)

			query.Key = "env"
			_, err = f.repo.Values(context.Background(), query)
			require.NoError(t, err)
		})
	}
}
