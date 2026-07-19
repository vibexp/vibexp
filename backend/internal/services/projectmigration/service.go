package projectmigration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ErrTeamMismatch is returned when the URL team_id does not match the project's team.
var ErrTeamMismatch = errors.New("project does not belong to the specified team")

// Ensure Service satisfies the interface expected by the container.
// The interface is declared in internal/services to avoid an import cycle;
// the compile-time assertion lives here because this package owns the concrete type.
var _ interface {
	GetInventory(ctx context.Context, userID, teamID, projectID string) (*MigrationInventory, error)
	Migrate(ctx context.Context, userID, teamID, sourceProjectID string, req *MigrationRequest) (*MigrationResult, error)
} = (*Service)(nil)

// Service performs project-scoped resource migrations within a team.
// All resources are MOVED (project_id reparented) — never copied.
// The entire migration runs inside a single database transaction.
type Service struct {
	db          *database.DB
	projectRepo repositories.ProjectRepository
	logger      *slog.Logger
}

// NewService creates a new project migration service.
func NewService(
	db *database.DB,
	projectRepo repositories.ProjectRepository,
	logger *slog.Logger,
) *Service {
	return &Service{
		db:          db,
		projectRepo: projectRepo,
		logger:      logger,
	}
}

// GetInventory returns a count and item list of every resource in the given source project.
func (s *Service) GetInventory(
	ctx context.Context,
	userID, teamID, projectID string,
) (*MigrationInventory, error) {
	// Verify the user can access the project.
	srcProject, err := s.projectRepo.GetByID(ctx, userID, projectID)
	if err != nil {
		return nil, fmt.Errorf("GetInventory: source project not accessible: %w", err)
	}

	// Enforce that the URL team_id matches the project's actual team.
	if srcProject.TeamID != teamID {
		return nil, fmt.Errorf("GetInventory: %w", ErrTeamMismatch)
	}

	inv := &MigrationInventory{}

	// Prompts
	prompts, err := s.queryInventoryItems(ctx, projectID,
		`SELECT id, slug FROM prompts WHERE project_id = $1 ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("GetInventory: query prompts: %w", err)
	}
	inv.Prompts = prompts

	// Artifacts
	artifacts, err := s.queryInventoryItems(ctx, projectID,
		`SELECT id, slug FROM artifacts WHERE project_id = $1 ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("GetInventory: query artifacts: %w", err)
	}
	inv.Artifacts = artifacts

	// Blueprints
	blueprints, err := s.queryInventoryItems(ctx, projectID,
		`SELECT id, slug FROM blueprints WHERE project_id = $1 ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("GetInventory: query blueprints: %w", err)
	}
	inv.Blueprints = blueprints

	// Feed items — project_id is NULLABLE; only count rows with an explicit assignment.
	feedItems, err := s.queryInventoryItems(ctx, projectID,
		`SELECT id, title FROM feed_items WHERE project_id = $1 ORDER BY posted_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("GetInventory: query feed_items: %w", err)
	}
	inv.FeedItems = feedItems

	return inv, nil
}

// queryInventoryItems runs a single-column query (id, name) and builds a ResourceInventory.
// The query must accept exactly one $1 parameter (projectID).
func (s *Service) queryInventoryItems(
	ctx context.Context, projectID, query string,
) (ResourceInventory, error) {
	rows, err := s.db.QueryContext(ctx, query, projectID)
	if err != nil {
		return ResourceInventory{}, fmt.Errorf("query failed: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			s.logger.With("error", closeErr).Warn("failed to close rows in queryInventoryItems")
		}
	}()

	var items []ResourceInventoryItem
	for rows.Next() {
		var item ResourceInventoryItem
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			return ResourceInventory{}, fmt.Errorf("scan row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ResourceInventory{}, fmt.Errorf("iterate rows: %w", err)
	}

	return ResourceInventory{Count: len(items), Items: items}, nil
}

// Migrate moves the selected resources from sourceProjectID to req.DestinationProjectID.
// Both projects must belong to the same team and be accessible to userID.
// All UPDATEs run in a single transaction; any DB error rolls back the entire migration.
func (s *Service) Migrate(
	ctx context.Context,
	userID, teamID, sourceProjectID string,
	req *MigrationRequest,
) (*MigrationResult, error) {
	srcProject, dstProject, err := s.validateMigrationProjects(
		ctx, userID, teamID, sourceProjectID, req.DestinationProjectID,
	)
	if err != nil {
		return nil, err
	}

	result := &MigrationResult{
		SourceProjectName:      srcProject.Name,
		DestinationProjectName: dstProject.Name,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("Migrate: begin transaction: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && rbErr != sql.ErrTxDone {
			s.logger.With("error", rbErr).Error("Migrate: failed to rollback transaction")
		}
	}()

	if err := s.migratePrompts(ctx, tx, sourceProjectID, req, result); err != nil {
		return nil, fmt.Errorf("Migrate: prompts: %w", err)
	}
	if err := s.migrateArtifacts(ctx, tx, sourceProjectID, req, result); err != nil {
		return nil, fmt.Errorf("Migrate: artifacts: %w", err)
	}
	if err := s.migrateBlueprints(ctx, tx, sourceProjectID, req, result); err != nil {
		return nil, fmt.Errorf("Migrate: blueprints: %w", err)
	}
	if err := s.migrateFeedItems(ctx, tx, sourceProjectID, req, result); err != nil {
		return nil, fmt.Errorf("Migrate: feed_items: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("Migrate: commit transaction: %w", err)
	}

	return result, nil
}

// validateMigrationProjects loads and validates both source and destination projects.
// It checks team membership and same-team constraint before returning the project records.
func (s *Service) validateMigrationProjects(
	ctx context.Context,
	userID, teamID, sourceProjectID, destProjectID string,
) (*models.Project, *models.Project, error) {
	srcProject, err := s.projectRepo.GetByID(ctx, userID, sourceProjectID)
	if err != nil {
		return nil, nil, fmt.Errorf("Migrate: source project not accessible: %w", err)
	}
	if srcProject.TeamID != teamID {
		return nil, nil, fmt.Errorf("Migrate: %w", ErrTeamMismatch)
	}

	dstProject, err := s.projectRepo.GetByID(ctx, userID, destProjectID)
	if err != nil {
		return nil, nil, fmt.Errorf("Migrate: destination project not accessible: %w", err)
	}
	if srcProject.TeamID != dstProject.TeamID {
		return nil, nil, fmt.Errorf("cross-team migration not supported: source team %s, destination team %s",
			srcProject.TeamID, dstProject.TeamID)
	}

	return srcProject, dstProject, nil
}

// migratePrompts moves selected prompts to the destination project.
// Prompts are uniquely keyed by (user_id, team_id, slug) — not by project — so there are
// no intra-project slug collisions to resolve.
func (s *Service) migratePrompts(
	ctx context.Context,
	tx *sql.Tx,
	sourceProjectID string,
	req *MigrationRequest,
	result *MigrationResult,
) error {
	ids, err := s.resolveIDs(ctx, tx, sourceProjectID, req.Resources.Prompts,
		`SELECT id FROM prompts WHERE project_id = $1`,
	)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	for _, id := range ids {
		res, err := tx.ExecContext(ctx,
			`UPDATE prompts SET project_id = $1, version = version + 1, updated_at = NOW() WHERE id = $2 AND project_id = $3`,
			req.DestinationProjectID, id, sourceProjectID,
		)
		if err != nil {
			result.Failed.Prompts = append(result.Failed.Prompts, ResourceOutcome{ID: id, Reason: err.Error()})
			continue
		}
		if n, rowErr := res.RowsAffected(); rowErr == nil && n == 0 {
			result.Failed.Prompts = append(result.Failed.Prompts, ResourceOutcome{ID: id, Reason: "concurrent_modification"})
			continue
		}
		result.Migrated.Prompts++
	}
	return nil
}

// sluggedMigration carries the per-table context shared by migrateSluggedResources:
// the source/destination projects, the table with its selection/queries, and the
// result sinks to record outcomes into.
type sluggedMigration struct {
	srcProjectID  string
	destProjectID string
	table         string
	sel           ResourceSelection
	allQuery      string
	failed        *[]ResourceOutcome
	count         *int
}

// migrateSluggedResources is the shared move loop for artifacts and blueprints.
//
// There is no destination slug-collision path: artifact and blueprint slugs are
// team-unique (artifacts_slug_team_id_key / blueprints_slug_team_id_key) and
// migration is same-team-only (enforced in validateMigrationProjects), so a source
// row's slug is already the sole holder of that slug team-wide and a same-team
// destination can never already contain a colliding slug. Every selected row is
// therefore reparented directly.
func (s *Service) migrateSluggedResources(
	ctx context.Context,
	tx *sql.Tx,
	m *sluggedMigration,
) error {
	ids, err := s.resolveIDs(ctx, tx, m.srcProjectID, m.sel, m.allQuery)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	// #nosec G201 - table is a hardcoded string literal from callers, never user input
	updateQ := fmt.Sprintf(
		`UPDATE %s SET project_id = $1, version = version + 1, updated_at = NOW() WHERE id = $2 AND project_id = $3`,
		m.table,
	)
	for _, id := range ids {
		execRes, execErr := tx.ExecContext(ctx, updateQ, m.destProjectID, id, m.srcProjectID)
		if execErr != nil {
			*m.failed = append(*m.failed, ResourceOutcome{ID: id, Reason: execErr.Error()})
			continue
		}
		if n, rowErr := execRes.RowsAffected(); rowErr == nil && n == 0 {
			*m.failed = append(*m.failed, ResourceOutcome{ID: id, Reason: "concurrent_modification"})
			continue
		}
		*m.count++
	}
	return nil
}

// migrateArtifacts moves selected artifacts to the destination project.
func (s *Service) migrateArtifacts(
	ctx context.Context,
	tx *sql.Tx,
	sourceProjectID string,
	req *MigrationRequest,
	result *MigrationResult,
) error {
	return s.migrateSluggedResources(ctx, tx, &sluggedMigration{
		srcProjectID:  sourceProjectID,
		destProjectID: req.DestinationProjectID,
		table:         "artifacts",
		sel:           req.Resources.Artifacts,
		allQuery:      `SELECT id FROM artifacts WHERE project_id = $1`,
		failed:        &result.Failed.Artifacts,
		count:         &result.Migrated.Artifacts,
	})
}

// migrateBlueprints moves selected blueprints to the destination project.
func (s *Service) migrateBlueprints(
	ctx context.Context,
	tx *sql.Tx,
	sourceProjectID string,
	req *MigrationRequest,
	result *MigrationResult,
) error {
	return s.migrateSluggedResources(ctx, tx, &sluggedMigration{
		srcProjectID:  sourceProjectID,
		destProjectID: req.DestinationProjectID,
		table:         "blueprints",
		sel:           req.Resources.Blueprints,
		allQuery:      `SELECT id FROM blueprints WHERE project_id = $1`,
		failed:        &result.Failed.Blueprints,
		count:         &result.Migrated.Blueprints,
	})
}

// migrateFeedItems moves feed items whose project_id matches sourceProjectID.
// Feed items have a nullable project_id with ON DELETE SET NULL — we only touch rows
// that are explicitly assigned to the source project.
func (s *Service) migrateFeedItems(
	ctx context.Context,
	tx *sql.Tx,
	sourceProjectID string,
	req *MigrationRequest,
	result *MigrationResult,
) error {
	ids, err := s.resolveIDs(ctx, tx, sourceProjectID, req.Resources.FeedItems,
		`SELECT id FROM feed_items WHERE project_id = $1`,
	)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	for _, id := range ids {
		res, execErr := tx.ExecContext(ctx,
			`UPDATE feed_items SET project_id = $1 WHERE id = $2 AND project_id = $3`,
			req.DestinationProjectID, id, sourceProjectID,
		)
		if execErr != nil {
			result.Failed.FeedItems = append(result.Failed.FeedItems,
				ResourceOutcome{ID: id, Reason: execErr.Error()})
			continue
		}
		if n, rowErr := res.RowsAffected(); rowErr == nil && n == 0 {
			result.Failed.FeedItems = append(result.Failed.FeedItems,
				ResourceOutcome{ID: id, Reason: "concurrent_modification"})
			continue
		}
		result.Migrated.FeedItems++
	}
	return nil
}

// resolveIDs returns the IDs to migrate for a given resource type.
// When sel.All is true every ID in the source project is returned.
// When sel.All is false sel.IDs is filtered to those that actually belong to the source project.
func (s *Service) resolveIDs(
	ctx context.Context,
	tx *sql.Tx,
	sourceProjectID string,
	sel ResourceSelection,
	allQuery string,
) ([]string, error) {
	if sel.All {
		return s.queryIDsFromTx(ctx, tx, allQuery, sourceProjectID)
	}

	if len(sel.IDs) == 0 {
		return nil, nil
	}

	// Build a parameterized IN clause to confirm IDs belong to the source project.
	// Extract table name from allQuery to build the validation query.
	table := extractTableName(allQuery)
	if table == "" {
		return nil, fmt.Errorf("could not extract table name from query: %s", allQuery)
	}

	placeholders := make([]string, len(sel.IDs))
	args := make([]interface{}, len(sel.IDs)+1)
	args[0] = sourceProjectID
	for i, id := range sel.IDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}

	query := fmt.Sprintf(
		`SELECT id FROM %s WHERE project_id = $1 AND id IN (%s)`,
		table, strings.Join(placeholders, ","),
	)
	return s.queryIDsFromTxArgs(ctx, tx, query, args...)
}

// queryIDsFromTx runs a single-$1 query inside a transaction and collects ids.
func (s *Service) queryIDsFromTx(ctx context.Context, tx *sql.Tx, query, arg string) ([]string, error) {
	return s.queryIDsFromTxArgs(ctx, tx, query, arg)
}

func (s *Service) queryIDsFromTxArgs(
	ctx context.Context, tx *sql.Tx, query string, args ...interface{},
) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("queryIDsFromTxArgs: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			s.logger.With("error", closeErr).Warn("failed to close rows in queryIDsFromTxArgs")
		}
	}()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// extractTableName parses the table name from a simple "SELECT ... FROM <table> ..." query.
// It is used only for building dynamic IN queries from validated table names already hardcoded
// in this file — never from user input.
func extractTableName(query string) string {
	lower := strings.ToLower(query)
	fromIdx := strings.Index(lower, " from ")
	if fromIdx < 0 {
		return ""
	}
	rest := strings.TrimSpace(query[fromIdx+6:])
	// Take the first token (table name).
	end := strings.IndexAny(rest, " \t\n\r")
	if end < 0 {
		return rest
	}
	return rest[:end]
}
