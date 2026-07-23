package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Masterminds/squirrel"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// feedsTable is the aliased FROM target shared by the feed list queries.
const feedsTable = "feeds f"

// feedListColumns is the 7-column projection shared by the feed list queries
// (queryList's SELECT and queryListWithLastPost's SELECT and GROUP BY).
var feedListColumns = []string{
	"f.id", "f.team_id", "f.name", "f.description",
	"f.created_by_user_id", "f.created_at", "f.updated_at",
}

// FeedRepository implements repositories.FeedRepository for PostgreSQL
type FeedRepository struct {
	db *database.DB
}

// NewFeedRepository creates a new FeedRepository
func NewFeedRepository(db *database.DB) repositories.FeedRepository {
	return &FeedRepository{db: db}
}

// Create inserts a new feed into the database
func (r *FeedRepository) Create(ctx context.Context, feed *models.Feed) error {
	query := `
		INSERT INTO feeds (team_id, name, description, created_by_user_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		feed.TeamID, feed.Name, feed.Description, feed.CreatedByUserID,
		feed.CreatedAt, feed.UpdatedAt,
	).Scan(&feed.ID, &feed.CreatedAt, &feed.UpdatedAt)

	if err != nil {
		if uniqueViolation(err) != nil {
			return fmt.Errorf("feed with name '%s' already exists for this team", feed.Name)
		}
		if isFKViolation(err) {
			return fmt.Errorf("team or user not found")
		}
		return fmt.Errorf("failed to create feed: %w", err)
	}

	return nil
}

// GetByID retrieves a feed by ID, enforcing team membership via EXISTS subquery
func (r *FeedRepository) GetByID(ctx context.Context, userID, teamID, feedID string) (*models.Feed, error) {
	query := `
		SELECT f.id, f.team_id, f.name, f.description, f.created_by_user_id, f.created_at, f.updated_at
		FROM feeds f
		WHERE f.id = $1
			AND f.team_id = $2
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
			)
	`

	var feed models.Feed
	err := r.db.QueryRowContext(ctx, query, feedID, teamID, userID).Scan(
		&feed.ID, &feed.TeamID, &feed.Name, &feed.Description,
		&feed.CreatedByUserID, &feed.CreatedAt, &feed.UpdatedAt,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get feed by ID: %w", err), repositories.ErrFeedNotFound)
	}

	return &feed, nil
}

// buildFeedListWhereClause builds the shared WHERE conditions for the feed list
// queries. List, its count query, and ListWithLastPost all consume the same
// conditions, so they can never diverge. EXISTS subqueries avoid a Cartesian
// product with multi-member teams.
func buildFeedListWhereClause(userID string, filters repositories.FeedFilters) squirrel.Sqlizer {
	teamID := filters.TeamID
	where := squirrel.And{
		squirrel.Eq{"f.team_id": teamID},
		teamReadAccess(teamID, userID),
	}

	if filters.Search != "" {
		searchTerm := "%" + filters.Search + "%"
		where = append(where, squirrel.Expr(
			"(f.name ILIKE ? OR f.description ILIKE ?)",
			searchTerm, searchTerm,
		))
	}

	return where
}

// List retrieves feeds with filtering and pagination, enforcing team membership
func (r *FeedRepository) List(
	ctx context.Context, userID string, filters repositories.FeedFilters,
) ([]models.Feed, int, error) {
	if filters.TeamID == "" {
		return nil, 0, fmt.Errorf("TeamID is required but was empty")
	}

	where := buildFeedListWhereClause(userID, filters)

	totalCount, err := r.countList(ctx, where)
	if err != nil {
		return nil, 0, err
	}

	feeds, err := r.queryList(ctx, where, filters)
	if err != nil {
		return nil, 0, err
	}

	return feeds, totalCount, nil
}

// countList counts feeds matching the shared WHERE conditions used by List, so
// the count and page queries can never diverge. COUNT(*) is safe because the
// EXISTS subqueries (rather than a JOIN) eliminate multi-member duplicates.
func (r *FeedRepository) countList(ctx context.Context, where squirrel.Sqlizer) (int, error) {
	query, args, err := psql.
		Select("COUNT(*)").
		From(feedsTable).
		Where(where).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build feeds count query: %w", err)
	}

	var totalCount int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); err != nil {
		return 0, fmt.Errorf("failed to count feeds: %w", err)
	}

	return totalCount, nil
}

// feedLimitOffset clamps the paging inputs to the non-negative values squirrel's
// unsigned Limit/Offset require. A non-positive Limit emits LIMIT 0 (empty page),
// matching the previous hand-built query. Callers must pass Limit >= 1 and
// Page >= 1 for a populated page.
func feedLimitOffset(page, limit int) (uint64, uint64) {
	limitOut := uint64(0)
	if limit > 0 {
		limitOut = uint64(limit)
	}
	offsetOut := uint64(0)
	if page > 1 && limit > 0 {
		if rawOffset := (page - 1) * limit; rawOffset > 0 {
			offsetOut = uint64(rawOffset)
		}
	}
	return limitOut, offsetOut
}

// queryList runs the paginated page query for List using the shared WHERE
// conditions, so it can never diverge from the count query.
func (r *FeedRepository) queryList(
	ctx context.Context, where squirrel.Sqlizer, filters repositories.FeedFilters,
) ([]models.Feed, error) {
	limit, offset := feedLimitOffset(filters.Page, filters.Limit)

	query, args, err := psql.
		Select(feedListColumns...).
		From(feedsTable).
		Where(where).
		OrderBy("f.created_at DESC").
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build feeds list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list feeds: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close feed rows", "error", closeErr)
		}
	}()

	feeds := make([]models.Feed, 0)
	for rows.Next() {
		var feed models.Feed
		if scanErr := rows.Scan(
			&feed.ID, &feed.TeamID, &feed.Name, &feed.Description,
			&feed.CreatedByUserID, &feed.CreatedAt, &feed.UpdatedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan feed: %w", scanErr)
		}
		feeds = append(feeds, feed)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate feeds: %w", err)
	}

	return feeds, nil
}

// ListWithLastPost retrieves feeds enriched with MAX(posted_at) from feed_items via a LEFT JOIN.
// It is used exclusively by the MCP list-feeds tool to avoid N+1 queries.
func (r *FeedRepository) ListWithLastPost(
	ctx context.Context, userID string, filters repositories.FeedFilters,
) ([]models.FeedWithLastPost, error) {
	if filters.TeamID == "" {
		return nil, fmt.Errorf("TeamID is required but was empty")
	}

	where := buildFeedListWhereClause(userID, filters)

	return r.queryListWithLastPost(ctx, where, filters)
}

// queryListWithLastPost runs the LEFT JOIN page query for ListWithLastPost using
// the shared WHERE conditions. The GROUP BY collapses the per-item join rows back
// to one row per feed, and MAX(fi.posted_at) yields the most recent active post.
func (r *FeedRepository) queryListWithLastPost(
	ctx context.Context, where squirrel.Sqlizer, filters repositories.FeedFilters,
) ([]models.FeedWithLastPost, error) {
	limit, offset := feedLimitOffset(filters.Page, filters.Limit)

	query, args, err := psql.
		Select(feedListColumns...).
		Column("MAX(fi.posted_at) AS last_post_at").
		From(feedsTable).
		LeftJoin("feed_items fi ON fi.feed_id = f.id AND fi.archived_at IS NULL").
		Where(where).
		GroupBy(feedListColumns...).
		OrderBy("f.created_at DESC").
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build feeds with last post list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list feeds with last post: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close feed rows", "error", closeErr)
		}
	}()

	feeds := make([]models.FeedWithLastPost, 0)
	for rows.Next() {
		var f models.FeedWithLastPost
		if scanErr := rows.Scan(
			&f.ID, &f.TeamID, &f.Name, &f.Description,
			&f.CreatedByUserID, &f.CreatedAt, &f.UpdatedAt,
			&f.LastPostAt,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan feed with last post: %w", scanErr)
		}
		feeds = append(feeds, f)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate feeds with last post: %w", err)
	}

	return feeds, nil
}

// Update updates a feed, enforcing team membership via EXISTS subquery
func (r *FeedRepository) Update(ctx context.Context, feed *models.Feed) error {
	query := `
		UPDATE feeds
		SET name = $3, description = $4, updated_at = $5
		WHERE id = $1
			AND team_id = $2
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $6)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $6)
			)
		RETURNING updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		feed.ID, feed.TeamID, feed.Name, feed.Description, feed.UpdatedAt, feed.CreatedByUserID,
	).Scan(&feed.UpdatedAt)

	if err != nil {
		if uniqueViolation(err) != nil {
			return fmt.Errorf("feed with name '%s' already exists for this team", feed.Name)
		}
		return mapNoRows(fmt.Errorf("failed to update feed: %w", err), repositories.ErrFeedNotFound)
	}

	return nil
}

// Delete deletes a feed, enforcing team membership via EXISTS subquery
func (r *FeedRepository) Delete(ctx context.Context, userID, teamID, feedID string) error {
	query := `
		DELETE FROM feeds
		WHERE id = $1
			AND team_id = $2
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
			)
	`

	result, err := r.db.ExecContext(ctx, query, feedID, teamID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete feed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrFeedNotFound
	}

	return nil
}

// CountAll counts all feeds accessible to the user across all their teams.
// It uses team-membership scoped SQL to count feeds in teams the user owns or belongs to.
func (r *FeedRepository) CountAll(ctx context.Context, userID string) (int, error) {
	query := `
		SELECT COUNT(DISTINCT f.id) FROM feeds f
		WHERE (
			EXISTS (SELECT 1 FROM teams WHERE id = f.team_id AND owner_id = $1)
			OR EXISTS (SELECT 1 FROM team_members WHERE team_id = f.team_id AND user_id = $1)
		)
	`

	var count int
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count feeds: %w", err)
	}

	return count, nil
}

// FeedItemRepository implements repositories.FeedItemRepository for PostgreSQL
type FeedItemRepository struct {
	db *database.DB
}

// NewFeedItemRepository creates a new FeedItemRepository
func NewFeedItemRepository(db *database.DB) repositories.FeedItemRepository {
	return &FeedItemRepository{db: db}
}

// Create inserts a new feed item into the database
func (r *FeedItemRepository) Create(ctx context.Context, item *models.FeedItem) error {
	query := `
		INSERT INTO feed_items
			(team_id, feed_id, project_id, title, content, excerpt, ai_assistant_name, posted_by_user_id, archived_at, posted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, posted_at
	`

	err := r.db.QueryRowContext(ctx, query,
		item.TeamID, item.FeedID, item.ProjectID, item.Title, item.Content,
		item.Excerpt, item.AIAssistantName, item.PostedByUserID, item.ArchivedAt, item.PostedAt,
	).Scan(&item.ID, &item.PostedAt)

	if err != nil {
		if isFKViolation(err) {
			return fmt.Errorf("feed, team, project, or user not found")
		}
		return fmt.Errorf("failed to create feed item: %w", err)
	}

	return nil
}

// GetByID retrieves a feed item by ID, enforcing team membership via EXISTS subquery
func (r *FeedItemRepository) GetByID(ctx context.Context, userID, teamID, itemID string) (*models.FeedItem, error) {
	query := `
		SELECT fi.id, fi.team_id, fi.feed_id, fi.project_id, fi.title, fi.content,
			fi.excerpt, fi.ai_assistant_name, fi.posted_by_user_id, fi.archived_at, fi.posted_at
		FROM feed_items fi
		WHERE fi.id = $1
			AND fi.team_id = $2
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
			)
	`

	var item models.FeedItem
	err := r.db.QueryRowContext(ctx, query, itemID, teamID, userID).Scan(
		&item.ID, &item.TeamID, &item.FeedID, &item.ProjectID,
		&item.Title, &item.Content, &item.Excerpt, &item.AIAssistantName,
		&item.PostedByUserID, &item.ArchivedAt, &item.PostedAt,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get feed item by ID: %w", err), repositories.ErrFeedItemNotFound)
	}

	return &item, nil
}

// GetByIDForPoster retrieves a feed item by ID scoped to its posting user
// (posted_by_user_id), mirroring how the embedding pipeline keys the item's row.
func (r *FeedItemRepository) GetByIDForPoster(
	ctx context.Context, posterUserID, itemID string,
) (*models.FeedItem, error) {
	query := `
		SELECT fi.id, fi.team_id, fi.feed_id, fi.project_id, fi.title, fi.content,
			fi.excerpt, fi.ai_assistant_name, fi.posted_by_user_id, fi.archived_at, fi.posted_at
		FROM feed_items fi
		WHERE fi.id = $1 AND fi.posted_by_user_id = $2
	`

	var item models.FeedItem
	err := r.db.QueryRowContext(ctx, query, itemID, posterUserID).Scan(
		&item.ID, &item.TeamID, &item.FeedID, &item.ProjectID,
		&item.Title, &item.Content, &item.Excerpt, &item.AIAssistantName,
		&item.PostedByUserID, &item.ArchivedAt, &item.PostedAt,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get feed item by ID (poster): %w", err),
			repositories.ErrFeedItemNotFound,
		)
	}

	return &item, nil
}

// buildFeedItemListWhereClause builds the shared WHERE conditions for the feed
// item list query. Count and page queries consume the same conditions, so they
// can never diverge. EXISTS subqueries avoid a Cartesian product with
// multi-member teams.
func buildFeedItemListWhereClause(userID string, filters repositories.FeedItemFilters) squirrel.Sqlizer {
	teamID := filters.TeamID
	where := squirrel.And{
		squirrel.Eq{"fi.team_id": teamID},
		teamReadAccess(teamID, userID),
	}

	if filters.FeedID != nil && *filters.FeedID != "" {
		where = append(where, squirrel.Eq{"fi.feed_id": *filters.FeedID})
	}

	if filters.ProjectID != nil && *filters.ProjectID != "" {
		where = append(where, squirrel.Eq{"fi.project_id": *filters.ProjectID})
	}

	if filters.AIAssistantName != nil && *filters.AIAssistantName != "" {
		where = append(where, squirrel.Eq{"fi.ai_assistant_name": *filters.AIAssistantName})
	}

	if filters.Search != "" {
		searchTerm := "%" + filters.Search + "%"
		where = append(where, squirrel.Expr(
			"(fi.title ILIKE ? OR fi.content ILIKE ?)",
			searchTerm, searchTerm,
		))
	}

	// Archived filter: nil or false = active only, true = archived only
	if filters.Archived == nil || !*filters.Archived {
		where = append(where, squirrel.Expr("fi.archived_at IS NULL"))
	} else {
		where = append(where, squirrel.Expr("fi.archived_at IS NOT NULL"))
	}

	return where
}

// List retrieves feed items with filtering and pagination, enforcing team membership
func (r *FeedItemRepository) List(
	ctx context.Context, userID string, filters repositories.FeedItemFilters,
) ([]models.FeedItem, int, error) {
	if filters.TeamID == "" {
		return nil, 0, fmt.Errorf("TeamID is required but was empty")
	}

	where := buildFeedItemListWhereClause(userID, filters)

	totalCount, err := r.countList(ctx, where)
	if err != nil {
		return nil, 0, err
	}

	items, err := r.queryList(ctx, where, filters)
	if err != nil {
		return nil, 0, err
	}

	return items, totalCount, nil
}

// countList counts feed items matching the shared WHERE conditions used by List,
// so the count and page queries can never diverge.
func (r *FeedItemRepository) countList(ctx context.Context, where squirrel.Sqlizer) (int, error) {
	query, args, err := psql.
		Select("COUNT(*)").
		From("feed_items fi").
		Where(where).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build feed items count query: %w", err)
	}

	var totalCount int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); err != nil {
		return 0, fmt.Errorf("failed to count feed items: %w", err)
	}

	return totalCount, nil
}

// queryList runs the paginated page query for List using the shared WHERE
// conditions, so it can never diverge from the count query.
func (r *FeedItemRepository) queryList(
	ctx context.Context, where squirrel.Sqlizer, filters repositories.FeedItemFilters,
) ([]models.FeedItem, error) {
	limit, offset := feedLimitOffset(filters.Page, filters.Limit)

	query, args, err := psql.
		Select(
			"fi.id", "fi.team_id", "fi.feed_id", "fi.project_id", "fi.title", "fi.content",
			"fi.excerpt", "fi.ai_assistant_name", "fi.posted_by_user_id", "fi.archived_at", "fi.posted_at",
		).
		From("feed_items fi").
		Where(where).
		OrderBy("fi.posted_at DESC").
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build feed items list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list feed items: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close feed item rows", "error", closeErr)
		}
	}()

	items := make([]models.FeedItem, 0)
	for rows.Next() {
		var item models.FeedItem
		if scanErr := rows.Scan(
			&item.ID, &item.TeamID, &item.FeedID, &item.ProjectID,
			&item.Title, &item.Content, &item.Excerpt, &item.AIAssistantName,
			&item.PostedByUserID, &item.ArchivedAt, &item.PostedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan feed item: %w", scanErr)
		}
		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate feed items: %w", err)
	}

	return items, nil
}

// Archive sets archived_at = NOW() for a feed item, enforcing team membership
func (r *FeedItemRepository) Archive(ctx context.Context, userID, teamID, itemID string) error {
	query := `
		UPDATE feed_items
		SET archived_at = NOW()
		WHERE id = $1
			AND team_id = $2
			AND archived_at IS NULL
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
			)
	`

	result, err := r.db.ExecContext(ctx, query, itemID, teamID, userID)
	if err != nil {
		return fmt.Errorf("failed to archive feed item: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("feed item not found or already archived")
	}

	return nil
}

// Unarchive sets archived_at = NULL for a feed item, enforcing team membership
func (r *FeedItemRepository) Unarchive(ctx context.Context, userID, teamID, itemID string) error {
	query := `
		UPDATE feed_items
		SET archived_at = NULL
		WHERE id = $1
			AND team_id = $2
			AND archived_at IS NOT NULL
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
			)
	`

	result, err := r.db.ExecContext(ctx, query, itemID, teamID, userID)
	if err != nil {
		return fmt.Errorf("failed to unarchive feed item: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("feed item not found or not archived")
	}

	return nil
}

// Delete hard-deletes a feed item, distinguishing a missing item from a denial.
//
// The predicate is TENANCY ONLY (epic #220 decision D3): the existence check
// still tells "no such item in this team" (404) apart from a denial, but WHO may
// delete it is decided by FeedItemService via the authz matrix before this is
// reached — the poster may delete their own, Owner/Admin may delete anyone's as
// moderation.
//
// The former role re-assertion inside the DELETE is gone with it. That guarded a
// SELECT->DELETE race (role revoked between the two queries); D3 explicitly
// accepts losing that atomicity — the window is negligible — in exchange for one
// testable enforcement point instead of two that drift.
func (r *FeedItemRepository) Delete(ctx context.Context, userID, teamID, itemID string) error {
	const existsQuery = `
		SELECT EXISTS (SELECT 1 FROM feed_items WHERE id = $1 AND team_id = $2)
	`

	var itemExists bool
	if err := r.db.QueryRowContext(ctx, existsQuery, itemID, teamID).Scan(&itemExists); err != nil {
		return fmt.Errorf("failed to check feed item existence: %w", err)
	}

	if !itemExists {
		return fmt.Errorf("%w: id=%s team=%s", repositories.ErrFeedItemNotFound, itemID, teamID)
	}

	const deleteQuery = `
		DELETE FROM feed_items
		WHERE id = $1
			AND team_id = $2
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
			)
	`
	result, err := r.db.ExecContext(ctx, deleteQuery, itemID, teamID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete feed item: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		// Race: between the existence check and the DELETE the row was removed or
		// the caller's membership was revoked. Either way the operation did not
		// delete what we intended; return not-found so the client sees a
		// deterministic outcome instead of a silent 204 no-op.
		return fmt.Errorf("%w: id=%s team=%s (concurrent change)", repositories.ErrFeedItemNotFound, itemID, teamID)
	}

	return nil
}

// CountAll counts all feed items (including archived) accessible to the user across all their teams.
// It uses team-membership scoped SQL to count items in teams the user owns or belongs to.
func (r *FeedItemRepository) CountAll(ctx context.Context, userID string) (int, error) {
	query := `
		SELECT COUNT(DISTINCT fi.id) FROM feed_items fi
		WHERE (
			EXISTS (SELECT 1 FROM teams WHERE id = fi.team_id AND owner_id = $1)
			OR EXISTS (SELECT 1 FROM team_members WHERE team_id = fi.team_id AND user_id = $1)
		)
	`

	var count int
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count feed items: %w", err)
	}

	return count, nil
}
