package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Freshness surfacing on the artifact endpoints (#735). Artifacts stand in for
// all four resource types on the handler side — the attach helper is generic
// and shared, so what is type-specific is only which accessor each call site
// passes, which the service-layer tests pin per type.
//
// The governing constraint is that this change is ADDITIVE: an existing client
// that knows nothing about freshness must see byte-identical responses.

const freshArtRuleID = "990e8400-e29b-41d4-a716-446655440009"

// sampleFreshnessState is the state a stale artifact carries.
func sampleFreshnessState() *models.ResourceFreshnessState {
	return &models.ResourceFreshnessState{
		Status:         models.FreshnessStatusStale,
		Since:          time.Now().UTC().Add(-72 * time.Hour),
		MatchedRuleIDs: models.JSONArray[string]{freshArtRuleID},
		Reason:         models.FreshnessReasonRuleRun,
	}
}

// A stale resource carries its freshness on the detail GET, and the payload
// still conforms to the spec with the new field present.
func TestHandleGetArtifact_StaleCarriesFreshness(t *testing.T) {
	artSvc := servicesmocks.NewMockArtifactServiceInterface(t)
	expectGetArtifact(artSvc)
	relSvc := servicesmocks.NewMockRelationServiceInterface(t)
	relSvc.EXPECT().ListByResource(
		mock.Anything, relArtUserID, relArtTeamID, models.RelationResourceTypeArtifact, "art-1", 1, relatedOnReadCap,
	).Return(&models.RelationListResponse{Related: []models.RelatedResource{}}, nil)

	freshSvc := servicesmocks.NewMockFreshnessServiceInterface(t)
	freshSvc.EXPECT().
		GetResourceFreshness(mock.Anything, relArtTeamID, models.RelationResourceTypeArtifact, "art-1").
		Return(sampleFreshnessState(), nil)

	srv := newArtifactRelatedServer(artSvc, relSvc)
	srv.container.(*MockArtifactContainer).FreshnessServiceMock = freshSvc

	req := getArtifactRelatedRequest()
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	var resp models.Artifact
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.NotNil(t, resp.Freshness)
	assert.Equal(t, models.FreshnessStatusStale, resp.Freshness.Status)
	assert.Equal(t, models.FreshnessReasonRuleRun, resp.Freshness.Reason)
	assert.Equal(t, []string{freshArtRuleID}, []string(resp.Freshness.MatchedRuleIDs))
}

// A FRESH resource omits the field entirely. That is the additive guarantee:
// the JSON is exactly what it was before #735, so no client can be surprised.
func TestHandleGetArtifact_FreshOmitsTheField(t *testing.T) {
	artSvc := servicesmocks.NewMockArtifactServiceInterface(t)
	expectGetArtifact(artSvc)
	relSvc := servicesmocks.NewMockRelationServiceInterface(t)
	relSvc.EXPECT().ListByResource(
		mock.Anything, relArtUserID, relArtTeamID, models.RelationResourceTypeArtifact, "art-1", 1, relatedOnReadCap,
	).Return(&models.RelationListResponse{Related: []models.RelatedResource{}}, nil)

	freshSvc := servicesmocks.NewMockFreshnessServiceInterface(t)
	freshSvc.EXPECT().
		GetResourceFreshness(mock.Anything, relArtTeamID, models.RelationResourceTypeArtifact, "art-1").
		Return(nil, nil)

	srv := newArtifactRelatedServer(artSvc, relSvc)
	srv.container.(*MockArtifactContainer).FreshnessServiceMock = freshSvc

	req := getArtifactRelatedRequest()
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &raw))
	_, present := raw["freshness"]
	assert.False(t, present, "a fresh resource must not carry the key at all, not even as null")
}

// A freshness failure must not fail the read. The badge is advisory; a detail
// GET that 500s because it could not be loaded would be strictly worse than
// one without it.
func TestHandleGetArtifact_FreshnessFailureDoesNotFailTheRead(t *testing.T) {
	artSvc := servicesmocks.NewMockArtifactServiceInterface(t)
	expectGetArtifact(artSvc)
	relSvc := servicesmocks.NewMockRelationServiceInterface(t)
	relSvc.EXPECT().ListByResource(
		mock.Anything, relArtUserID, relArtTeamID, models.RelationResourceTypeArtifact, "art-1", 1, relatedOnReadCap,
	).Return(&models.RelationListResponse{Related: []models.RelatedResource{}}, nil)

	freshSvc := servicesmocks.NewMockFreshnessServiceInterface(t)
	freshSvc.EXPECT().
		GetResourceFreshness(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, assert.AnError)

	srv := newArtifactRelatedServer(artSvc, relSvc)
	srv.container.(*MockArtifactContainer).FreshnessServiceMock = freshSvc

	req := getArtifactRelatedRequest()
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// A container with no freshness service at all — the state of every existing
// handler test — must behave exactly as before.
func TestHandleGetArtifact_NoFreshnessServiceIsUnchanged(t *testing.T) {
	artSvc := servicesmocks.NewMockArtifactServiceInterface(t)
	expectGetArtifact(artSvc)
	relSvc := servicesmocks.NewMockRelationServiceInterface(t)
	relSvc.EXPECT().ListByResource(
		mock.Anything, relArtUserID, relArtTeamID, models.RelationResourceTypeArtifact, "art-1", 1, relatedOnReadCap,
	).Return(&models.RelationListResponse{Related: []models.RelatedResource{}}, nil)

	srv := newArtifactRelatedServer(artSvc, relSvc)

	req := getArtifactRelatedRequest()
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &raw))
	_, present := raw["freshness"]
	assert.False(t, present)
}

// The filter is validated, not ignored. An ignored `?freshness=stail` returns
// the FULL list, which reads as a legitimate answer to the question asked —
// the reason this rejects rather than falling back.
func TestParseFreshnessFilter(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		want      string
		wantOK    bool
		wantsCode int
	}{
		{name: "absent", query: "", want: "", wantOK: true},
		{name: "stale", query: "?freshness=stale", want: "stale", wantOK: true},
		{name: "typo", query: "?freshness=stail", wantOK: false, wantsCode: http.StatusBadRequest},
		{name: "fresh is not a value", query: "?freshness=fresh", wantOK: false, wantsCode: http.StatusBadRequest},
		{name: "empty value", query: "?freshness=", want: "", wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/x/artifacts"+tt.query, nil)
			rr := httptest.NewRecorder()

			got, ok := parseFreshnessFilter(rr, req)

			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
			if !tt.wantOK {
				assert.Equal(t, tt.wantsCode, rr.Code)
			}
		})
	}
}

// The page attach must issue ONE lookup for the whole page and key each item on
// its own id — a transposition here would label resources with each other's
// staleness, which no other test would notice.
func TestAttachPageFreshness_KeysEachItemOnItsOwnID(t *testing.T) {
	freshSvc := servicesmocks.NewMockFreshnessServiceInterface(t)
	freshSvc.EXPECT().
		ListResourceFreshness(mock.Anything, relArtTeamID, models.RelationResourceTypeArtifact,
			[]string{"art-1", "art-2", "art-3"}).
		Return(map[string]*models.ResourceFreshnessState{
			"art-2": sampleFreshnessState(),
		}, nil).Once()

	srv := newArtifactRelatedServer(
		servicesmocks.NewMockArtifactServiceInterface(t), servicesmocks.NewMockRelationServiceInterface(t))
	srv.container.(*MockArtifactContainer).FreshnessServiceMock = freshSvc

	artifacts := []models.Artifact{{ID: "art-1"}, {ID: "art-2"}, {ID: "art-3"}}
	attachPageFreshness(srv, t.Context(), relArtTeamID, models.RelationResourceTypeArtifact,
		artifacts, artifactID, setArtifactFreshness)

	assert.Nil(t, artifacts[0].Freshness)
	require.NotNil(t, artifacts[1].Freshness, "only the stale one is labelled")
	assert.Equal(t, models.FreshnessStatusStale, artifacts[1].Freshness.Status)
	assert.Nil(t, artifacts[2].Freshness)
}

// An empty page must not issue a query at all — the t-bound mock fails if it does.
func TestAttachPageFreshness_EmptyPageIssuesNoQuery(t *testing.T) {
	freshSvc := servicesmocks.NewMockFreshnessServiceInterface(t)
	srv := newArtifactRelatedServer(
		servicesmocks.NewMockArtifactServiceInterface(t), servicesmocks.NewMockRelationServiceInterface(t))
	srv.container.(*MockArtifactContainer).FreshnessServiceMock = freshSvc

	attachPageFreshness(srv, t.Context(), relArtTeamID, models.RelationResourceTypeArtifact,
		[]models.Artifact{}, artifactID, setArtifactFreshness)
}

// A page-level failure leaves every item unlabelled rather than failing the list.
func TestAttachPageFreshness_FailureLeavesPageUnlabelled(t *testing.T) {
	freshSvc := servicesmocks.NewMockFreshnessServiceInterface(t)
	freshSvc.EXPECT().
		ListResourceFreshness(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, assert.AnError).Once()

	srv := newArtifactRelatedServer(
		servicesmocks.NewMockArtifactServiceInterface(t), servicesmocks.NewMockRelationServiceInterface(t))
	srv.container.(*MockArtifactContainer).FreshnessServiceMock = freshSvc

	artifacts := []models.Artifact{{ID: "art-1"}}
	attachPageFreshness(srv, t.Context(), relArtTeamID, models.RelationResourceTypeArtifact,
		artifacts, artifactID, setArtifactFreshness)

	assert.Nil(t, artifacts[0].Freshness)
}
