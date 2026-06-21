package server

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

// Canonical fixtures used across team-scoped MCP tool tests.
const (
	testMemberUserID  = "user-member"
	testTeamUUID      = "550e8400-e29b-41d4-a716-446655440000"
	testTeamSlug      = "acme-team"
	testOtherTeamUUID = "550e8400-e29b-41d4-a716-44665544ffff"
	testOtherTeamSlug = "other-team"
)

// newServerWithNullLogger builds a *Server with a silent logger for tests.
func newServerWithNullLogger(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	return New("8080", nil, "test-api-key", cfg, logger)
}

// stubUserTeams configures a MockTeamServiceInterface so ListTeams returns the
// given teams for testMemberUserID (single page). This is the only dependency
// resolveTeam has, so it is enough to drive membership resolution in tool tests.
// It is marked optional (.Maybe()) because some tool tests (e.g. missing team_id)
// reject before resolveTeam reaches the team listing.
func stubUserTeams(m *mocks.MockTeamServiceInterface, teams []models.Team) {
	m.On("ListTeams", mock.Anything, testMemberUserID, mock.AnythingOfType("int"), mock.AnythingOfType("int")).
		Return(&models.TeamListResponse{
			Teams:      teams,
			TotalCount: len(teams),
			Page:       1,
			PageSize:   resolveTeamPageSize,
		}, nil).Maybe()
}

// memberTeam returns the single team the member user belongs to.
func memberTeam() models.Team {
	return models.Team{ID: testTeamUUID, Name: "Acme Team", Slug: testTeamSlug}
}

func TestResolveTeam_MatchesByUUID(t *testing.T) {
	srv := newServerWithNullLogger(t)
	mockTeam := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{TeamServiceMock: mockTeam}
	stubUserTeams(mockTeam, []models.Team{memberTeam()})

	teamID, errResult := srv.resolveTeam(context.Background(), testMemberUserID, testTeamUUID)

	assert.Nil(t, errResult)
	assert.Equal(t, testTeamUUID, teamID)
}

func TestResolveTeam_MatchesBySlug(t *testing.T) {
	srv := newServerWithNullLogger(t)
	mockTeam := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{TeamServiceMock: mockTeam}
	stubUserTeams(mockTeam, []models.Team{memberTeam()})

	teamID, errResult := srv.resolveTeam(context.Background(), testMemberUserID, testTeamSlug)

	assert.Nil(t, errResult)
	assert.Equal(t, testTeamUUID, teamID, "slug must resolve to the canonical UUID")
}

func TestResolveTeam_NonMemberDenied(t *testing.T) {
	srv := newServerWithNullLogger(t)
	mockTeam := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{TeamServiceMock: mockTeam}
	// The user only belongs to their own team; a different team's UUID must not resolve.
	stubUserTeams(mockTeam, []models.Team{memberTeam()})

	teamID, errResult := srv.resolveTeam(context.Background(), testMemberUserID, testOtherTeamUUID)

	require.NotNil(t, errResult)
	assert.True(t, errResult.IsError)
	assert.Empty(t, teamID)
	assertGenericAccessDenied(t, errResult)
}

func TestResolveTeam_NonMemberSlugDenied(t *testing.T) {
	srv := newServerWithNullLogger(t)
	mockTeam := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{TeamServiceMock: mockTeam}
	stubUserTeams(mockTeam, []models.Team{memberTeam()})

	teamID, errResult := srv.resolveTeam(context.Background(), testMemberUserID, testOtherTeamSlug)

	require.NotNil(t, errResult)
	assert.True(t, errResult.IsError)
	assert.Empty(t, teamID)
	assertGenericAccessDenied(t, errResult)
}

func TestResolveTeam_EmptyIdentifier(t *testing.T) {
	srv := newServerWithNullLogger(t)
	mockTeam := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{TeamServiceMock: mockTeam}

	teamID, errResult := srv.resolveTeam(context.Background(), testMemberUserID, "  ")

	require.NotNil(t, errResult)
	assert.True(t, errResult.IsError)
	assert.Empty(t, teamID)
	text := extractText(t, errResult)
	assert.Contains(t, text, "team_id is required")
	assert.Contains(t, text, "vibexp_io_list_teams")
	// Membership lookup must not even be attempted for empty input.
	mockTeam.AssertNotCalled(t, "ListTeams")
}

func TestResolveTeam_ListError_GenericDenied(t *testing.T) {
	srv := newServerWithNullLogger(t)
	mockTeam := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{TeamServiceMock: mockTeam}
	mockTeam.On("ListTeams", mock.Anything, testMemberUserID, mock.AnythingOfType("int"), mock.AnythingOfType("int")).
		Return(nil, errors.New("db error"))

	teamID, errResult := srv.resolveTeam(context.Background(), testMemberUserID, testTeamUUID)

	require.NotNil(t, errResult)
	assert.True(t, errResult.IsError)
	assert.Empty(t, teamID)
	assertGenericAccessDenied(t, errResult)
}

// assertGenericAccessDenied verifies the result is the generic, anti-enumeration
// access-denied message that nudges the model to list_teams and does not reveal
// whether the team exists.
func assertGenericAccessDenied(t *testing.T, res *mcp.CallToolResult) {
	t.Helper()
	text := extractText(t, res)
	assert.Contains(t, text, "Access denied")
	assert.Contains(t, text, "vibexp_io_list_teams")
}
