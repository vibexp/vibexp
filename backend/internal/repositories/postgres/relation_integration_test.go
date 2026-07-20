//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// newRelation builds an edge row for the repository (which persists status and
// origin verbatim; the tiered-trust rule lives in the service layer).
func newRelation(at accessTeam, createdBy, fromType, fromID, toType, toID, relType, origin, status string) *models.Relation {
	return &models.Relation{
		TeamID:       at.teamID,
		ProjectID:    at.projectID,
		FromType:     fromType,
		FromID:       fromID,
		ToType:       toType,
		ToID:         toID,
		RelationType: relType,
		Origin:       origin,
		Status:       status,
		CreatedBy:    &createdBy,
	}
}

func TestIntegrationRelation_CreateIsIdempotent(t *testing.T) {
	resetIntegrationTables(t)
	at := seedAccessTeam(t)
	artifactID := insertAccessArtifact(t, at, at.ownerID, "rel-artifact")
	blueprintID := insertAccessBlueprint(t, at, at.ownerID, "rel-blueprint")
	repo := NewRelationRepository(integrationDB)
	ctx := context.Background()

	first, created, err := repo.Create(ctx, newRelation(at, at.ownerID,
		models.RelationResourceTypeArtifact, artifactID, models.RelationResourceTypeBlueprint, blueprintID,
		models.RelationTypeGovernedBy, models.RelationOriginHuman, models.RelationStatusConfirmed))
	require.NoError(t, err)
	assert.True(t, created, "the first insert reports created=true")
	require.NotEmpty(t, first.ID)
	require.False(t, first.CreatedAt.IsZero())

	// Same endpoint tuple, different origin/status: the unique index makes this a
	// no-op that returns the PRE-EXISTING row unchanged.
	second, created, err := repo.Create(ctx, newRelation(at, at.memberID,
		models.RelationResourceTypeArtifact, artifactID, models.RelationResourceTypeBlueprint, blueprintID,
		models.RelationTypeGovernedBy, models.RelationOriginAI, models.RelationStatusSuggested))
	require.NoError(t, err)
	assert.False(t, created, "the duplicate reports created=false")
	assert.Equal(t, first.ID, second.ID, "duplicate create must return the existing row")
	assert.Equal(t, models.RelationStatusConfirmed, second.Status, "existing status is preserved")
	assert.Equal(t, models.RelationOriginHuman, second.Origin)
}

func TestIntegrationRelation_ListBothDirectionsAndDeleteEitherSide(t *testing.T) {
	resetIntegrationTables(t)
	at := seedAccessTeam(t)
	mainID := insertAccessArtifact(t, at, at.ownerID, "rel-main")
	otherArtifactID := insertAccessArtifact(t, at, at.ownerID, "rel-successor")
	blueprintID := insertAccessBlueprint(t, at, at.ownerID, "rel-governor")
	repo := NewRelationRepository(integrationDB)
	ctx := context.Background()

	// Outgoing: main --governed-by--> blueprint.
	_, _, err := repo.Create(ctx, newRelation(at, at.ownerID,
		models.RelationResourceTypeArtifact, mainID, models.RelationResourceTypeBlueprint, blueprintID,
		models.RelationTypeGovernedBy, models.RelationOriginHuman, models.RelationStatusConfirmed))
	require.NoError(t, err)

	// Incoming: successor --supersedes--> main.
	_, _, err = repo.Create(ctx, newRelation(at, at.ownerID,
		models.RelationResourceTypeArtifact, otherArtifactID, models.RelationResourceTypeArtifact, mainID,
		models.RelationTypeSupersedes, models.RelationOriginHuman, models.RelationStatusConfirmed))
	require.NoError(t, err)

	related, total, err := repo.ListByResource(ctx, at.teamID, models.RelationResourceTypeArtifact, mainID, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, related, 2)

	byDir := map[string]models.RelatedResource{}
	for _, r := range related {
		byDir[r.Direction] = r
	}
	require.Contains(t, byDir, models.RelationDirectionOutgoing)
	require.Contains(t, byDir, models.RelationDirectionIncoming)
	assert.Equal(t, models.RelationResourceTypeBlueprint, byDir[models.RelationDirectionOutgoing].ResourceType)
	assert.Equal(t, "Blueprint Title", byDir[models.RelationDirectionOutgoing].Title)
	assert.Equal(t, otherArtifactID, byDir[models.RelationDirectionIncoming].ResourceID)
	assert.Equal(t, "Artifact Title", byDir[models.RelationDirectionIncoming].Title)

	// Deleting the main artifact's edges must remove BOTH — the one where it is
	// the subject and the one where it is the object.
	n, err := repo.DeleteByResource(ctx, at.teamID, models.RelationResourceTypeArtifact, mainID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	_, total, err = repo.ListByResource(ctx, at.teamID, models.RelationResourceTypeArtifact, mainID, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
}

func TestIntegrationRelation_ConfirmFlipsOnce(t *testing.T) {
	resetIntegrationTables(t)
	at := seedAccessTeam(t)
	artifactID := insertAccessArtifact(t, at, at.ownerID, "rel-confirm-art")
	blueprintID := insertAccessBlueprint(t, at, at.ownerID, "rel-confirm-bp")
	repo := NewRelationRepository(integrationDB)
	ctx := context.Background()

	created, wasCreated, err := repo.Create(ctx, newRelation(at, at.ownerID,
		models.RelationResourceTypeArtifact, artifactID, models.RelationResourceTypeBlueprint, blueprintID,
		models.RelationTypeGovernedBy, models.RelationOriginAI, models.RelationStatusSuggested))
	require.NoError(t, err)
	require.True(t, wasCreated)
	require.Equal(t, models.RelationStatusSuggested, created.Status)

	confirmed, err := repo.Confirm(ctx, at.teamID, created.ID, at.adminID)
	require.NoError(t, err)
	assert.Equal(t, models.RelationStatusConfirmed, confirmed.Status)
	require.NotNil(t, confirmed.ConfirmedBy)
	assert.Equal(t, at.adminID, *confirmed.ConfirmedBy)

	// A second confirm finds no suggested row and reports not-found.
	_, err = repo.Confirm(ctx, at.teamID, created.ID, at.adminID)
	assert.ErrorIs(t, err, repositories.ErrRelationNotFound)
}

func TestIntegrationRelation_ResourceProjectID(t *testing.T) {
	resetIntegrationTables(t)
	at := seedAccessTeam(t)
	artifactID := insertAccessArtifact(t, at, at.ownerID, "rel-proj-art")
	repo := NewRelationRepository(integrationDB)
	ctx := context.Background()

	proj, exists, err := repo.ResourceProjectID(ctx, at.teamID, models.RelationResourceTypeArtifact, artifactID)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, at.projectID, proj)

	_, exists, err = repo.ResourceProjectID(ctx, at.teamID, models.RelationResourceTypeArtifact, uuid.New().String())
	require.NoError(t, err)
	assert.False(t, exists)
}
