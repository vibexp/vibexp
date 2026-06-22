package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

const (
	// MaxAgentCardJSONSize defines the maximum size for agent card JSON stored in database
	// This prevents memory exhaustion when unmarshaling large JSON objects
	MaxAgentCardJSONSize = 1024 * 1024 // 1MB
)

type agentRepository struct {
	db *database.DB
}

// NewAgentRepository creates a new agent repository
func NewAgentRepository(db *database.DB) repositories.AgentRepository {
	return &agentRepository{db: db}
}

// Create creates a new agent
func (r *agentRepository) Create(ctx context.Context, agent *models.Agent) error {
	logger := contextkeys.GetLoggerFromContext(ctx)

	agent.ID = uuid.New().String()
	agent.CreatedAt = time.Now()
	agent.UpdatedAt = time.Now()
	agent.TotalRuns = 0
	agent.SuccessRate = 0.0

	if agent.Status == "" {
		agent.Status = "active"
	}

	var agentCardJSON []byte
	var err error
	if agent.AgentCard != nil {
		agentCardJSON, err = json.Marshal(agent.AgentCard)
		if err != nil {
			return fmt.Errorf("failed to marshal agent card: %w", err)
		}
	}

	query := `
		INSERT INTO agents
		(id, user_id, team_id, name, description, status, card_url, agent_card,
		total_runs, success_rate, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err = r.db.ExecContext(ctx, query,
		agent.ID, agent.UserID, agent.TeamID, agent.Name, agent.Description, agent.Status,
		agent.CardURL, agentCardJSON, agent.TotalRuns, agent.SuccessRate, agent.CreatedAt, agent.UpdatedAt)

	if err != nil {
		if uniqueViolation(err) != nil {
			return fmt.Errorf("%w (name %q)", repositories.ErrAgentNameConflict, agent.Name)
		}
		logger.With(
			"method", "Create",
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create agent")
		return fmt.Errorf("failed to create agent: %w", err)
	}

	return nil
}

// GetByID retrieves an agent by ID
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
//
//nolint:funlen // Repository code with necessary complexity
func (r *agentRepository) GetByID(ctx context.Context, userID, teamID, agentID string) (*models.Agent, error) {
	logger := contextkeys.GetLoggerFromContext(ctx)

	query := `
		SELECT a.id, a.user_id, a.team_id, a.name, a.description, a.status, a.card_url, a.agent_card,
		a.credentials, a.last_run, a.last_synced_at, a.total_runs, a.success_rate, a.created_at,
		a.updated_at, a.version
		FROM agents a
		WHERE a.id = $1
			AND a.team_id = $2
			AND (
				EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3)
			)
	`

	var agent models.Agent
	var agentCardJSON []byte
	var credentialsJSON []byte
	var cardURL sql.NullString
	var lastRun sql.NullTime
	var lastSyncedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, agentID, teamID, userID).Scan(
		&agent.ID, &agent.UserID, &agent.TeamID, &agent.Name, &agent.Description, &agent.Status,
		&cardURL, &agentCardJSON, &credentialsJSON, &lastRun, &lastSyncedAt, &agent.TotalRuns, &agent.SuccessRate,
		&agent.CreatedAt, &agent.UpdatedAt, &agent.Version)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repositories.ErrAgentNotFound
		}
		logger.With(
			"method", "GetByID",
			"agent_id", agentID,
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get agent by ID")
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	if cardURL.Valid {
		agent.CardURL = &cardURL.String
	}

	if lastRun.Valid {
		agent.LastRun = &lastRun.Time
	}

	if lastSyncedAt.Valid {
		agent.LastSyncedAt = &lastSyncedAt.Time
	}

	if len(agentCardJSON) > 0 {
		// Check size limit before unmarshaling to prevent memory exhaustion
		if len(agentCardJSON) > MaxAgentCardJSONSize {
			logger.With(
				"method", "GetByID",
				"agent_id", agentID,
				"size", len(agentCardJSON),
				"max_size", MaxAgentCardJSONSize,
			).Error("Agent card JSON exceeds maximum allowed size")
			return nil, fmt.Errorf(
				"agent card JSON too large: %d bytes (maximum: %d bytes)",
				len(agentCardJSON), MaxAgentCardJSONSize,
			)
		}

		var agentCard models.AgentCard
		if err := json.Unmarshal(agentCardJSON, &agentCard); err != nil {
			logger.With(
				"method", "GetByID",
				"agent_id", agentID,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to unmarshal agent card")
			return nil, fmt.Errorf("failed to unmarshal agent card: %w", err)
		}
		agent.AgentCard = &agentCard
	}

	if len(credentialsJSON) > 0 {
		var credentials models.AgentCredentials
		if err := json.Unmarshal(credentialsJSON, &credentials); err != nil {
			logger.With(
				"method", "GetByID",
				"agent_id", agentID,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to unmarshal credentials")
			return nil, fmt.Errorf("failed to unmarshal credentials: %w", err)
		}
		agent.Credentials = &credentials
		logger.With(
			"method", "GetByID",
			"agent_id", agentID,
			"has_creds", len(credentials) > 0,
			"creds_count", len(credentials),
		).Debug("Loaded agent credentials")
	} else {
		logger.With(
			"method", "GetByID",
			"agent_id", agentID,
		).Debug("No credentials found for agent")
	}

	return &agent, nil
}

// GetByIDCrossTeam retrieves an agent by ID across all user's teams
//
//nolint:funlen // Repository code with necessary complexity
func (r *agentRepository) GetByIDCrossTeam(ctx context.Context, userID, agentID string) (*models.Agent, error) {
	logger := contextkeys.GetLoggerFromContext(ctx)

	query := `
		SELECT id, user_id, team_id, name, description, status, card_url, agent_card, credentials, last_run,
		last_synced_at, total_runs, success_rate, created_at, updated_at, version
		FROM agents
		WHERE id = $1 AND user_id = $2
	`

	var agent models.Agent
	var agentCardJSON []byte
	var credentialsJSON []byte
	var cardURL sql.NullString
	var lastRun sql.NullTime
	var lastSyncedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, agentID, userID).Scan(
		&agent.ID, &agent.UserID, &agent.TeamID, &agent.Name, &agent.Description, &agent.Status,
		&cardURL, &agentCardJSON, &credentialsJSON, &lastRun, &lastSyncedAt, &agent.TotalRuns, &agent.SuccessRate,
		&agent.CreatedAt, &agent.UpdatedAt, &agent.Version)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repositories.ErrAgentNotFound
		}
		logger.With(
			"method", "GetByIDCrossTeam",
			"agent_id", agentID,
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get agent by ID (cross-team)")
		return nil, fmt.Errorf("failed to get agent (cross-team): %w", err)
	}

	if cardURL.Valid {
		agent.CardURL = &cardURL.String
	}

	if lastRun.Valid {
		agent.LastRun = &lastRun.Time
	}

	if lastSyncedAt.Valid {
		agent.LastSyncedAt = &lastSyncedAt.Time
	}

	if len(agentCardJSON) > 0 {
		// Check size limit before unmarshaling to prevent memory exhaustion
		if len(agentCardJSON) > MaxAgentCardJSONSize {
			logger.With(
				"method", "GetByIDCrossTeam",
				"agent_id", agentID,
				"size", len(agentCardJSON),
				"max_size", MaxAgentCardJSONSize,
			).Error("Agent card JSON exceeds maximum allowed size")
			return nil, fmt.Errorf(
				"agent card JSON too large: %d bytes (maximum: %d bytes)",
				len(agentCardJSON), MaxAgentCardJSONSize,
			)
		}

		var agentCard models.AgentCard
		if err := json.Unmarshal(agentCardJSON, &agentCard); err != nil {
			logger.With(
				"method", "GetByIDCrossTeam",
				"agent_id", agentID,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to unmarshal agent card")
			return nil, fmt.Errorf("failed to unmarshal agent card: %w", err)
		}
		agent.AgentCard = &agentCard
	}

	if len(credentialsJSON) > 0 {
		var credentials models.AgentCredentials
		if err := json.Unmarshal(credentialsJSON, &credentials); err != nil {
			logger.With(
				"method", "GetByIDCrossTeam",
				"agent_id", agentID,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to unmarshal credentials")
			return nil, fmt.Errorf("failed to unmarshal credentials: %w", err)
		}
		agent.Credentials = &credentials
		logger.With(
			"method", "GetByIDCrossTeam",
			"agent_id", agentID,
			"has_creds", len(credentials) > 0,
			"creds_count", len(credentials),
		).Debug("Loaded agent credentials")
	} else {
		logger.With(
			"method", "GetByIDCrossTeam",
			"agent_id", agentID,
		).Debug("No credentials found for agent")
	}

	return &agent, nil
}

// buildAgentListOrderByClause builds the ORDER BY clause for the agent list
// query using an allowlist of sortable columns. Any SortBy outside the allowlist
// falls back to the default. The returned string is already an `a.`-prefixed
// column expression, so the caller passes the result straight to OrderBy without
// an additional prefix.
func buildAgentListOrderByClause(filters repositories.AgentFilters) string {
	orderBy := "a.updated_at DESC"
	if filters.SortBy == "" {
		return orderBy
	}
	direction := "DESC"
	if filters.SortOrder == "asc" {
		direction = "ASC"
	}
	switch filters.SortBy {
	case "name", "status", "updated_at", "created_at", "last_run", "success_rate":
		return fmt.Sprintf("a.%s %s", filters.SortBy, direction)
	}
	return orderBy
}

// buildAgentListWhereClause builds the shared WHERE conditions for the agent
// list query. Count and page queries consume the same conditions, so they can
// never diverge. EXISTS subqueries avoid a Cartesian product with multi-member
// teams. The team-access predicate intentionally binds teamID/userID twice each
// (one pair per EXISTS branch) — squirrel emits a positional placeholder per
// argument rather than reusing them.
func buildAgentListWhereClause(userID string, filters repositories.AgentFilters) squirrel.Sqlizer {
	teamID := filters.TeamID
	where := squirrel.And{
		squirrel.Eq{"a.team_id": teamID},
		teamReadAccess(teamID, userID),
	}

	if filters.Status != "" {
		where = append(where, squirrel.Eq{"a.status": filters.Status})
	}

	if filters.Search != "" {
		where = append(where, squirrel.Expr(
			"(a.name ILIKE ? OR a.description ILIKE ?)",
			"%"+filters.Search+"%", "%"+filters.Search+"%",
		))
	}

	return where
}

// List retrieves agents with filtering and pagination
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *agentRepository) List(
	ctx context.Context, userID string, filters repositories.AgentFilters,
) ([]models.Agent, int, error) {
	// Validate required TeamID
	if filters.TeamID == "" {
		return nil, 0, fmt.Errorf("TeamID is required but was empty")
	}

	// Shared WHERE conditions guarantee the count and page queries can never diverge.
	where := buildAgentListWhereClause(userID, filters)

	totalCount, err := r.countList(ctx, where)
	if err != nil {
		return nil, 0, err
	}

	agents, err := r.queryList(ctx, where, filters)
	if err != nil {
		return nil, 0, err
	}

	return agents, totalCount, nil
}

// countList counts agents matching the shared WHERE conditions used by List, so
// the count and page queries can never diverge. COUNT(*) is safe because the
// EXISTS subqueries (rather than a JOIN) eliminate multi-member duplicates.
func (r *agentRepository) countList(ctx context.Context, where squirrel.Sqlizer) (int, error) {
	logger := contextkeys.GetLoggerFromContext(ctx)

	query, args, err := psql.
		Select("COUNT(*)").
		From("agents a").
		Where(where).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build agents count query: %w", err)
	}

	var totalCount int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); err != nil {
		logger.With(
			"method", "List",
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to count agents")
		return 0, fmt.Errorf("failed to count agents: %w", err)
	}

	return totalCount, nil
}

// agentPaging resolves the LIMIT/OFFSET for the List page query.
//
// Defaulting contract (NOT clamping): this repository historically treats a
// non-positive Limit as "use the default page size 10" and a non-positive Page
// as "page 1". This differs deliberately from the LIMIT-0 clamp convention of
// some sibling repositories; the defaulting predates the squirrel migration and
// is preserved verbatim. After defaulting both values are guaranteed positive,
// so the conversions cannot wrap; the explicit `> 0` guard makes that provable
// to gosec (G115).
func agentPaging(filters repositories.AgentFilters) (limit, offset uint64) {
	pageSize := filters.Limit
	if pageSize <= 0 {
		pageSize = 10
	}
	page := filters.Page
	if page <= 0 {
		page = 1
	}

	if pageSize > 0 {
		limit = uint64(pageSize)
		if rawOffset := (page - 1) * pageSize; rawOffset > 0 {
			offset = uint64(rawOffset)
		}
	}
	return limit, offset
}

// queryList runs the paginated page query for List using the shared WHERE
// conditions, so it can never diverge from the count query.
func (r *agentRepository) queryList(
	ctx context.Context, where squirrel.Sqlizer, filters repositories.AgentFilters,
) ([]models.Agent, error) {
	logger := contextkeys.GetLoggerFromContext(ctx)
	limit, offset := agentPaging(filters)

	query, args, err := psql.
		Select(
			"a.id", "a.user_id", "a.team_id", "a.name", "a.description",
			"a.status", "a.card_url", "a.agent_card", "a.last_run",
			"a.last_synced_at", "a.total_runs", "a.success_rate",
			"a.created_at", "a.updated_at",
		).
		From("agents a").
		Where(where).
		OrderBy(buildAgentListOrderByClause(filters)).
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build agents list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		logger.With(
			"method", "List",
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list agents")
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logger.With("error", closeErr).Error("Failed to close rows")
		}
	}()

	agents, err := scanAgentListRows(ctx, rows)
	if err != nil {
		return nil, err
	}

	if err := rows.Err(); err != nil {
		logger.With(
			"method", "List",
			"error", fmt.Sprintf("%+v", err),
		).Error("Error iterating agent rows")
		return nil, fmt.Errorf("failed to iterate agents: %w", err)
	}

	return agents, nil
}

// scanAgentListRows scans the 14-column projection used by the List page query,
// mapping nullable columns to pointers. An oversized or malformed agent_card
// column is logged at WARN and the row is kept without a card (never an error) —
// this matches the prior hand-built behaviour exactly. The request-scoped logger
// is threaded through so the WARN/Error lines carry the same fields as before.
func scanAgentListRows(ctx context.Context, rows *sql.Rows) ([]models.Agent, error) {
	logger := contextkeys.GetLoggerFromContext(ctx)

	agents := make([]models.Agent, 0)
	for rows.Next() {
		var agent models.Agent
		var agentCardJSON []byte
		var cardURL sql.NullString
		var lastRun sql.NullTime
		var lastSyncedAt sql.NullTime

		if scanErr := rows.Scan(&agent.ID, &agent.UserID, &agent.TeamID, &agent.Name, &agent.Description,
			&agent.Status, &cardURL, &agentCardJSON, &lastRun, &lastSyncedAt, &agent.TotalRuns, &agent.SuccessRate,
			&agent.CreatedAt, &agent.UpdatedAt); scanErr != nil {
			logger.With(
				"method", "List",
				"error", fmt.Sprintf("%+v", scanErr),
			).Error("Failed to scan agent row")
			return nil, fmt.Errorf("failed to scan agent: %w", scanErr)
		}

		if cardURL.Valid {
			agent.CardURL = &cardURL.String
		}
		if lastRun.Valid {
			agent.LastRun = &lastRun.Time
		}
		if lastSyncedAt.Valid {
			agent.LastSyncedAt = &lastSyncedAt.Time
		}

		applyAgentCard(logger, &agent, agentCardJSON)

		agents = append(agents, agent)
	}

	return agents, nil
}

// applyAgentCard decodes the agent_card JSON column onto the agent, preserving
// the WARN-and-continue contract: an oversized payload (guarding against memory
// exhaustion) or a malformed payload leaves the card unset and never surfaces an
// error.
func applyAgentCard(logger *slog.Logger, agent *models.Agent, agentCardJSON []byte) {
	if len(agentCardJSON) == 0 {
		return
	}

	if len(agentCardJSON) > MaxAgentCardJSONSize {
		logger.With(
			"method", "List",
			"agent_id", agent.ID,
			"size", len(agentCardJSON),
			"max_size", MaxAgentCardJSONSize,
		).Warn("Agent card JSON exceeds maximum allowed size, continuing without card")
		return
	}

	var agentCard models.AgentCard
	if err := json.Unmarshal(agentCardJSON, &agentCard); err != nil {
		logger.With(
			"method", "List",
			"agent_id", agent.ID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to unmarshal agent card, continuing without card")
		return
	}

	agent.AgentCard = &agentCard
}

// Update updates an existing agent
//
//nolint:funlen // Update method with team validation and optimistic locking requires extra logic
func (r *agentRepository) Update(ctx context.Context, agent *models.Agent) error {
	logger := contextkeys.GetLoggerFromContext(ctx)

	agent.UpdatedAt = time.Now()

	// Validate ownership/admin rights BEFORE checking version to distinguish errors
	// This prevents conflating authorization failures with optimistic lock conflicts
	// Only allow: resource owner, team owner, or team admin (matches Delete method permissions)
	// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
	var exists bool
	ownershipQuery := `
		SELECT EXISTS(
			SELECT 1 FROM agents a
			WHERE a.id = $1
				AND a.team_id = $2
				AND (
					a.user_id = $3
					OR EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
					OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3 AND role IN ('owner', 'admin'))
				)
		)
	`
	err := r.db.QueryRowContext(ctx, ownershipQuery, agent.ID, agent.TeamID, agent.UserID).Scan(&exists)
	if err != nil {
		logger.With(
			"method", "Update",
			"agent_id", agent.ID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to validate agent ownership")
		return fmt.Errorf("failed to validate agent ownership: %w", err)
	}
	if !exists {
		// Agent doesn't exist for this user/team - authorization failure
		return repositories.ErrAgentNotFound
	}

	var agentCardJSON []byte
	if agent.AgentCard != nil {
		agentCardJSON, err = json.Marshal(agent.AgentCard)
		if err != nil {
			return fmt.Errorf("failed to marshal agent card: %w", err)
		}
	}

	var credentialsJSON []byte
	if agent.Credentials != nil {
		credentialsJSON, err = json.Marshal(agent.Credentials)
		if err != nil {
			return fmt.Errorf("failed to marshal credentials: %w", err)
		}
	}

	// Use EXISTS subqueries to avoid multiple row matches from JOIN
	query := `
		UPDATE agents
		SET name = $1, description = $2, status = $3, card_url = $4, agent_card = $5,
		credentials = $6, last_synced_at = $7, updated_at = $8, version = version + 1
		WHERE id = $9
			AND team_id = $10
			AND version = $11
			AND (
				user_id = $12
				OR EXISTS (SELECT 1 FROM teams WHERE id = $10 AND owner_id = $12)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $10 AND user_id = $12 AND role IN ('owner', 'admin'))
			)
		RETURNING updated_at, version
	`

	err = r.db.QueryRowContext(ctx, query,
		agent.Name, agent.Description, agent.Status, agent.CardURL, agentCardJSON,
		credentialsJSON, agent.LastSyncedAt, agent.UpdatedAt, agent.ID, agent.TeamID, agent.Version, agent.UserID,
	).Scan(&agent.UpdatedAt, &agent.Version)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Since we already validated ownership, this must be a version mismatch
			return fmt.Errorf("version conflict: resource was modified by another request")
		}
		if uniqueViolation(err) != nil {
			return fmt.Errorf("%w (name %q)", repositories.ErrAgentNameConflict, agent.Name)
		}
		logger.With(
			"method", "Update",
			"agent_id", agent.ID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to update agent")
		return fmt.Errorf("failed to update agent: %w", err)
	}

	return nil
}

// Delete deletes an agent
// Allows deletion if user is: resource owner, team owner, or team admin
func (r *agentRepository) Delete(ctx context.Context, userID, teamID, agentID string) error {
	logger := contextkeys.GetLoggerFromContext(ctx)

	// Use EXISTS subqueries to avoid multiple row matches from JOIN
	query := `
		DELETE FROM agents
		WHERE id = $1
			AND team_id = $2
			AND (
				user_id = $3
				OR EXISTS (SELECT 1 FROM teams WHERE id = $2 AND owner_id = $3)
				OR EXISTS (SELECT 1 FROM team_members WHERE team_id = $2 AND user_id = $3 AND role IN ('owner', 'admin'))
			)
	`

	result, err := r.db.ExecContext(ctx, query, agentID, teamID, userID)
	if err != nil {
		logger.With(
			"method", "Delete",
			"agent_id", agentID,
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to delete agent")
		return fmt.Errorf("failed to delete agent: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrAgentNotFound
	}

	return nil
}

// GetStats retrieves agent statistics for a user
// If teamID is empty, returns stats across all teams for the user
func (r *agentRepository) GetStats(ctx context.Context, userID, teamID string) (*models.AgentStatsResponse, error) {
	logger := contextkeys.GetLoggerFromContext(ctx)
	stats := &models.AgentStatsResponse{}

	// Get basic counts and averages
	var basicStatsQuery string
	var args []interface{}

	if teamID == "" {
		// Count across all teams for the user
		basicStatsQuery = `
			SELECT
				COUNT(*) as total_agents,
				COUNT(CASE WHEN status = 'active' THEN 1 END) as active_agents,
				COUNT(CASE WHEN status = 'paused' THEN 1 END) as paused_agents,
				COUNT(CASE WHEN status = 'error' THEN 1 END) as error_agents,
				COALESCE(SUM(total_runs), 0) as total_runs,
				COALESCE(AVG(success_rate), 0) as avg_success_rate
			FROM agents
			WHERE user_id = $1
		`
		args = []interface{}{userID}
	} else {
		// Count for specific team
		basicStatsQuery = `
			SELECT
				COUNT(*) as total_agents,
				COUNT(CASE WHEN status = 'active' THEN 1 END) as active_agents,
				COUNT(CASE WHEN status = 'paused' THEN 1 END) as paused_agents,
				COUNT(CASE WHEN status = 'error' THEN 1 END) as error_agents,
				COALESCE(SUM(total_runs), 0) as total_runs,
				COALESCE(AVG(success_rate), 0) as avg_success_rate
			FROM agents
			WHERE user_id = $1 AND team_id = $2
		`
		args = []interface{}{userID, teamID}
	}

	err := r.db.QueryRowContext(ctx, basicStatsQuery, args...).Scan(
		&stats.TotalAgents, &stats.ActiveAgents, &stats.PausedAgents,
		&stats.ErrorAgents, &stats.TotalRuns, &stats.AvgSuccessRate)

	if err != nil {
		logger.With(
			"method", "GetStats",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get basic agent stats")
		return nil, fmt.Errorf("failed to get agent stats: %w", err)
	}

	// For now, set runs today and this week to 0 since we don't have execution tracking yet
	stats.RunsToday = 0
	stats.RunsThisWeek = 0

	// Get recent activities (simulated for now)
	stats.RecentActivities = []models.AgentActivity{}

	return stats, nil
}

// GetNamesByIDsCrossTeam returns a map of agentID → name for the given IDs visible to userID
// across all teams the user belongs to (owner or member).
// Unknown or inaccessible IDs are omitted from the result map.
func (r *agentRepository) GetNamesByIDsCrossTeam(
	ctx context.Context, userID string, ids []string,
) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}

	// $1 is userID; agent IDs start at $2
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids)+1)
	args[0] = userID
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}

	query := fmt.Sprintf(
		`SELECT a.id, a.name FROM agents a
		WHERE a.id IN (%s)
			AND (
				EXISTS (SELECT 1 FROM teams t WHERE t.id = a.team_id AND t.owner_id = $1)
				OR EXISTS (SELECT 1 FROM team_members tm WHERE tm.team_id = a.team_id AND tm.user_id = $1)
			)`,
		strings.Join(placeholders, ", "),
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get agent names by ids: %w", err)
	}

	result, scanErr := scanIDNameRows(rows, len(ids), "agent")
	if closeErr := rows.Close(); closeErr != nil && scanErr == nil {
		return nil, fmt.Errorf("close agent name rows: %w", closeErr)
	}
	if scanErr != nil {
		return nil, scanErr
	}
	return result, nil
}

// UpdateExecutionStats updates agent execution statistics using atomic SQL operations to prevent race conditions
func (r *agentRepository) UpdateExecutionStats(ctx context.Context, agentID string, success bool, duration int) error {
	// Use a single atomic SQL statement to prevent race conditions
	// This calculates the new success rate based on current database values
	var successIncrement int
	if success {
		successIncrement = 1
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE agents
		SET
			total_runs = total_runs + 1,
			success_rate = CASE
				WHEN total_runs + 1 = 0 THEN 0
				ELSE ((success_rate / 100.0 * total_runs) + $2) / (total_runs + 1) * 100.0
			END,
			last_run = $3,
			updated_at = $4
		WHERE id = $1`,
		agentID, successIncrement, time.Now(), time.Now())

	if err != nil {
		return fmt.Errorf("failed to update agent stats: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("%w: %s", repositories.ErrAgentNotFound, agentID)
	}

	return nil
}
