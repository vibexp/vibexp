package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ErrInvalidFreshnessRule is returned when a rule definition is rejected by
// validation. Handlers map it to 400.
var ErrInvalidFreshnessRule = errors.New("invalid freshness rule")

// ErrInvalidFreshnessSettings is returned when freshness settings are rejected
// by validation. Handlers map it to 400.
var ErrInvalidFreshnessSettings = errors.New("invalid freshness settings")

// freshnessRuleResourceTypes and freshnessRuleMediums are the value sets the
// API accepts. They are validated here rather than by database CHECK
// constraints (see migration 013) so widening either is not a migration; they
// must stay in step with the enums in schemas/freshness.yaml, which
// TestSpecEnumsMatchServiceAllowlists enforces (#774).
//
// Note mediums deliberately excludes "api": accesses from generic API clients
// are recorded, but are not offered as rule criteria.
var (
	freshnessRuleResourceTypes = []string{"artifact", "prompt", "blueprint", "memory"}
	freshnessRuleMediums       = []string{"web", "cli", "mcp"}
)

// Upper bounds on the two numeric dimensions, mirroring the schema's `maximum`
// so an out-of-range value is a clean 400 rather than a column overflow the
// database reports as a 500. They also keep both values provably inside the
// int32 the wire format and the columns use.
const (
	// MaxFreshnessThresholdDays is 100 years, mirroring the search-settings
	// half-life cap; past it a rule can never fire.
	MaxFreshnessThresholdDays = 36500
	// MaxFreshnessIntervalSeconds is 365 days; a longer evaluation interval is
	// indistinguishable from disabling evaluation.
	MaxFreshnessIntervalSeconds = 31536000
)

// FreshnessRuleInput is the mutable definition of a rule, shared by create and
// update. A nil ProjectID means "every project in the team"; an empty Mediums
// means "any medium".
type FreshnessRuleInput struct {
	ProjectID     *string
	ResourceTypes []string
	Mediums       []string
	ThresholdDays int
	Enabled       bool
}

// FreshnessServiceInterface manages a team's freshness rules and settings.
//
// Reads take no permission check: the tenancy middleware has already
// established team membership, and every member may see the team's policy.
// Every write authorizes authz.TeamSettingsUpdate — the same permission the
// other settings surfaces use, so no new permission constant enters the
// published `permissions` enum.
type FreshnessServiceInterface interface {
	// ListRules returns the team's rules oldest first, never nil.
	ListRules(ctx context.Context, teamID string) ([]*models.FreshnessRule, error)
	// CreateRule validates and stores a new rule.
	CreateRule(ctx context.Context, userID, teamID string, input FreshnessRuleInput) (*models.FreshnessRule, error)
	// UpdateRule replaces a rule in full. It returns
	// repositories.ErrFreshnessRuleNotFound when the team has no such rule.
	UpdateRule(ctx context.Context, userID, teamID, ruleID string, input FreshnessRuleInput) (*models.FreshnessRule, error)
	// DeleteRule removes a rule and strips it from any freshness state that
	// matched it. It returns repositories.ErrFreshnessRuleNotFound when the
	// team has no such rule.
	DeleteRule(ctx context.Context, userID, teamID, ruleID string) error
	// GetSettings returns the team's effective settings with their provenance
	// and the defaults a reset would restore.
	GetSettings(ctx context.Context, teamID string) (*models.TeamFreshnessSettingsView, error)
	// UpdateSettings validates and stores the team's settings, returning the
	// resulting view.
	UpdateSettings(
		ctx context.Context, userID, teamID string, values models.FreshnessSettingsValues,
	) (*models.TeamFreshnessSettingsView, error)
	// ResetSettings removes the team's stored settings so it inherits the
	// defaults. Resetting a team that has none is a no-op, not an error.
	ResetSettings(ctx context.Context, userID, teamID string) error

	// The five reads below back the analytics charts and the audit tab (#734).
	// Like the other reads here they take no userID and perform no permission
	// check: the tenancy middleware has established membership, and every
	// member may see what the engine did.

	// GetOverTimeMetrics returns the daily marked/cleared flows and the
	// reconstructed stale level over the window, zero-filled.
	GetOverTimeMetrics(ctx context.Context, teamID string, rangeDays int) (*models.FreshnessOverTimeMetrics, error)
	// GetByTypeMetrics returns current stale counts for all four resource
	// types, including the types with nothing stale.
	GetByTypeMetrics(ctx context.Context, teamID string) (*models.FreshnessTypeMetrics, error)
	// GetByProjectMetrics returns current stale counts for every project in
	// the team, including projects with nothing stale.
	GetByProjectMetrics(ctx context.Context, teamID string) (*models.FreshnessProjectMetrics, error)
	// GetByRuleMetrics returns how many resources each of the team's rules
	// currently marks, including rules matching nothing.
	GetByRuleMetrics(ctx context.Context, teamID string) (*models.FreshnessRuleMetrics, error)
	// ListAudit returns one page of the team's audit log, newest first.
	ListAudit(ctx context.Context, teamID string, page, limit int) (*models.FreshnessAuditPage, error)

	// GetResourceFreshness returns one resource's freshness state, or nil when
	// it is fresh or belongs to another team (#735).
	GetResourceFreshness(
		ctx context.Context, teamID, resourceType, resourceID string,
	) (*models.ResourceFreshnessState, error)
	// ListResourceFreshness returns the freshness state of a page of resources
	// keyed by resource id, in one query. Fresh resources are absent.
	ListResourceFreshness(
		ctx context.Context, teamID, resourceType string, resourceIDs []string,
	) (map[string]*models.ResourceFreshnessState, error)

	// ReconcileSchedules provisions a `freshness_evaluate` schedule for every
	// team that has rules but no schedule row (#768). It is NOT an API
	// operation: the scheduler's reconcile loop is the only caller, so it
	// takes no userID and authorizes nothing, like the evaluator the same loop
	// drives. It satisfies scheduler.Reconciler.
	ReconcileSchedules(ctx context.Context) error
}

// FreshnessService implements FreshnessServiceInterface.
type FreshnessService struct {
	rules     repositories.FreshnessRuleRepository
	freshness repositories.ResourceFreshnessRepository
	settings  repositories.TeamFreshnessSettingsRepository
	audit     repositories.FreshnessAuditRepository
	schedules repositories.ScheduleRepository
	projects  repositories.ProjectRepository
	authz     AuthorizationServiceInterface
	logger    *slog.Logger
}

var _ FreshnessServiceInterface = (*FreshnessService)(nil)

// NewFreshnessService creates a new FreshnessService.
func NewFreshnessService(
	rules repositories.FreshnessRuleRepository,
	freshness repositories.ResourceFreshnessRepository,
	settings repositories.TeamFreshnessSettingsRepository,
	audit repositories.FreshnessAuditRepository,
	schedules repositories.ScheduleRepository,
	projects repositories.ProjectRepository,
	authzService AuthorizationServiceInterface,
	logger *slog.Logger,
) *FreshnessService {
	return &FreshnessService{
		rules:     rules,
		freshness: freshness,
		settings:  settings,
		audit:     audit,
		schedules: schedules,
		projects:  projects,
		authz:     authzService,
		logger:    logger,
	}
}

// ListRules returns the team's rules, oldest first.
func (s *FreshnessService) ListRules(ctx context.Context, teamID string) ([]*models.FreshnessRule, error) {
	rules, err := s.rules.ListByTeam(ctx, teamID, false)
	if err != nil {
		return nil, fmt.Errorf("FreshnessService.ListRules: %w", err)
	}
	return rules, nil
}

// CreateRule validates and stores a new rule.
func (s *FreshnessService) CreateRule(
	ctx context.Context, userID, teamID string, input FreshnessRuleInput,
) (*models.FreshnessRule, error) {
	if err := s.authz.Can(ctx, userID, teamID, authz.TeamSettingsUpdate); err != nil {
		return nil, err
	}
	if err := s.validateRule(ctx, userID, teamID, input); err != nil {
		return nil, err
	}

	rule := &models.FreshnessRule{
		TeamID:        teamID,
		ProjectID:     input.ProjectID,
		ResourceTypes: input.ResourceTypes,
		Mediums:       input.Mediums,
		ThresholdDays: input.ThresholdDays,
		Enabled:       input.Enabled,
	}
	if err := s.rules.Create(ctx, rule); err != nil {
		return nil, fmt.Errorf("FreshnessService.CreateRule: %w", err)
	}
	s.syncSchedule(ctx, teamID)
	return rule, nil
}

// UpdateRule replaces a rule in full.
func (s *FreshnessService) UpdateRule(
	ctx context.Context, userID, teamID, ruleID string, input FreshnessRuleInput,
) (*models.FreshnessRule, error) {
	if err := s.authz.Can(ctx, userID, teamID, authz.TeamSettingsUpdate); err != nil {
		return nil, err
	}
	if err := s.validateRule(ctx, userID, teamID, input); err != nil {
		return nil, err
	}

	rule := &models.FreshnessRule{
		ID:            ruleID,
		TeamID:        teamID,
		ProjectID:     input.ProjectID,
		ResourceTypes: input.ResourceTypes,
		Mediums:       input.Mediums,
		ThresholdDays: input.ThresholdDays,
		Enabled:       input.Enabled,
	}
	if err := s.rules.Update(ctx, rule); err != nil {
		if errors.Is(err, repositories.ErrFreshnessRuleNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("FreshnessService.UpdateRule: %w", err)
	}
	s.syncSchedule(ctx, teamID)
	return rule, nil
}

// DeleteRule removes a rule and the references to it held by freshness state.
//
// The strip runs BEFORE the delete deliberately. If the strip succeeds and the
// delete then fails, the rule still exists and the next evaluation run simply
// re-marks what it matches — self-healing. The opposite order fails the other
// way: a deleted rule whose ids were never stripped leaves freshness rows
// pointing at a rule that no longer exists, with nothing to repair them.
func (s *FreshnessService) DeleteRule(ctx context.Context, userID, teamID, ruleID string) error {
	if err := s.authz.Can(ctx, userID, teamID, authz.TeamSettingsUpdate); err != nil {
		return err
	}

	// Scoped by team, so another team's rule id is indistinguishable from a
	// missing one and cannot be probed for existence.
	existing, err := s.rules.GetByID(ctx, teamID, ruleID)
	if err != nil {
		return fmt.Errorf("FreshnessService.DeleteRule: %w", err)
	}
	if existing == nil {
		return repositories.ErrFreshnessRuleNotFound
	}

	if _, err = s.freshness.RemoveRule(ctx, ruleID); err != nil {
		return fmt.Errorf("FreshnessService.DeleteRule: %w", err)
	}

	deleted, err := s.rules.Delete(ctx, teamID, ruleID)
	if err != nil {
		return fmt.Errorf("FreshnessService.DeleteRule: %w", err)
	}
	if !deleted {
		// Lost a race with a concurrent delete; the caller's intent still holds.
		return repositories.ErrFreshnessRuleNotFound
	}
	s.syncSchedule(ctx, teamID)
	return nil
}

// GetSettings returns the team's effective settings and their provenance.
func (s *FreshnessService) GetSettings(
	ctx context.Context, teamID string,
) (*models.TeamFreshnessSettingsView, error) {
	stored, err := s.settings.Get(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("FreshnessService.GetSettings: %w", err)
	}
	if stored == nil {
		return s.view(models.FreshnessSettingsSourceInstance, models.DefaultFreshnessSettingsValues()), nil
	}
	return s.view(models.FreshnessSettingsSourceTeam, models.FreshnessSettingsValues{
		IntervalSeconds:      stored.IntervalSeconds,
		ReversibilityEnabled: stored.ReversibilityEnabled,
	}), nil
}

// UpdateSettings validates and stores the team's settings.
func (s *FreshnessService) UpdateSettings(
	ctx context.Context, userID, teamID string, values models.FreshnessSettingsValues,
) (*models.TeamFreshnessSettingsView, error) {
	if err := s.authz.Can(ctx, userID, teamID, authz.TeamSettingsUpdate); err != nil {
		return nil, err
	}
	if err := ValidateFreshnessSettings(values); err != nil {
		return nil, err
	}

	stored := &models.TeamFreshnessSettings{
		TeamID:               teamID,
		IntervalSeconds:      values.IntervalSeconds,
		ReversibilityEnabled: values.ReversibilityEnabled,
	}
	if err := s.settings.Upsert(ctx, stored); err != nil {
		return nil, fmt.Errorf("FreshnessService.UpdateSettings: %w", err)
	}
	s.syncSchedule(ctx, teamID)
	return s.view(models.FreshnessSettingsSourceTeam, values), nil
}

// ResetSettings drops the team's stored settings so it inherits the defaults.
func (s *FreshnessService) ResetSettings(ctx context.Context, userID, teamID string) error {
	if err := s.authz.Can(ctx, userID, teamID, authz.TeamSettingsUpdate); err != nil {
		return err
	}
	if err := s.settings.Delete(ctx, teamID); err != nil {
		return fmt.Errorf("FreshnessService.ResetSettings: %w", err)
	}
	s.syncSchedule(ctx, teamID)
	return nil
}

// syncSchedule brings the team's `freshness_evaluate` schedule row in line
// with its rules and settings. It is what connects the two halves of the
// feature: the scheduler's timing comes entirely from the `schedules` table,
// while a team configures freshness through its rules and
// team_freshness_settings.interval_seconds, and nothing else bridges them.
//
// The schedule exists while the team has ANY rule, enabled or not -- the
// condition is deliberately NOT "has an enabled rule". Disabling the last
// enabled rule does not clear the state it produced; the next evaluation run
// does, because no rule matches anything any more. Dropping the schedule at
// that moment would remove the only thing that ever calls the evaluator and
// strand every stale flag the team had, with no audit entry and no way back
// until someone re-enables a rule. A team whose rules are all disabled
// therefore keeps a cheap two-query pass.
//
// Removing the LAST rule is different and does delete the schedule: DeleteRule
// strips the rule from freshness state first, so by then there is nothing left
// to clear.
//
// Failure is logged, not returned. The caller's write has already succeeded
// and is the user-visible outcome; failing it because the schedule could not
// be touched would report a successful rule change as an error.
//
// The drift is repaired either by the next rule or settings write, or -- when
// the row is MISSING entirely rather than merely out of date -- by
// ReconcileSchedules, the periodic sweep the scheduler drives (#768). That
// sweep is what stops a failure on a team's very FIRST rule from being
// permanent: before it existed such a team was never evaluated until someone
// happened to write again, which might be never. A one-off seeding migration
// used to cover the analogous case of rules created before this write path
// existed, but it was dropped in the 013_consolidated squash: on that chain
// `freshness_rules` is created empty in the same migration, so it could never
// have matched a row.
func (s *FreshnessService) syncSchedule(ctx context.Context, teamID string) {
	log := s.logger.With("team_id", teamID, "job_type", models.JobTypeFreshnessEvaluate)

	rules, err := s.rules.ListByTeam(ctx, teamID, false)
	if err != nil {
		log.Error("Failed to load freshness rules while syncing schedule", "error", err)
		return
	}

	if len(rules) == 0 {
		if delErr := s.schedules.Delete(ctx, teamID, models.JobTypeFreshnessEvaluate); delErr != nil {
			log.Error("Failed to remove freshness evaluation schedule", "error", delErr)
		}
		return
	}

	if err := s.provisionSchedule(ctx, teamID); err != nil {
		log.Error("Failed to provision freshness evaluation schedule", "error", err)
	}
}

// provisionSchedule resolves the team's evaluation interval and writes its
// `freshness_evaluate` schedule row. It is the SINGLE provisioning path:
// syncSchedule (the user-write side) and ReconcileSchedules (the repair side)
// both go through it, so the two can never drift into subtly different
// versions of "what a freshness schedule looks like". It returns the error
// rather than logging it, because its two callers report failure differently.
//
// NextRunAt is left zero, which the repository stamps with the database clock
// -- so the team becomes due on the next tick. That is deliberate on every
// write, not just the first: a rule the admin just changed takes effect within
// a tick instead of up to a day later, and re-evaluating is idempotent by
// construction, so the extra run costs a query and writes nothing.
//
// Repeating that reset cannot make the job run faster than its interval:
// ListDue applies a run-spacing floor at last_run_at + interval_seconds
// (#767), so saving settings in a loop no longer keeps the team permanently
// due on the serial run loop. The floor is the engine's, and it is the single
// definition of "too soon" -- do not add a second one here.
func (s *FreshnessService) provisionSchedule(ctx context.Context, teamID string) error {
	interval, err := s.effectiveIntervalSeconds(ctx, teamID)
	if err != nil {
		return fmt.Errorf("resolve freshness interval: %w", err)
	}

	if err := s.schedules.Upsert(ctx, &models.Schedule{
		TeamID:          teamID,
		JobType:         models.JobTypeFreshnessEvaluate,
		IntervalSeconds: interval,
	}); err != nil {
		return fmt.Errorf("upsert freshness evaluation schedule: %w", err)
	}
	return nil
}

// ReconcileSchedules provisions a `freshness_evaluate` schedule for every team
// that has freshness rules but no schedule row, healing the one drift
// syncSchedule's best-effort contract cannot heal on its own: a failure while
// a team creates its VERY FIRST rule leaves rules with no schedule, and
// nothing evaluates that team until somebody happens to write again -- which
// may be never (#768). The same end state is reachable by a manual DELETE, a
// partial restore, or any future bug in the write path, so this is a standing
// repair rather than a one-off backfill (the migration that used to cover the
// historical case is gone; see syncSchedule).
//
// It is scoped STRICTLY to missing rows. A team whose schedule already exists
// is not returned by the query and is not touched, so the sweep can never
// reset a healthy row's next_run_at -- doing that instance-wide is #767's
// monopolisation bug applied to every team at once. Broadening this to
// "reconcile every team's interval too" would reintroduce it, and must not be
// done without moving the re-arm decision out of Upsert first.
//
// One team's failure never aborts the sweep: a bad team is logged and the rest
// still get their schedules. Only failing to LIST the teams is returned, since
// then there is no sweep to speak of.
//
// System-invoked (the scheduler's reconcile loop), so there is no user to
// authorize: it takes no userID and performs no permission check, exactly as
// the freshness evaluator the scheduler drives does. It writes nothing a
// team's own rules do not already entitle it to.
func (s *FreshnessService) ReconcileSchedules(ctx context.Context) error {
	teamIDs, err := s.rules.ListTeamIDsMissingSchedule(ctx, models.JobTypeFreshnessEvaluate)
	if err != nil {
		return fmt.Errorf("FreshnessService.ReconcileSchedules: %w", err)
	}
	if len(teamIDs) == 0 {
		return nil
	}

	s.logger.Info("Reconciling missing freshness evaluation schedules",
		"job_type", models.JobTypeFreshnessEvaluate, "team_count", len(teamIDs))

	for _, teamID := range teamIDs {
		// A cancelled context makes every remaining Upsert fail identically,
		// so keep-going would become a fast loop of identical log lines
		// during shutdown. Stop instead; the next sweep picks the rest up.
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("FreshnessService.ReconcileSchedules: %w", err)
		}
		if err := s.provisionSchedule(ctx, teamID); err != nil {
			s.logger.Error("Failed to reconcile freshness evaluation schedule",
				"team_id", teamID, "job_type", models.JobTypeFreshnessEvaluate, "error", err)
			continue
		}
		s.logger.Info("Reconciled missing freshness evaluation schedule",
			"team_id", teamID, "job_type", models.JobTypeFreshnessEvaluate)
	}
	return nil
}

// effectiveIntervalSeconds returns the team's evaluation interval, falling
// back to the default when it stores no settings row -- the same "absent row
// means inherit" rule GetSettings applies.
func (s *FreshnessService) effectiveIntervalSeconds(ctx context.Context, teamID string) (int, error) {
	stored, err := s.settings.Get(ctx, teamID)
	if err != nil {
		return 0, err
	}
	if stored == nil {
		return models.DefaultFreshnessIntervalSeconds, nil
	}
	return stored.IntervalSeconds, nil
}

// view assembles the read model, attaching the defaults a reset would restore.
func (s *FreshnessService) view(
	source string, values models.FreshnessSettingsValues,
) *models.TeamFreshnessSettingsView {
	return &models.TeamFreshnessSettingsView{
		Source:   source,
		Values:   values,
		Defaults: models.DefaultFreshnessSettingsValues(),
	}
}

// validateRule rejects a rule definition, including a project outside the team.
func (s *FreshnessService) validateRule(
	ctx context.Context, userID, teamID string, input FreshnessRuleInput,
) error {
	if err := ValidateFreshnessRuleInput(input); err != nil {
		return err
	}
	if input.ProjectID == nil {
		return nil
	}

	// The schema has a plain FK to projects(id) with no team predicate, so
	// without this check a rule could scope itself to another team's project.
	project, err := s.projects.GetByID(ctx, userID, *input.ProjectID)

	// A missing project is the caller's mistake; anything else is ours. Folding
	// the two together would report a database outage as "your project_id is
	// wrong" — a 400 the caller can never satisfy, with the real cause invisible.
	if err != nil && !errors.Is(err, repositories.ErrProjectNotFoundForRepo) {
		return fmt.Errorf("FreshnessService.validateRule: %w", err)
	}
	if err != nil || project == nil || project.TeamID != teamID {
		return fmt.Errorf("%w: project_id does not belong to this team", ErrInvalidFreshnessRule)
	}
	return nil
}

// ValidateFreshnessRuleInput is the pure rule validator, shared by create and
// update so both reject identically.
func ValidateFreshnessRuleInput(input FreshnessRuleInput) error {
	if input.ThresholdDays <= 0 {
		return fmt.Errorf("%w: threshold_days must be greater than 0", ErrInvalidFreshnessRule)
	}
	if input.ThresholdDays > MaxFreshnessThresholdDays {
		return fmt.Errorf(
			"%w: threshold_days must be at most %d", ErrInvalidFreshnessRule, MaxFreshnessThresholdDays)
	}
	if len(input.ResourceTypes) == 0 {
		return fmt.Errorf("%w: resource_types must not be empty", ErrInvalidFreshnessRule)
	}
	for _, resourceType := range input.ResourceTypes {
		if !slices.Contains(freshnessRuleResourceTypes, resourceType) {
			return fmt.Errorf(
				"%w: resource_types contains unsupported value %q", ErrInvalidFreshnessRule, resourceType)
		}
	}
	for _, medium := range input.Mediums {
		if !slices.Contains(freshnessRuleMediums, medium) {
			return fmt.Errorf("%w: mediums contains unsupported value %q", ErrInvalidFreshnessRule, medium)
		}
	}
	return nil
}

// ValidateFreshnessSettings is the pure settings validator. The floor mirrors
// the CHECK constraint on team_freshness_settings, so a rejected value never
// reaches the database as a constraint violation.
func ValidateFreshnessSettings(values models.FreshnessSettingsValues) error {
	if values.IntervalSeconds < models.MinFreshnessIntervalSeconds {
		return fmt.Errorf(
			"%w: interval_seconds must be at least %d seconds (one hour)",
			ErrInvalidFreshnessSettings, models.MinFreshnessIntervalSeconds,
		)
	}
	if values.IntervalSeconds > MaxFreshnessIntervalSeconds {
		return fmt.Errorf(
			"%w: interval_seconds must be at most %d seconds (365 days)",
			ErrInvalidFreshnessSettings, MaxFreshnessIntervalSeconds,
		)
	}
	return nil
}
