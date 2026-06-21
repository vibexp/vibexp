package notifications

import (
	"context"
	"fmt"

	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// RecipientResolverInterface resolves notification recipients from domain events
type RecipientResolverInterface interface {
	// ResolveForEvent returns (recipientUserIDs, teamID, error).
	// The author of the event is excluded from the recipient list.
	ResolveForEvent(ctx context.Context, event events.Event) ([]string, string, error)
}

// RecipientResolver resolves notification recipients from domain events using team membership
type RecipientResolver struct {
	teamMemberRepo repositories.TeamMemberRepository
}

// NewRecipientResolver creates a new RecipientResolver
func NewRecipientResolver(teamMemberRepo repositories.TeamMemberRepository) *RecipientResolver {
	return &RecipientResolver{teamMemberRepo: teamMemberRepo}
}

// ResolveForEvent returns the list of recipient user IDs and team ID for the given event.
// For feed_item.created events: all team members excluding the author.
func (r *RecipientResolver) ResolveForEvent(
	ctx context.Context, event events.Event,
) ([]string, string, error) {
	switch event.Type() {
	case events.EventTypeFeedItemCreated:
		return r.resolveForFeedItemCreated(ctx, event)
	default:
		return nil, "", nil
	}
}

func (r *RecipientResolver) resolveForFeedItemCreated(
	ctx context.Context, event events.Event,
) ([]string, string, error) {
	payload, ok := event.Payload().(*events.FeedItemCreatedPayload)
	if !ok {
		return nil, "", fmt.Errorf("unexpected payload type for feed_item.created event")
	}

	members, err := r.teamMemberRepo.GetByTeamID(ctx, payload.TeamID)
	if err != nil {
		return nil, payload.TeamID, fmt.Errorf("get team members for notification: %w", err)
	}

	recipients := make([]string, 0, len(members))
	for _, m := range members {
		if m.UserID != payload.UserID {
			recipients = append(recipients, m.UserID)
		}
	}

	return recipients, payload.TeamID, nil
}
