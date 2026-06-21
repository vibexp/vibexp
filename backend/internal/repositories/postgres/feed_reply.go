package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// FeedItemReplyRepository implements repositories.FeedItemReplyRepository for PostgreSQL
type FeedItemReplyRepository struct {
	db *database.DB
}

// NewFeedItemReplyRepository creates a new FeedItemReplyRepository
func NewFeedItemReplyRepository(db *database.DB) repositories.FeedItemReplyRepository {
	return &FeedItemReplyRepository{db: db}
}

// CreateReply inserts a new reply into the database, enforcing team membership via EXISTS subquery
func (r *FeedItemReplyRepository) CreateReply(
	ctx context.Context, reply *models.FeedItemReply,
) (*models.FeedItemReply, error) {
	query := `
		INSERT INTO feed_item_replies
			(team_id, feed_item_id, content, posted_by_user_id, ai_assistant_name, posted_at)
		SELECT $1, $2, $3, $4, $5, $6
		WHERE (
			EXISTS (SELECT 1 FROM teams WHERE id = $1 AND owner_id = $4)
			OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $1 AND user_id = $4)
		)
		RETURNING id, posted_at
	`

	now := time.Now()
	err := r.db.QueryRowContext(ctx, query,
		reply.TeamID, reply.FeedItemID, reply.Content, reply.PostedByUserID, reply.AIAssistantName, now,
	).Scan(&reply.ID, &reply.PostedAt)

	if err != nil {
		if isFKViolation(err) {
			return nil, fmt.Errorf("feed item, team, or user not found")
		}
		return nil, mapNoRows(
			fmt.Errorf("failed to create feed item reply: %w", err),
			fmt.Errorf("user is not a member of the specified team"),
		)
	}

	return reply, nil
}

// GetReply retrieves a single feed item reply by ID, scoped to the given team and user.
// Enforces team membership at the SQL layer (same defense-in-depth pattern as CreateReply).
func (r *FeedItemReplyRepository) GetReply(
	ctx context.Context, userID, teamID, replyID string,
) (*models.FeedItemReply, error) {
	query := `
		SELECT id, team_id, feed_item_id, content, posted_by_user_id, ai_assistant_name, posted_at
		FROM feed_item_replies
		WHERE team_id = $1 AND id = $2
		AND (
			EXISTS (SELECT 1 FROM teams WHERE id = $1 AND owner_id = $3)
			OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $1 AND user_id = $3)
		)
	`
	var reply models.FeedItemReply
	err := r.db.QueryRowContext(ctx, query, teamID, replyID, userID).Scan(
		&reply.ID, &reply.TeamID, &reply.FeedItemID, &reply.Content,
		&reply.PostedByUserID, &reply.AIAssistantName, &reply.PostedAt,
	)
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get feed item reply: %w", err), repositories.ErrFeedItemReplyNotFound)
	}
	return &reply, nil
}

// GetReplyForPoster retrieves a reply by ID scoped to its posting user
// (posted_by_user_id), mirroring how the embedding pipeline keys the reply's row.
func (r *FeedItemReplyRepository) GetReplyForPoster(
	ctx context.Context, posterUserID, replyID string,
) (*models.FeedItemReply, error) {
	query := `
		SELECT id, team_id, feed_item_id, content, posted_by_user_id, ai_assistant_name, posted_at
		FROM feed_item_replies
		WHERE id = $1 AND posted_by_user_id = $2
	`
	var reply models.FeedItemReply
	err := r.db.QueryRowContext(ctx, query, replyID, posterUserID).Scan(
		&reply.ID, &reply.TeamID, &reply.FeedItemID, &reply.Content,
		&reply.PostedByUserID, &reply.AIAssistantName, &reply.PostedAt,
	)
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get feed item reply (poster): %w", err),
			repositories.ErrFeedItemReplyNotFound,
		)
	}
	return &reply, nil
}

// ListReplyPostersByItemID returns the (reply_id, posted_by_user_id) pairs for every
// reply on feedItemID within teamID. Used to clean up each reply's embedding row
// (keyed by its poster) before a feed item is hard-deleted.
func (r *FeedItemReplyRepository) ListReplyPostersByItemID(
	ctx context.Context, teamID, feedItemID string,
) ([]repositories.FeedItemReplyPoster, error) {
	query := `
		SELECT id, posted_by_user_id
		FROM feed_item_replies
		WHERE team_id = $1 AND feed_item_id = $2
	`

	rows, err := r.db.QueryContext(ctx, query, teamID, feedItemID)
	if err != nil {
		return nil, fmt.Errorf("failed to list reply posters by item ID: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close reply poster rows")
		}
	}()

	posters := make([]repositories.FeedItemReplyPoster, 0)
	for rows.Next() {
		var poster repositories.FeedItemReplyPoster
		if scanErr := rows.Scan(&poster.ReplyID, &poster.PostedByUserID); scanErr != nil {
			return nil, fmt.Errorf("failed to scan reply poster: %w", scanErr)
		}
		posters = append(posters, poster)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate reply posters: %w", err)
	}

	return posters, nil
}

// ListReplies retrieves replies for a feed item with pagination, ordered by newest-first
//
//nolint:funlen // Repository code with necessary complexity for pagination
func (r *FeedItemReplyRepository) ListReplies(
	ctx context.Context, teamID, feedItemID string, page, limit int,
) ([]models.FeedItemReply, int, error) {
	countQuery := `
		SELECT COUNT(*)
		FROM feed_item_replies
		WHERE team_id = $1 AND feed_item_id = $2
	`
	var totalCount int
	if err := r.db.QueryRowContext(ctx, countQuery, teamID, feedItemID).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("failed to count feed item replies: %w", err)
	}

	offset := (page - 1) * limit
	query := `
		SELECT id, team_id, feed_item_id, content, posted_by_user_id, ai_assistant_name, posted_at
		FROM feed_item_replies
		WHERE team_id = $1 AND feed_item_id = $2
		ORDER BY posted_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.db.QueryContext(ctx, query, teamID, feedItemID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list feed item replies: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close feed item reply rows")
		}
	}()

	replies := make([]models.FeedItemReply, 0)
	for rows.Next() {
		var reply models.FeedItemReply
		if scanErr := rows.Scan(
			&reply.ID, &reply.TeamID, &reply.FeedItemID, &reply.Content,
			&reply.PostedByUserID, &reply.AIAssistantName, &reply.PostedAt,
		); scanErr != nil {
			return nil, 0, fmt.Errorf("failed to scan feed item reply: %w", scanErr)
		}
		replies = append(replies, reply)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate feed item replies: %w", err)
	}

	return replies, totalCount, nil
}

// CountRepliesByItemIDs returns a map of feed_item_id -> reply count for the given item IDs
func (r *FeedItemReplyRepository) CountRepliesByItemIDs(
	ctx context.Context, teamID string, itemIDs []string,
) (map[string]int, error) {
	if len(itemIDs) == 0 {
		return make(map[string]int), nil
	}

	query := `
		SELECT feed_item_id, COUNT(*)
		FROM feed_item_replies
		WHERE team_id = $1 AND feed_item_id = ANY($2)
		GROUP BY feed_item_id
	`

	rows, err := r.db.QueryContext(ctx, query, teamID, pq.Array(itemIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to count replies by item IDs: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close reply count rows")
		}
	}()

	counts := make(map[string]int, len(itemIDs))
	for rows.Next() {
		var feedItemID string
		var count int
		if scanErr := rows.Scan(&feedItemID, &count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan reply count: %w", scanErr)
		}
		counts[feedItemID] = count
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate reply counts: %w", err)
	}

	return counts, nil
}

// CountAll counts all feed item replies accessible to the user across all their teams.
// It uses team-membership scoped SQL to count replies in teams the user owns or belongs to.
func (r *FeedItemReplyRepository) CountAll(ctx context.Context, userID string) (int, error) {
	query := `
		SELECT COUNT(DISTINCT fir.id) FROM feed_item_replies fir
		WHERE (
			EXISTS (SELECT 1 FROM teams WHERE id = fir.team_id AND owner_id = $1)
			OR EXISTS (SELECT 1 FROM team_members WHERE team_id = fir.team_id AND user_id = $1)
		)
	`

	var count int
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count feed item replies: %w", err)
	}

	return count, nil
}
