package notifications_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/pkg/events"
)

func TestRecipientResolver_ResolveForEvent_FeedItemCreated(t *testing.T) {
	ctx := context.Background()

	teamMemberRepo := new(repomocks.MockTeamMemberRepository)

	resolver := notifications.NewRecipientResolver(teamMemberRepo)

	teamID := "team-abc"
	authorID := "author-user"
	member1 := "member-user-1"
	member2 := "member-user-2"

	teamMemberRepo.On("GetByTeamID", ctx, teamID).Return([]models.TeamMember{
		{UserID: authorID, TeamID: teamID, Role: models.TeamMemberRoleMember},
		{UserID: member1, TeamID: teamID, Role: models.TeamMemberRoleMember},
		{UserID: member2, TeamID: teamID, Role: models.TeamMemberRoleOwner},
	}, nil)

	event := events.NewFeedItemCreatedEvent("item-1", authorID, teamID, "feed-1", "", "", "", time.Now())

	recipients, gotTeamID, err := resolver.ResolveForEvent(ctx, event)

	require.NoError(t, err)
	assert.Equal(t, teamID, gotTeamID)
	assert.Len(t, recipients, 2)
	assert.Contains(t, recipients, member1)
	assert.Contains(t, recipients, member2)
	assert.NotContains(t, recipients, authorID, "author should be excluded")

	teamMemberRepo.AssertExpectations(t)
}

func TestRecipientResolver_ResolveForEvent_OnlyAuthorInTeam(t *testing.T) {
	ctx := context.Background()

	teamMemberRepo := new(repomocks.MockTeamMemberRepository)
	resolver := notifications.NewRecipientResolver(teamMemberRepo)

	teamID := "team-solo"
	authorID := "solo-author"

	teamMemberRepo.On("GetByTeamID", ctx, teamID).Return([]models.TeamMember{
		{UserID: authorID, TeamID: teamID},
	}, nil)

	event := events.NewFeedItemCreatedEvent("item-1", authorID, teamID, "feed-1", "", "", "", time.Now())

	recipients, gotTeamID, err := resolver.ResolveForEvent(ctx, event)

	require.NoError(t, err)
	assert.Equal(t, teamID, gotTeamID)
	assert.Empty(t, recipients, "no recipients when author is the only team member")
}

func TestRecipientResolver_ResolveForEvent_UnknownEvent(t *testing.T) {
	ctx := context.Background()

	teamMemberRepo := new(repomocks.MockTeamMemberRepository)
	resolver := notifications.NewRecipientResolver(teamMemberRepo)

	event := events.NewUserCreatedEvent("user-1", "user@example.com", "Test User", time.Now())

	recipients, teamID, err := resolver.ResolveForEvent(ctx, event)

	require.NoError(t, err)
	assert.Empty(t, recipients)
	assert.Empty(t, teamID)
	teamMemberRepo.AssertNotCalled(t, mock.Anything)
}
