package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// The search, AI summary and freshness settings in the instance audit log
// (#1520). None of them holds a secret.

const settingsAuditTeam = "team-1"

var errSettingsStore = errors.New("store down")

// instanceSearchDefaults answers the instance search defaults.
type instanceSearchDefaults struct{ InstanceSearchSettingsReader }

func (instanceSearchDefaults) Get(context.Context) (*models.InstanceSearchSettingsView, error) {
	return &models.InstanceSearchSettingsView{Values: models.InstanceSearchSettingsValues{
		RecencyRankingEnabled: true, RankWeightRelevance: 0.6, RankWeightCreated: 0.2,
		RankWeightUpdated: 0.2, RankHalfLifeDays: 90,
	}}, nil
}

// unreadableInstanceSearch fails every read of the instance search defaults.
type unreadableInstanceSearch struct{ InstanceSearchSettingsReader }

func (unreadableInstanceSearch) Get(context.Context) (*models.InstanceSearchSettingsView, error) {
	return nil, errSettingsStore
}

// unreadableInstanceAISummary fails every read of the instance AI summary
// defaults.
type unreadableInstanceAISummary struct {
	InstanceAISummarySettingsReader
}

func (unreadableInstanceAISummary) Get(context.Context) (*models.InstanceAISummarySettingsView, error) {
	return nil, errSettingsStore
}

// instanceAISummaryDefaults answers the instance AI summary defaults.
type instanceAISummaryDefaults struct {
	InstanceAISummarySettingsReader
}

func (instanceAISummaryDefaults) Get(context.Context) (*models.InstanceAISummarySettingsView, error) {
	return &models.InstanceAISummarySettingsView{Values: models.InstanceAISummarySettingsValues{
		Enabled: true, TopN: 5, Style: "concise", MaxOutputTokens: 400,
	}}, nil
}

func searchAuditProfile() models.TeamSearchSettingsValues {
	return models.TeamSearchSettingsValues{
		RecencyRankingEnabled: true, RankWeightRelevance: 0.6,
		RankWeightCreated: 0.2, RankWeightUpdated: 0.2, RankHalfLifeDays: 30,
	}
}

func newSearchAuditService(
	t *testing.T, authorizer AuthorizationServiceInterface,
) (*TeamSearchSettingsService, *repomocks.MockTeamSearchSettingsRepository, *auditEvents) {
	t.Helper()
	repo := repomocks.NewMockTeamSearchSettingsRepository(t)
	events := &auditEvents{}
	svc := NewTeamSearchSettingsService(repo, authorizer, instanceSearchDefaults{}, slog.New(slog.DiscardHandler),
		WithTeamSearchSettingsAuditRecorder(events))
	return svc, repo, events
}

func TestTeamSearchSettingsAudit_UpdateRecordsOldAndNew(t *testing.T) {
	svc, repo, events := newSearchAuditService(t, allowAllAuthz{})
	repo.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(&models.TeamSearchSettings{
		TeamID: settingsAuditTeam, RecencyRankingEnabled: true, RankWeightRelevance: 0.5,
		RankWeightCreated: 0.3, RankWeightUpdated: 0.2, RankHalfLifeDays: 30,
	}, nil)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)

	_, err := svc.Update(context.Background(), settingsAuditActor, settingsAuditTeam, searchAuditProfile())

	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, settingsAuditTeam, teamSettingsSurfaceSearch, teamSettingsActionUpdate)
	assert.Equal(t, []string{"rank_weight_created", "rank_weight_relevance"}, changedFields(t, metadata))
	assert.Equal(t, map[string]any{"old": 0.5, "new": 0.6}, metadata["rank_weight_relevance"])
	assert.Equal(t, settingsAuditActor, *events.only(t).Actor.UserID)
}

// A team's first save compares against the instance defaults it inherited,
// so only what it actually changed is recorded (#1551).
func TestTeamSearchSettingsAudit_FirstSaveRecordsOnlyWhatChangedFromTheDefaults(t *testing.T) {
	svc, repo, events := newSearchAuditService(t, allowAllAuthz{})
	repo.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(nil, nil)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)
	profile := models.TeamSearchSettingsValues{
		RecencyRankingEnabled: true, RankWeightRelevance: 0.6, RankWeightCreated: 0.2,
		RankWeightUpdated: 0.2, RankHalfLifeDays: 45,
	}

	_, err := svc.Update(context.Background(), settingsAuditActor, settingsAuditTeam, profile)

	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, settingsAuditTeam, teamSettingsSurfaceSearch, teamSettingsActionUpdate)
	assert.Equal(t, []string{"rank_half_life_days"}, changedFields(t, metadata))
	assert.Equal(t, map[string]any{"old": 90.0, "new": 45.0}, metadata["rank_half_life_days"])
	assert.Equal(t, teamSettingsSourceInstanceDefault, metadata[teamSettingsMetaBeforeSource])
	assert.NotContains(t, metadata, teamSettingsMetaAfterSource)
}

func TestTeamSearchSettingsAudit_AnUpdateOfAStoredProfileIsNotInherited(t *testing.T) {
	svc, repo, events := newSearchAuditService(t, allowAllAuthz{})
	repo.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(&models.TeamSearchSettings{
		RecencyRankingEnabled: true, RankWeightRelevance: 0.6, RankWeightCreated: 0.2,
		RankWeightUpdated: 0.2, RankHalfLifeDays: 45,
	}, nil)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)

	_, err := svc.Update(context.Background(), settingsAuditActor, settingsAuditTeam, searchAuditProfile())

	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, settingsAuditTeam, teamSettingsSurfaceSearch, teamSettingsActionUpdate)
	assert.Equal(t, []string{"rank_half_life_days"}, changedFields(t, metadata))
	assert.Equal(t, map[string]any{"old": 45.0, "new": 30.0}, metadata["rank_half_life_days"])
	assert.NotContains(t, metadata, teamSettingsMetaBeforeSource)
}

func TestTeamSearchSettingsAudit_AFailedOldValueReadDoesNotBlockTheWrite(t *testing.T) {
	svc, repo, events := newSearchAuditService(t, allowAllAuthz{})
	repo.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(nil, errSettingsStore)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)

	_, err := svc.Update(context.Background(), settingsAuditActor, settingsAuditTeam, searchAuditProfile())

	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, settingsAuditTeam, teamSettingsSurfaceSearch, teamSettingsActionUpdate)
	assert.NotContains(t, metadata, teamSettingsMetaBeforeSource, "a failed read is not an inherited side")
}

// When the instance defaults cannot be read, the old side is unknown, never
// marked inherited; the write still succeeds (#1551).
func TestTeamSearchSettingsAudit_UnreadableDefaultsLeaveTheSideUnknown(t *testing.T) {
	repo := repomocks.NewMockTeamSearchSettingsRepository(t)
	events := &auditEvents{}
	svc := NewTeamSearchSettingsService(repo, allowAllAuthz{}, unreadableInstanceSearch{},
		slog.New(slog.DiscardHandler), WithTeamSearchSettingsAuditRecorder(events))
	repo.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(nil, nil)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)

	// Update reads the instance defaults again for its response and fails
	// closed there; the audit event is recorded before that.
	_, err := svc.Update(context.Background(), settingsAuditActor, settingsAuditTeam, searchAuditProfile())
	require.Error(t, err)
	metadata := requireTeamSettingsEvent(t, events, settingsAuditTeam, teamSettingsSurfaceSearch, teamSettingsActionUpdate)
	assert.Len(t, changedFields(t, metadata), 5)
	assert.Equal(t, map[string]any{"new": 30.0}, metadata["rank_half_life_days"])
	assert.NotContains(t, metadata, teamSettingsMetaBeforeSource)

	resetRepo := repomocks.NewMockTeamSearchSettingsRepository(t)
	resetEvents := &auditEvents{}
	resetSvc := NewTeamSearchSettingsService(resetRepo, allowAllAuthz{}, unreadableInstanceSearch{},
		slog.New(slog.DiscardHandler), WithTeamSearchSettingsAuditRecorder(resetEvents))
	resetRepo.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(&models.TeamSearchSettings{RankHalfLifeDays: 7}, nil)
	resetRepo.EXPECT().Delete(mock.Anything, settingsAuditTeam).Return(nil)
	require.NoError(t, resetSvc.Reset(context.Background(), settingsAuditActor, settingsAuditTeam))
	metadata = requireTeamSettingsEvent(t, resetEvents, settingsAuditTeam, teamSettingsSurfaceSearch,
		teamSettingsActionReset)
	assert.Equal(t, map[string]any{"old": 7.0}, metadata["rank_half_life_days"])
	assert.NotContains(t, metadata, teamSettingsMetaAfterSource)
}

// A reset's new side is the inherited defaults (#1551).
func TestTeamSearchSettingsAudit_ResetRecordsTheReturnToTheDefaults(t *testing.T) {
	svc, repo, events := newSearchAuditService(t, allowAllAuthz{})
	repo.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(&models.TeamSearchSettings{
		RecencyRankingEnabled: true, RankWeightRelevance: 0.6, RankWeightCreated: 0.2,
		RankWeightUpdated: 0.2, RankHalfLifeDays: 7,
	}, nil)
	repo.EXPECT().Delete(mock.Anything, settingsAuditTeam).Return(nil)

	require.NoError(t, svc.Reset(context.Background(), settingsAuditActor, settingsAuditTeam))

	metadata := requireTeamSettingsEvent(t, events, settingsAuditTeam, teamSettingsSurfaceSearch, teamSettingsActionReset)
	assert.Equal(t, []string{"rank_half_life_days"}, changedFields(t, metadata))
	assert.Equal(t, map[string]any{"old": 7.0, "new": 90.0}, metadata["rank_half_life_days"])
	assert.Equal(t, teamSettingsSourceInstanceDefault, metadata[teamSettingsMetaAfterSource])
	assert.NotContains(t, metadata, teamSettingsMetaBeforeSource)
}

func TestTeamSearchSettingsAudit_ResettingAnInheritingTeamChangesNothing(t *testing.T) {
	svc, repo, events := newSearchAuditService(t, allowAllAuthz{})
	repo.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(nil, nil)
	repo.EXPECT().Delete(mock.Anything, settingsAuditTeam).Return(nil)

	require.NoError(t, svc.Reset(context.Background(), settingsAuditActor, settingsAuditTeam))

	metadata := requireTeamSettingsEvent(t, events, settingsAuditTeam, teamSettingsSurfaceSearch, teamSettingsActionReset)
	assert.Empty(t, changedFields(t, metadata))
}

func TestTeamSearchSettingsAudit_NothingOnFailureOrDenial(t *testing.T) {
	svc, repo, events := newSearchAuditService(t, allowAllAuthz{})
	repo.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(nil, nil)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(errSettingsStore)
	repo.EXPECT().Delete(mock.Anything, settingsAuditTeam).Return(errSettingsStore)

	_, err := svc.Update(context.Background(), settingsAuditActor, settingsAuditTeam, searchAuditProfile())
	require.Error(t, err)
	require.Error(t, svc.Reset(context.Background(), settingsAuditActor, settingsAuditTeam))

	denied, _, deniedEvents := newSearchAuditService(t, denyAllAuthz{})
	_, err = denied.Update(context.Background(), settingsAuditActor, settingsAuditTeam, searchAuditProfile())
	require.ErrorIs(t, err, ErrPermissionDenied)

	assert.Empty(t, events.got)
	assert.Empty(t, deniedEvents.got)
}

func TestTeamSearchSettingsAudit_WithoutARecorderThereIsNoExtraRead(t *testing.T) {
	repo := repomocks.NewMockTeamSearchSettingsRepository(t)
	svc := NewTeamSearchSettingsService(repo, allowAllAuthz{}, instanceSearchDefaults{}, slog.New(slog.DiscardHandler))
	// The strict mock fails the test on an unexpected Get.
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)
	repo.EXPECT().Delete(mock.Anything, settingsAuditTeam).Return(nil)

	_, err := svc.Update(context.Background(), settingsAuditActor, settingsAuditTeam, searchAuditProfile())
	require.NoError(t, err)
	require.NoError(t, svc.Reset(context.Background(), settingsAuditActor, settingsAuditTeam))
}

func newAISummaryAuditService(
	t *testing.T, authorizer AuthorizationServiceInterface,
) (*TeamAISummarySettingsService, *repomocks.MockTeamAISummarySettingsRepository, *auditEvents) {
	t.Helper()
	repo := repomocks.NewMockTeamAISummarySettingsRepository(t)
	providers := repomocks.NewMockModelProviderRepository(t)
	providers.EXPECT().Count(mock.Anything, settingsAuditTeam).Return(1, nil).Maybe()
	events := &auditEvents{}
	svc := NewTeamAISummarySettingsService(repo, providers, authorizer, instanceAISummaryDefaults{},
		slog.New(slog.DiscardHandler), WithTeamAISummarySettingsAuditRecorder(events))
	return svc, repo, events
}

func aiSummaryAuditValues() models.TeamAISummarySettingsValues {
	return models.TeamAISummarySettingsValues{Enabled: true, TopN: 5, Style: "concise", MaxOutputTokens: 400}
}

func TestTeamAISummarySettingsAudit_UpdateAndReset(t *testing.T) {
	svc, repo, events := newAISummaryAuditService(t, allowAllAuthz{})
	repo.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(&models.TeamAISummarySettings{
		Enabled: false, TopN: 5, Style: "concise", MaxOutputTokens: 400,
	}, nil)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)

	_, err := svc.Update(context.Background(), settingsAuditActor, settingsAuditTeam, aiSummaryAuditValues())

	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, settingsAuditTeam, teamSettingsSurfaceAISummary, teamSettingsActionUpdate)
	assert.Equal(t, []string{"enabled"}, changedFields(t, metadata))
	assert.Equal(t, map[string]any{"old": false, "new": true}, metadata["enabled"])

	resetSvc, resetRepo, resetEvents := newAISummaryAuditService(t, allowAllAuthz{})
	resetRepo.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(&models.TeamAISummarySettings{
		Enabled: true, TopN: 8, Style: "concise", MaxOutputTokens: 400,
	}, nil)
	resetRepo.EXPECT().Delete(mock.Anything, settingsAuditTeam).Return(nil)
	require.NoError(t, resetSvc.Reset(context.Background(), settingsAuditActor, settingsAuditTeam))
	metadata = requireTeamSettingsEvent(t, resetEvents, settingsAuditTeam, teamSettingsSurfaceAISummary,
		teamSettingsActionReset)
	assert.Equal(t, []string{"top_n"}, changedFields(t, metadata))
	assert.Equal(t, map[string]any{"old": 8, "new": 5}, metadata["top_n"])
	assert.Equal(t, teamSettingsSourceInstanceDefault, metadata[teamSettingsMetaAfterSource])
}

// A team's first AI summary save compares against the inherited defaults (#1551).
func TestTeamAISummarySettingsAudit_FirstSaveRecordsOnlyWhatChangedFromTheDefaults(t *testing.T) {
	svc, repo, events := newAISummaryAuditService(t, allowAllAuthz{})
	repo.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(nil, nil)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)
	values := aiSummaryAuditValues()
	values.TopN = 8

	_, err := svc.Update(context.Background(), settingsAuditActor, settingsAuditTeam, values)

	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, settingsAuditTeam, teamSettingsSurfaceAISummary, teamSettingsActionUpdate)
	assert.Equal(t, []string{"top_n"}, changedFields(t, metadata))
	assert.Equal(t, map[string]any{"old": 5, "new": 8}, metadata["top_n"])
	assert.Equal(t, teamSettingsSourceInstanceDefault, metadata[teamSettingsMetaBeforeSource])
}

// When the instance defaults cannot be read, neither side is marked inherited
// (#1551).
func TestTeamAISummarySettingsAudit_UnreadableDefaultsLeaveTheSideUnknown(t *testing.T) {
	newService := func() (*TeamAISummarySettingsService, *repomocks.MockTeamAISummarySettingsRepository, *auditEvents) {
		repo := repomocks.NewMockTeamAISummarySettingsRepository(t)
		providers := repomocks.NewMockModelProviderRepository(t)
		providers.EXPECT().Count(mock.Anything, settingsAuditTeam).Return(1, nil).Maybe()
		events := &auditEvents{}
		return NewTeamAISummarySettingsService(repo, providers, allowAllAuthz{}, unreadableInstanceAISummary{},
			slog.New(slog.DiscardHandler), WithTeamAISummarySettingsAuditRecorder(events)), repo, events
	}

	svc, repo, events := newService()
	repo.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(nil, nil)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)
	_, err := svc.Update(context.Background(), settingsAuditActor, settingsAuditTeam, aiSummaryAuditValues())
	require.Error(t, err, "the response's own read of the defaults fails closed")
	metadata := requireTeamSettingsEvent(t, events, settingsAuditTeam, teamSettingsSurfaceAISummary, teamSettingsActionUpdate)
	assert.Equal(t, map[string]any{"new": 5}, metadata["top_n"])
	assert.NotContains(t, metadata, teamSettingsMetaBeforeSource)

	resetSvc, resetRepo, resetEvents := newService()
	resetRepo.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(&models.TeamAISummarySettings{TopN: 8}, nil)
	resetRepo.EXPECT().Delete(mock.Anything, settingsAuditTeam).Return(nil)
	require.NoError(t, resetSvc.Reset(context.Background(), settingsAuditActor, settingsAuditTeam))
	metadata = requireTeamSettingsEvent(t, resetEvents, settingsAuditTeam, teamSettingsSurfaceAISummary,
		teamSettingsActionReset)
	assert.Equal(t, map[string]any{"old": 8}, metadata["top_n"])
	assert.NotContains(t, metadata, teamSettingsMetaAfterSource)
}

func TestTeamAISummarySettingsAudit_NothingOnFailureOrDenial(t *testing.T) {
	svc, repo, events := newAISummaryAuditService(t, allowAllAuthz{})
	repo.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(nil, errSettingsStore)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(errSettingsStore)
	repo.EXPECT().Delete(mock.Anything, settingsAuditTeam).Return(errSettingsStore)

	_, err := svc.Update(context.Background(), settingsAuditActor, settingsAuditTeam, aiSummaryAuditValues())
	require.Error(t, err)
	require.Error(t, svc.Reset(context.Background(), settingsAuditActor, settingsAuditTeam))

	denied, _, deniedEvents := newAISummaryAuditService(t, denyAllAuthz{})
	require.ErrorIs(t, denied.Reset(context.Background(), settingsAuditActor, settingsAuditTeam), ErrPermissionDenied)

	assert.Empty(t, events.got)
	assert.Empty(t, deniedEvents.got)
}

type freshnessAuditFixture struct {
	svc      *FreshnessService
	rules    *repomocks.MockFreshnessRuleRepository
	settings *repomocks.MockTeamFreshnessSettingsRepository
	events   *auditEvents
}

func newFreshnessAuditFixture(t *testing.T, authorizer AuthorizationServiceInterface) *freshnessAuditFixture {
	t.Helper()
	f := &freshnessAuditFixture{
		rules:    repomocks.NewMockFreshnessRuleRepository(t),
		settings: repomocks.NewMockTeamFreshnessSettingsRepository(t),
		events:   &auditEvents{},
	}
	schedules := repomocks.NewMockScheduleRepository(t)
	// The schedule sync is best-effort and not under test here.
	f.rules.EXPECT().ListByTeam(mock.Anything, settingsAuditTeam, mock.Anything).Return(nil, nil).Maybe()
	f.settings.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(nil, nil).Maybe()
	schedules.EXPECT().Delete(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	schedules.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil).Maybe()
	f.svc = NewFreshnessService(FreshnessServiceDeps{
		Rules: f.rules, Settings: f.settings, Schedules: schedules,
		Freshness: repomocks.NewMockResourceFreshnessRepository(t),
		Authz:     authorizer, Logger: slog.New(slog.DiscardHandler), AuditRecorder: f.events,
	})
	return f
}

func freshnessAuditInput() FreshnessRuleInput {
	return FreshnessRuleInput{ResourceTypes: []string{"prompt", "memory"}, Mediums: []string{"web"},
		ThresholdDays: 30, Enabled: true}
}

func TestFreshnessAudit_RuleCreateUpdateDelete(t *testing.T) {
	f := newFreshnessAuditFixture(t, allowAllAuthz{})
	f.rules.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, r *models.FreshnessRule) { r.ID = "rule-1" }).Return(nil)
	_, err := f.svc.CreateRule(context.Background(), settingsAuditActor, settingsAuditTeam, freshnessAuditInput())
	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, f.events, settingsAuditTeam, teamSettingsSurfaceFreshness, teamSettingsActionCreate)
	assert.Equal(t, "rule-1", metadata["rule_id"])
	assert.Equal(t, map[string]any{"new": "prompt, memory"}, metadata["resource_types"])

	f = newFreshnessAuditFixture(t, allowAllAuthz{})
	existing := &models.FreshnessRule{ID: "rule-1", TeamID: settingsAuditTeam,
		ResourceTypes: []string{"prompt", "memory"}, Mediums: []string{"web"}, ThresholdDays: 14, Enabled: true}
	f.rules.EXPECT().GetByID(mock.Anything, settingsAuditTeam, "rule-1").Return(existing, nil)
	f.rules.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)
	_, err = f.svc.UpdateRule(context.Background(), settingsAuditActor, settingsAuditTeam, "rule-1", freshnessAuditInput())
	require.NoError(t, err)
	metadata = requireTeamSettingsEvent(t, f.events, settingsAuditTeam, teamSettingsSurfaceFreshness, teamSettingsActionUpdate)
	assert.Equal(t, []string{"threshold_days"}, changedFields(t, metadata))
	assert.Equal(t, map[string]any{"old": 14, "new": 30}, metadata["threshold_days"])

	f = newFreshnessAuditFixture(t, allowAllAuthz{})
	f.rules.EXPECT().GetByID(mock.Anything, settingsAuditTeam, "rule-1").Return(existing, nil)
	f.svc.freshness.(*repomocks.MockResourceFreshnessRepository).EXPECT().
		RemoveRule(mock.Anything, "rule-1").Return(0, nil)
	f.rules.EXPECT().Delete(mock.Anything, settingsAuditTeam, "rule-1").Return(true, nil)
	require.NoError(t, f.svc.DeleteRule(context.Background(), settingsAuditActor, settingsAuditTeam, "rule-1"))
	metadata = requireTeamSettingsEvent(t, f.events, settingsAuditTeam, teamSettingsSurfaceFreshness, teamSettingsActionDelete)
	assert.Equal(t, map[string]any{"old": 14}, metadata["threshold_days"])
}

func TestFreshnessAudit_SettingsUpdateAndReset(t *testing.T) {
	defaults := models.DefaultFreshnessSettingsValues()

	// First save: the fixture's settings read finds no row, so the old side is
	// the inherited defaults and only the interval changed (#1551).
	f := newFreshnessAuditFixture(t, allowAllAuthz{})
	f.settings.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)
	_, err := f.svc.UpdateSettings(context.Background(), settingsAuditActor, settingsAuditTeam,
		models.FreshnessSettingsValues{IntervalSeconds: 7200, ReversibilityEnabled: defaults.ReversibilityEnabled})
	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, f.events, settingsAuditTeam, teamSettingsSurfaceFreshness, teamSettingsActionUpdate)
	assert.Equal(t, []string{"interval_seconds"}, changedFields(t, metadata))
	assert.Equal(t, map[string]any{"old": defaults.IntervalSeconds, "new": 7200}, metadata["interval_seconds"])
	assert.Equal(t, teamSettingsSourceInstanceDefault, metadata[teamSettingsMetaBeforeSource])

	// Reset of an inheriting team: both sides are the defaults, nothing changed.
	f = newFreshnessAuditFixture(t, allowAllAuthz{})
	f.settings.EXPECT().Delete(mock.Anything, settingsAuditTeam).Return(nil)
	require.NoError(t, f.svc.ResetSettings(context.Background(), settingsAuditActor, settingsAuditTeam))
	metadata = requireTeamSettingsEvent(t, f.events, settingsAuditTeam, teamSettingsSurfaceFreshness, teamSettingsActionReset)
	assert.Empty(t, changedFields(t, metadata))
	assert.Equal(t, teamSettingsSourceInstanceDefault, metadata[teamSettingsMetaAfterSource])
}

func TestFreshnessAudit_ResetOfStoredSettingsRecordsTheReturnToTheDefaults(t *testing.T) {
	defaults := models.DefaultFreshnessSettingsValues()
	settings := repomocks.NewMockTeamFreshnessSettingsRepository(t)
	settings.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(&models.TeamFreshnessSettings{
		IntervalSeconds: 7200, ReversibilityEnabled: defaults.ReversibilityEnabled,
	}, nil).Once()
	settings.EXPECT().Delete(mock.Anything, settingsAuditTeam).Return(nil)
	settings.EXPECT().Get(mock.Anything, settingsAuditTeam).Return(nil, nil).Maybe()
	f := newFreshnessAuditFixture(t, allowAllAuthz{})
	f.svc.settings = settings

	require.NoError(t, f.svc.ResetSettings(context.Background(), settingsAuditActor, settingsAuditTeam))

	metadata := requireTeamSettingsEvent(t, f.events, settingsAuditTeam, teamSettingsSurfaceFreshness, teamSettingsActionReset)
	assert.Equal(t, map[string]any{"old": 7200, "new": defaults.IntervalSeconds}, metadata["interval_seconds"])
	assert.NotContains(t, metadata, teamSettingsMetaBeforeSource)
}

func TestFreshnessAudit_NothingOnFailureOrDenial(t *testing.T) {
	f := newFreshnessAuditFixture(t, allowAllAuthz{})
	f.rules.EXPECT().Create(mock.Anything, mock.Anything).Return(errSettingsStore)
	f.rules.EXPECT().GetByID(mock.Anything, settingsAuditTeam, "rule-1").Return(nil, nil)
	f.rules.EXPECT().Update(mock.Anything, mock.Anything).Return(repositories.ErrFreshnessRuleNotFound)
	f.settings.EXPECT().Upsert(mock.Anything, mock.Anything).Return(errSettingsStore)
	f.settings.EXPECT().Delete(mock.Anything, settingsAuditTeam).Return(errSettingsStore)

	ctx := context.Background()
	_, err := f.svc.CreateRule(ctx, settingsAuditActor, settingsAuditTeam, freshnessAuditInput())
	require.Error(t, err)
	_, err = f.svc.UpdateRule(ctx, settingsAuditActor, settingsAuditTeam, "rule-1", freshnessAuditInput())
	require.Error(t, err)
	require.Error(t, f.svc.DeleteRule(ctx, settingsAuditActor, settingsAuditTeam, "rule-1"))
	_, err = f.svc.UpdateSettings(ctx, settingsAuditActor, settingsAuditTeam,
		models.FreshnessSettingsValues{IntervalSeconds: 7200})
	require.Error(t, err)
	require.Error(t, f.svc.ResetSettings(ctx, settingsAuditActor, settingsAuditTeam))

	denied := newFreshnessAuditFixture(t, denyAllAuthz{})
	require.ErrorIs(t, denied.svc.ResetSettings(ctx, settingsAuditActor, settingsAuditTeam), ErrPermissionDenied)

	assert.Empty(t, f.events.got)
	assert.Empty(t, denied.events.got)
}
