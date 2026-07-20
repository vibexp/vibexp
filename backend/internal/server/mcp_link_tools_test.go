package server

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// setRelationMock installs a relation-service mock on the test container.
func setRelationMock(srv *Server, relSvc *servicesmocks.MockRelationServiceInterface) {
	srv.container.(*TestContainer).RelationServiceMock = relSvc
}

func linkParams(relationType string) *LinkResourcesParams {
	return &LinkResourcesParams{
		TeamID:       testTeamUUID,
		ProjectID:    testProjectID,
		FromType:     models.RelationResourceTypeArtifact,
		FromID:       "art-1",
		RelationType: relationType,
		ToType:       models.RelationResourceTypeBlueprint,
		ToID:         "bp-1",
	}
}

func TestLinkResources_Success(t *testing.T) {
	srv, _ := newDeleteResourceTestServer(t)
	relSvc := servicesmocks.NewMockRelationServiceInterface(t)
	setRelationMock(srv, relSvc)

	rel := &models.Relation{
		ID: "rel-1", TeamID: testTeamUUID, ProjectID: testProjectID,
		FromType: "artifact", FromID: "art-1", ToType: "blueprint", ToID: "bp-1",
		RelationType: models.RelationTypeGovernedBy, Origin: models.RelationOriginAI,
		Status: models.RelationStatusSuggested,
	}
	// The MCP tool always proposes edges with origin=ai; the service applies the
	// tiered status (governed-by/ai -> suggested).
	relSvc.EXPECT().Create(mock.Anything, testMemberUserID, testTeamUUID, mock.MatchedBy(
		func(req *models.CreateRelationRequest) bool {
			return req.Origin == models.RelationOriginAI &&
				req.FromType == "artifact" && req.ToType == "blueprint" &&
				req.RelationType == models.RelationTypeGovernedBy
		},
	)).Return(rel, true, nil)

	result, structured, err := srv.linkResources(
		context.Background(), nil, linkParams(models.RelationTypeGovernedBy), testMemberUserID)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)
	got, ok := structured.(*models.Relation)
	require.True(t, ok, "structured payload is a *models.Relation")
	assert.Equal(t, "rel-1", got.ID)
	assert.Equal(t, models.RelationStatusSuggested, got.Status)
}

// A duplicate link is a no-op that returns the existing edge (created=false).
func TestLinkResources_IdempotentReturnsExisting(t *testing.T) {
	srv, _ := newDeleteResourceTestServer(t)
	relSvc := servicesmocks.NewMockRelationServiceInterface(t)
	setRelationMock(srv, relSvc)

	existing := &models.Relation{ID: "rel-existing", Status: models.RelationStatusConfirmed}
	relSvc.EXPECT().Create(mock.Anything, testMemberUserID, testTeamUUID, mock.Anything).
		Return(existing, false, nil)

	result, structured, err := srv.linkResources(
		context.Background(), nil, linkParams(models.RelationTypeBuiltFrom), testMemberUserID)

	require.NoError(t, err)
	require.False(t, result.IsError)
	got, ok := structured.(*models.Relation)
	require.True(t, ok)
	assert.Equal(t, "rel-existing", got.ID)
}

func TestLinkResources_MatrixViolationIsError(t *testing.T) {
	srv, _ := newDeleteResourceTestServer(t)
	relSvc := servicesmocks.NewMockRelationServiceInterface(t)
	setRelationMock(srv, relSvc)

	relSvc.EXPECT().Create(mock.Anything, testMemberUserID, testTeamUUID, mock.Anything).
		Return(nil, false, services.ErrRelationInvalidType)

	result, structured, err := srv.linkResources(
		context.Background(), nil, linkParams(models.RelationTypeGovernedBy), testMemberUserID)

	require.NoError(t, err)
	require.Nil(t, structured)
	require.True(t, result.IsError)
	assert.Contains(t, extractText(t, result), "not allowed")
}

func TestLinkResources_NonMemberTeamDenied(t *testing.T) {
	srv, _ := newDeleteResourceTestServer(t)
	relSvc := servicesmocks.NewMockRelationServiceInterface(t)
	setRelationMock(srv, relSvc)

	params := linkParams(models.RelationTypeGovernedBy)
	params.TeamID = testOtherTeamUUID

	result, structured, err := srv.linkResources(context.Background(), nil, params, testMemberUserID)

	require.NoError(t, err)
	assert.Nil(t, structured)
	assertGenericAccessDenied(t, result)
	relSvc.AssertNotCalled(t, "Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// get_resource(memory) embeds the depth-1 related neighborhood.
func TestGetResource_MemoryIncludesRelated(t *testing.T) {
	srv, m := newDeleteResourceTestServer(t)
	relSvc := servicesmocks.NewMockRelationServiceInterface(t)
	setRelationMock(srv, relSvc)

	m.memory.On("GetMemory", testMemberUserID, testTeamUUID, "mem-1").
		Return(&models.Memory{ID: "mem-1", Text: "why we chose pgvector"}, nil)
	relSvc.EXPECT().ListByResource(
		mock.Anything, testMemberUserID, testTeamUUID, models.RelationResourceTypeMemory, "mem-1", 1, relatedOnReadCap,
	).Return(&models.RelationListResponse{
		Related: []models.RelatedResource{{
			RelationID: "r1", RelationType: models.RelationTypeExplainedBy,
			Direction: models.RelationDirectionIncoming, Origin: models.RelationOriginAI,
			Status: models.RelationStatusConfirmed, ResourceType: models.RelationResourceTypeArtifact,
			ResourceID: "art-9", Title: "Vector search benchmark",
		}},
		TotalCount: 1,
	}, nil)

	params := &GetResourceParams{TeamID: testTeamUUID, ResourceType: "memory", ID: "mem-1"}
	result, structured, err := srv.getResource(context.Background(), nil, params, testMemberUserID)

	require.NoError(t, err)
	require.False(t, result.IsError)
	mem, ok := structured.(*models.Memory)
	require.True(t, ok)
	require.Len(t, []models.RelatedResource(mem.Related), 1)
	assert.Equal(t, "Vector search benchmark", mem.Related[0].Title)
	assert.True(t, strings.Contains(extractText(t, result), "Vector search benchmark"))
}
