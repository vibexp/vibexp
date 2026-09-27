package services_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
)

const (
	testAISummaryUserID = "22222222-3333-4444-5555-666666666666"
	// aiSummaryTestProviderID is the provider aiSummaryTeamProfile points at;
	// Update looks it up to prove the team owns it.
	aiSummaryTestProviderID = "33333333-4444-5555-6666-777777777777"
)

// aiSummaryInstanceValues are the instance defaults the tests resolve: the
// built-in ones, which enable summaries with top_n 5, balanced, 800 tokens.
func aiSummaryInstanceValues() models.InstanceAISummarySettingsValues {
	return models.DefaultInstanceAISummarySettings()
}

// fakeAISummaryInstance reads whatever values it currently holds, so a test
// can change the instance defaults between two calls — the stand-in for an
// instance admin saving new ones — and observe the next call pick them up.
// getErr fails the fail-closed Get only; Resolve fails open by contract, so it
// always answers.
type fakeAISummaryInstance struct {
	values models.InstanceAISummarySettingsValues
	getErr error
}

func (f *fakeAISummaryInstance) Resolve(context.Context) models.InstanceAISummarySettingsValues {
	return f.values
}

func (f *fakeAISummaryInstance) Get(context.Context) (*models.InstanceAISummarySettingsView, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &models.InstanceAISummarySettingsView{
		Source: models.InstanceAISummarySettingsSourceInstance, Values: f.values,
	}, nil
}

// aiSummaryTeamProfile is deliberately different from the instance defaults in
// every field, so a test can tell which one came back.
func aiSummaryTeamProfile() models.TeamAISummarySettingsValues {
	providerID := aiSummaryTestProviderID
	return models.TeamAISummarySettingsValues{
		Enabled:         false,
		ModelProviderID: &providerID,
		TopN:            8,
		Style:           models.AISummaryStyleDetailed,
		MaxOutputTokens: 1200,
	}
}

func newAISummarySettingsService(
	t *testing.T, authzSvc services.AuthorizationServiceInterface, logs *bytes.Buffer,
) (
	*services.TeamAISummarySettingsService,
	*repomocks.MockTeamAISummarySettingsRepository,
	*repomocks.MockModelProviderRepository,
) {
	t.Helper()
	return newAISummarySettingsServiceWithInstance(
		t, authzSvc, logs, &fakeAISummaryInstance{values: aiSummaryInstanceValues()})
}

func newAISummarySettingsServiceWithInstance(
	t *testing.T, authzSvc services.AuthorizationServiceInterface, logs *bytes.Buffer,
	instance services.InstanceAISummarySettingsReader,
) (
	*services.TeamAISummarySettingsService,
	*repomocks.MockTeamAISummarySettingsRepository,
	*repomocks.MockModelProviderRepository,
) {
	t.Helper()
	repo := repomocks.NewMockTeamAISummarySettingsRepository(t)
	providers := repomocks.NewMockModelProviderRepository(t)
	handler := slog.Handler(slog.DiscardHandler)
	if logs != nil {
		handler = slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	return services.NewTeamAISummarySettingsService(
		repo, providers, authzSvc, instance, slog.New(handler)), repo, providers
}

// expectProviderOwnedByTeam stubs the tenancy lookup Update runs on a non-nil
// model_provider_id.
func expectProviderOwnedByTeam(providers *repomocks.MockModelProviderRepository) {
	teamID, providerID := testTeamID, aiSummaryTestProviderID
	providers.EXPECT().GetByID(mock.Anything, teamID, providerID).
		Return(&models.ModelProvider{ID: providerID, TeamID: &teamID}, nil)
}

// expectProviderCount stubs the D9 availability check (Get/Resolve/Update all
// run it: "available" is true iff the team has at least one provider row).
func expectProviderCount(providers *repomocks.MockModelProviderRepository, count int) {
	providers.EXPECT().Count(mock.Anything, testTeamID).Return(count, nil)
}

func TestTeamAISummarySettingsService_Resolve_NoRowReportsInstanceSource(t *testing.T) {
	svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil)
	expectProviderCount(providers, 1)

	view, err := svc.Resolve(context.Background(), testTeamID)

	require.NoError(t, err)
	assert.Equal(t, models.TeamAISummarySettingsSourceInstance, view.Source)
	assert.Equal(t, view.InstanceDefaults, view.Values,
		"with no override the effective values ARE the instance defaults")
	assert.True(t, view.Values.Enabled)
	assert.Equal(t, 5, view.Values.TopN)
	assert.Equal(t, models.AISummaryStyleBalanced, view.Values.Style)
	assert.Nil(t, view.Values.ModelProviderID, "the instance has no opinion on which provider to use")
	assert.Equal(t, models.MaxAISummaryTopN, view.MaxTopN, "the deprecated field reports the hard limit")
	assert.Equal(t, models.MaxAISummaryOutputTokens, view.MaxOutputTokensCeiling,
		"the deprecated field reports the hard limit")
	assert.True(t, view.Available)
}

func TestTeamAISummarySettingsService_Resolve_StoredRowReportsTeamSource(t *testing.T) {
	svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	providerID := "provider-9"
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(&models.TeamAISummarySettings{
		TeamID:          testTeamID,
		Enabled:         false,
		ModelProviderID: &providerID,
		TopN:            8,
		Style:           models.AISummaryStyleDetailed,
		MaxOutputTokens: 1200,
	}, nil)
	expectProviderCount(providers, 1)

	view, err := svc.Resolve(context.Background(), testTeamID)

	require.NoError(t, err)
	assert.Equal(t, models.TeamAISummarySettingsSourceTeam, view.Source)
	assert.False(t, view.Values.Enabled)
	assert.Equal(t, 8, view.Values.TopN)
	assert.Equal(t, models.AISummaryStyleDetailed, view.Values.Style)
	require.NotNil(t, view.Values.ModelProviderID)
	assert.Equal(t, providerID, *view.Values.ModelProviderID)
	assert.Equal(t, 5, view.InstanceDefaults.TopN,
		"instance_defaults must keep reporting the deployment values, not the team's")
	assert.Equal(t, models.MaxAISummaryTopN, view.MaxTopN)
}

// The resolver FAILS OPEN: a settings read failure degrades the tuning rather
// than failing the request that asked for a summary.
func TestTeamAISummarySettingsService_Resolve_RepositoryErrorFailsOpen(t *testing.T) {
	var logs bytes.Buffer
	svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, &logs)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, errors.New("connection refused"))
	expectProviderCount(providers, 1)

	view, err := svc.Resolve(context.Background(), testTeamID)

	require.NoError(t, err, "a settings read failure must not surface as an error")
	assert.Equal(t, models.TeamAISummarySettingsSourceInstance, view.Source)
	assert.Equal(t, view.InstanceDefaults, view.Values)
	assertAISummaryWarnLogged(t, logs.String())
}

// A provider-count outage must ALSO fail open, exactly like a settings-row
// outage: Resolve's whole contract is "the error is always nil".
func TestTeamAISummarySettingsService_Resolve_ProviderCountErrorFailsOpen(t *testing.T) {
	var logs bytes.Buffer
	svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, &logs)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil)
	providers.EXPECT().Count(mock.Anything, testTeamID).Return(0, errors.New("connection refused"))

	view, err := svc.Resolve(context.Background(), testTeamID)

	require.NoError(t, err)
	assert.False(t, view.Available, "a provider-count outage must degrade to unavailable, not panic or lie true")
	assertAISummaryWarnLogged(t, logs.String())
}

// assertAISummaryWarnLogged pins the observability contract: the fail-open path
// must be greppable, so it logs at warn and carries team_id. Logging it at debug
// would hide a real outage behind silently-default tuning.
func assertAISummaryWarnLogged(t *testing.T, output string) {
	t.Helper()
	require.NotEmpty(t, output, "fail-open must emit a log line")

	var found bool
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["level"] == "WARN" && entry["team_id"] == testTeamID {
			found = true
		}
	}
	assert.True(t, found, "expected a WARN log carrying team_id=%s, got: %s", testTeamID, output)
}

// The fail-open read must NOT be reachable from the settings API's interface —
// that separation is what keeps #1072 from serving instance defaults as fact
// during an outage, and prose alone could not enforce it.
//
// Note what does NOT catch a regression here: the compiler. Putting Resolve back
// on TeamAISummarySettingsServiceInterface still builds, because the one service
// implements everything on both interfaces. The assert.False below is the whole
// guard.
func TestTeamAISummarySettings_ResolveIsOffTheSettingsInterface(t *testing.T) {
	// Reflect over the INTERFACE TYPES, not over a value: a runtime type
	// assertion would go through the concrete service, which implements both,
	// and would therefore pass however the interfaces were shaped.
	settingsIface := reflect.TypeOf((*services.TeamAISummarySettingsServiceInterface)(nil)).Elem()
	resolverIface := reflect.TypeOf((*services.AISummarySettingsResolver)(nil)).Elem()

	_, hasResolve := settingsIface.MethodByName("Resolve")
	assert.False(t, hasResolve,
		"a handler holding TeamAISummarySettingsServiceInterface must not be able to reach the fail-open read")
	_, hasGet := settingsIface.MethodByName("Get")
	assert.True(t, hasGet, "the settings API's authoritative read must stay on its interface")
	_, resolverHasResolve := resolverIface.MethodByName("Resolve")
	assert.True(t, resolverHasResolve)

	// And the one service still satisfies both, so the split costs no wiring.
	svc, _, _ := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	assert.Implements(t, (*services.TeamAISummarySettingsServiceInterface)(nil), svc)
	assert.Implements(t, (*services.AISummarySettingsResolver)(nil), svc)
}

// Get is the settings API's read and must NOT fail open: reporting the instance
// defaults during an outage would present a guess as fact.
func TestTeamAISummarySettingsService_Get_RepositoryErrorPropagates(t *testing.T) {
	svc, repo, _ := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, errors.New("boom"))

	_, err := svc.Get(context.Background(), testTeamID)

	assert.Error(t, err, "unlike Resolve, the settings API read must surface a failed read")
}

func TestTeamAISummarySettingsService_Get_NoRowReportsInstanceSource(t *testing.T) {
	svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil)
	expectProviderCount(providers, 0)

	view, err := svc.Get(context.Background(), testTeamID)

	require.NoError(t, err)
	assert.Equal(t, models.TeamAISummarySettingsSourceInstance, view.Source)
	assert.Equal(t, view.InstanceDefaults, view.Values)
	assert.Equal(t, models.MaxAISummaryTopN, view.MaxTopN)
	assert.Equal(t, models.MaxAISummaryOutputTokens, view.MaxOutputTokensCeiling)
	assert.False(t, view.Available, "zero provider rows must report unavailable")
}

func TestTeamAISummarySettingsService_Get_StoredRowReportsTeamSource(t *testing.T) {
	svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(&models.TeamAISummarySettings{
		TeamID:          testTeamID,
		TopN:            8,
		Style:           models.AISummaryStyleConcise,
		MaxOutputTokens: 300,
	}, nil)
	expectProviderCount(providers, 2)

	view, err := svc.Get(context.Background(), testTeamID)

	require.NoError(t, err)
	assert.Equal(t, models.TeamAISummarySettingsSourceTeam, view.Source)
	assert.Equal(t, models.AISummaryStyleConcise, view.Values.Style)
	assert.Equal(t, models.AISummaryStyleBalanced, view.InstanceDefaults.Style,
		"instance_defaults must keep reporting the deployment values, not the team's")
	assert.True(t, view.Available)
}

// Get must NOT fail open on a provider-count error either — same reasoning as
// the settings-row read: a guess must not be reported as fact.
func TestTeamAISummarySettingsService_Get_ProviderCountErrorPropagates(t *testing.T) {
	svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil)
	providers.EXPECT().Count(mock.Anything, testTeamID).Return(0, errors.New("boom"))

	_, err := svc.Get(context.Background(), testTeamID)

	assert.Error(t, err)
}

// The column's FK references model_providers(id) alone, so it proves the
// provider EXISTS, not that this team owns it. Without the service check a team
// admin could point their summaries at another team's provider — and through
// #1073 at that team's base_url and encrypted API key.
func TestTeamAISummarySettingsService_Update_RejectsProviderOwnedByAnotherTeam(t *testing.T) {
	// No repo expectations: a cross-team reference must never reach storage.
	svc, _, providers := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	providers.EXPECT().GetByID(mock.Anything, testTeamID, aiSummaryTestProviderID).
		Return(nil, repositories.ErrModelProviderNotFound)

	_, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, aiSummaryTeamProfile())

	assert.ErrorIs(t, err, services.ErrInvalidAISummarySettings)
	assert.Contains(t, err.Error(), "model_provider_id")
}

// A nil provider means "use the team default", so it needs no lookup at all —
// asserting the mock was never called is what proves we do not pay a query for
// the common case.
func TestTeamAISummarySettingsService_Update_NilProviderSkipsTheLookup(t *testing.T) {
	svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)
	expectProviderCount(providers, 0)

	values := aiSummaryTeamProfile()
	values.ModelProviderID = nil

	_, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, values)

	require.NoError(t, err)
	providers.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything, mock.Anything)
}

// A provider lookup that fails for a reason OTHER than "not found" is an
// infrastructure error, not a rejected profile: it must not be dressed up as a
// 400 telling the admin their provider id is wrong.
func TestTeamAISummarySettingsService_Update_ProviderLookupErrorIsNotAValidationError(t *testing.T) {
	svc, _, providers := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	providers.EXPECT().GetByID(mock.Anything, testTeamID, aiSummaryTestProviderID).
		Return(nil, errors.New("connection refused"))

	_, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, aiSummaryTeamProfile())

	require.Error(t, err)
	assert.NotErrorIs(t, err, services.ErrInvalidAISummarySettings)
}

func TestTeamAISummarySettingsService_Update_StoresAndReportsTeamSource(t *testing.T) {
	svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	expectProviderOwnedByTeam(providers)
	repo.EXPECT().Upsert(mock.Anything, mock.MatchedBy(func(s *models.TeamAISummarySettings) bool {
		return s.TeamID == testTeamID && s.TopN == 8 && s.Style == models.AISummaryStyleDetailed
	})).Return(nil)
	expectProviderCount(providers, 1)

	view, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, aiSummaryTeamProfile())

	require.NoError(t, err)
	assert.Equal(t, models.TeamAISummarySettingsSourceTeam, view.Source)
	assert.Equal(t, 8, view.Values.TopN)
	assert.Equal(t, models.MaxAISummaryTopN, view.MaxTopN, "the deprecated field reports the hard limit")
	assert.True(t, view.Available)
}

// Since #1199 a team is bounded by the hard limits alone: top_n = 11 is
// rejected whatever the instance defaults say.
func TestTeamAISummarySettingsService_Update_RejectsTopNAboveTheHardLimit(t *testing.T) {
	// No repo expectations: a rejected profile must never reach storage.
	svc, _, _ := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	values := aiSummaryTeamProfile()
	values.TopN = models.MaxAISummaryTopN + 1

	_, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, values)

	assert.ErrorIs(t, err, services.ErrInvalidAISummarySettings)
	assert.Contains(t, err.Error(), "top_n must be between 1 and 10")
}

// The limit is inclusive, matching the storage CHECK, and no instance value
// narrows it: the instance default top_n of 3 is a default, not a ceiling.
func TestTeamAISummarySettingsService_Update_AcceptsTopNAtTheHardLimit(t *testing.T) {
	instance := aiSummaryInstanceValues()
	instance.TopN = 3
	svc, repo, providers := newAISummarySettingsServiceWithInstance(
		t, allowAllAuthz{}, nil, &fakeAISummaryInstance{values: instance})
	expectProviderOwnedByTeam(providers)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)
	expectProviderCount(providers, 1)

	values := aiSummaryTeamProfile()
	values.TopN = models.MaxAISummaryTopN

	view, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, values)

	require.NoError(t, err)
	assert.Equal(t, models.MaxAISummaryTopN, view.Values.TopN)
}

// max_output_tokens is bounded by the hard limit alone too (#1199), and the
// error names it.
func TestTeamAISummarySettingsService_Update_RejectsMaxOutputTokensAboveTheHardLimit(t *testing.T) {
	// No repo expectations: a rejected profile must never reach storage.
	svc, _, _ := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	values := aiSummaryTeamProfile()
	values.MaxOutputTokens = models.MaxAISummaryOutputTokens + 1

	_, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, values)

	assert.ErrorIs(t, err, services.ErrInvalidAISummarySettings)
	assert.Contains(t, err.Error(), "max_output_tokens must be between 1 and 32768")
}

// The limit is inclusive, and far above the instance default of 800.
func TestTeamAISummarySettingsService_Update_AcceptsMaxOutputTokensAtTheHardLimit(t *testing.T) {
	svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	expectProviderOwnedByTeam(providers)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)
	expectProviderCount(providers, 1)

	values := aiSummaryTeamProfile()
	values.MaxOutputTokens = models.MaxAISummaryOutputTokens

	view, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, values)

	require.NoError(t, err)
	assert.Equal(t, models.MaxAISummaryOutputTokens, view.Values.MaxOutputTokens)
	assert.Equal(t, models.MaxAISummaryOutputTokens, view.MaxOutputTokensCeiling)
}

// The instance defaults are resolved per call: an instance admin's change
// shows up as the effective values of a team with no profile, and as
// instance_defaults for every team, on the very next call — no restart.
func TestTeamAISummarySettingsService_InstanceChangeAppliesOnTheNextCall(t *testing.T) {
	instance := &fakeAISummaryInstance{values: aiSummaryInstanceValues()}
	svc, repo, providers := newAISummarySettingsServiceWithInstance(t, allowAllAuthz{}, nil, instance)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil)
	expectProviderCount(providers, 1)

	first, err := svc.Resolve(context.Background(), testTeamID)
	require.NoError(t, err)
	assert.Equal(t, 5, first.Values.TopN)

	instance.values.Enabled = false
	instance.values.TopN = 7
	instance.values.Style = models.AISummaryStyleConcise
	instance.values.MaxOutputTokens = 1500

	second, err := svc.Resolve(context.Background(), testTeamID)
	require.NoError(t, err)
	want := models.TeamAISummarySettingsValues{
		Enabled: false, TopN: 7, Style: models.AISummaryStyleConcise, MaxOutputTokens: 1500,
	}
	assert.Equal(t, want, second.Values)
	assert.Equal(t, want, second.InstanceDefaults)

	got, err := svc.Get(context.Background(), testTeamID)
	require.NoError(t, err)
	assert.Equal(t, want, got.Values)
	assert.False(t, svc.Availability(context.Background(), testTeamID).Enabled,
		"with no profile, the instance enabled default decides availability")
}

// A stored team profile still wins over the instance defaults — including
// enabled=true when the instance default is false: the instance value is a
// default, not a kill switch.
func TestTeamAISummarySettingsService_TeamProfileWinsOverInstanceDefaults(t *testing.T) {
	instance := aiSummaryInstanceValues()
	instance.Enabled = false
	svc, repo, providers := newAISummarySettingsServiceWithInstance(
		t, allowAllAuthz{}, nil, &fakeAISummaryInstance{values: instance})
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(&models.TeamAISummarySettings{
		TeamID: testTeamID, Enabled: true, TopN: 9, Style: models.AISummaryStyleDetailed, MaxOutputTokens: 5000,
	}, nil)
	expectProviderCount(providers, 1)

	view, err := svc.Resolve(context.Background(), testTeamID)

	require.NoError(t, err)
	assert.Equal(t, models.TeamAISummarySettingsSourceTeam, view.Source)
	assert.True(t, view.Values.Enabled)
	assert.Equal(t, 9, view.Values.TopN)
	assert.False(t, view.InstanceDefaults.Enabled)
	assert.Equal(t, models.AISummaryAvailability{Available: true, Enabled: true},
		svc.Availability(context.Background(), testTeamID))
}

// invalidAISummaryValues covers one violation per validation bound; each mirrors
// a CHECK constraint on team_ai_summary_settings.
func invalidAISummaryValues() map[string]models.TeamAISummarySettingsValues {
	zeroTopN := aiSummaryTeamProfile()
	zeroTopN.TopN = 0

	negativeTopN := aiSummaryTeamProfile()
	negativeTopN.TopN = -1

	zeroTokens := aiSummaryTeamProfile()
	zeroTokens.MaxOutputTokens = 0

	unknownStyle := aiSummaryTeamProfile()
	unknownStyle.Style = "verbose"

	emptyStyle := aiSummaryTeamProfile()
	emptyStyle.Style = ""

	return map[string]models.TeamAISummarySettingsValues{
		"zero top_n":             zeroTopN,
		"negative top_n":         negativeTopN,
		"zero max_output_tokens": zeroTokens,
		"unknown style":          unknownStyle,
		"empty style":            emptyStyle,
	}
}

func TestTeamAISummarySettingsService_Update_RejectsInvalidProfiles(t *testing.T) {
	for name, values := range invalidAISummaryValues() {
		t.Run(name, func(t *testing.T) {
			// No repo expectations: a rejected profile must never reach storage.
			svc, _, _ := newAISummarySettingsService(t, allowAllAuthz{}, nil)

			_, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, values)

			assert.ErrorIs(t, err, services.ErrInvalidAISummarySettings)
		})
	}
}

func TestTeamAISummarySettingsService_Update_DeniedWithoutPermission(t *testing.T) {
	svc, _, _ := newAISummarySettingsService(t, denyAuthz{}, nil)

	_, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, aiSummaryTeamProfile())

	assert.ErrorIs(t, err, services.ErrPermissionDenied)
}

// Authorization must be checked BEFORE validation, so an unauthorized caller
// cannot use the error body to probe which values the endpoint accepts.
func TestTeamAISummarySettingsService_Update_AuthorizesBeforeValidating(t *testing.T) {
	svc, _, _ := newAISummarySettingsService(t, denyAuthz{}, nil)
	invalid := aiSummaryTeamProfile()
	invalid.TopN = 999

	_, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, invalid)

	assert.ErrorIs(t, err, services.ErrPermissionDenied)
	assert.NotErrorIs(t, err, services.ErrInvalidAISummarySettings)
}

func TestTeamAISummarySettingsService_Update_RepositoryErrorPropagates(t *testing.T) {
	svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	expectProviderOwnedByTeam(providers)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(errors.New("boom"))

	_, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, aiSummaryTeamProfile())

	assert.Error(t, err, "unlike Resolve, a WRITE must never fail open — the caller must know it did not save")
}

// A provider-count failure AFTER a successful Upsert must still surface as an
// error: the write itself is not fail-open, so the response reporting it
// can't be either — a caller that saved successfully must not be told the
// count silently, incorrectly, defaulted.
func TestTeamAISummarySettingsService_Update_ProviderCountErrorPropagates(t *testing.T) {
	svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	expectProviderOwnedByTeam(providers)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)
	providers.EXPECT().Count(mock.Anything, testTeamID).Return(0, errors.New("boom"))

	_, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, aiSummaryTeamProfile())

	assert.Error(t, err)
}

func TestTeamAISummarySettingsService_Reset(t *testing.T) {
	svc, repo, _ := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	repo.EXPECT().Delete(mock.Anything, testTeamID).Return(nil)

	assert.NoError(t, svc.Reset(context.Background(), testAISummaryUserID, testTeamID))
}

func TestTeamAISummarySettingsService_Reset_DeniedWithoutPermission(t *testing.T) {
	svc, _, _ := newAISummarySettingsService(t, denyAuthz{}, nil)

	err := svc.Reset(context.Background(), testAISummaryUserID, testTeamID)

	assert.ErrorIs(t, err, services.ErrPermissionDenied)
}

func TestTeamAISummarySettingsService_Reset_RepositoryErrorPropagates(t *testing.T) {
	svc, repo, _ := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	repo.EXPECT().Delete(mock.Anything, testTeamID).Return(errors.New("boom"))

	assert.Error(t, svc.Reset(context.Background(), testAISummaryUserID, testTeamID))
}

func TestTeamAISummarySettingsService_Availability(t *testing.T) {
	enabledRow := &models.TeamAISummarySettings{
		TeamID: testTeamID, Enabled: true, TopN: 5, Style: models.AISummaryStyleBalanced, MaxOutputTokens: 800,
	}
	disabledRow := &models.TeamAISummarySettings{
		TeamID: testTeamID, Enabled: false, TopN: 5, Style: models.AISummaryStyleBalanced, MaxOutputTokens: 800,
	}

	for name, tc := range map[string]struct {
		row   *models.TeamAISummarySettings
		count int
		want  models.AISummaryAvailability
	}{
		"provider and enabled":    {row: enabledRow, count: 2, want: models.AISummaryAvailability{Available: true, Enabled: true}},
		"provider but disabled":   {row: disabledRow, count: 1, want: models.AISummaryAvailability{Available: true, Enabled: false}},
		"no provider":             {row: enabledRow, count: 0, want: models.AISummaryAvailability{Available: false, Enabled: true}},
		"no row inherits enabled": {row: nil, count: 1, want: models.AISummaryAvailability{Available: true, Enabled: true}},
	} {
		t.Run(name, func(t *testing.T) {
			svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, nil)
			repo.EXPECT().Get(mock.Anything, testTeamID).Return(tc.row, nil)
			providers.EXPECT().Count(mock.Anything, testTeamID).Return(tc.count, nil)

			assert.Equal(t, tc.want, svc.Availability(context.Background(), testTeamID))
		})
	}
}

// A failed provider lookup must degrade to available=false — never an error —
// because the caller is the search handler and a search must not fail over a
// UI affordance flag.
func TestTeamAISummarySettingsService_Availability_ProviderCountErrorFailsOpen(t *testing.T) {
	var logs bytes.Buffer
	svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, &logs)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil)
	providers.EXPECT().Count(mock.Anything, testTeamID).Return(0, errors.New("connection refused"))

	got := svc.Availability(context.Background(), testTeamID)

	assert.Equal(t, models.AISummaryAvailability{Available: false, Enabled: true}, got)
	assertAISummaryWarnLogged(t, logs.String())
}

// A settings read failure goes through Resolve's own fail-open: the instance
// defaults decide enabled, and availability is still answered.
func TestTeamAISummarySettingsService_Availability_SettingsErrorFailsOpen(t *testing.T) {
	var logs bytes.Buffer
	svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, &logs)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, errors.New("connection refused"))
	providers.EXPECT().Count(mock.Anything, testTeamID).Return(1, nil)

	got := svc.Availability(context.Background(), testTeamID)

	assert.Equal(t, models.AISummaryAvailability{Available: true, Enabled: true}, got)
	assertAISummaryWarnLogged(t, logs.String())
}

// Get and the Update response are the settings API's reads and fail CLOSED on
// the instance defaults too: reporting the built-in defaults as instance_defaults
// (or as a no-profile team's values) during an outage would present a guess as
// fact and invite an admin to save it as an override.
func TestTeamAISummarySettingsService_Get_InstanceReadErrorPropagates(t *testing.T) {
	instance := &fakeAISummaryInstance{values: aiSummaryInstanceValues(), getErr: errors.New("instance row unreadable")}
	svc, repo, providers := newAISummarySettingsServiceWithInstance(t, allowAllAuthz{}, nil, instance)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil)
	expectProviderCount(providers, 1)

	view, err := svc.Get(context.Background(), testTeamID)

	assert.ErrorContains(t, err, "instance row unreadable")
	assert.Nil(t, view)
}

func TestTeamAISummarySettingsService_Update_InstanceReadErrorPropagates(t *testing.T) {
	instance := &fakeAISummaryInstance{values: aiSummaryInstanceValues(), getErr: errors.New("instance row unreadable")}
	svc, repo, providers := newAISummarySettingsServiceWithInstance(t, allowAllAuthz{}, nil, instance)
	expectProviderOwnedByTeam(providers)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)
	expectProviderCount(providers, 1)

	view, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, aiSummaryTeamProfile())

	assert.ErrorContains(t, err, "instance row unreadable")
	assert.Nil(t, view)
}

// Resolve and Availability stay fail-open: the same instance outage that fails
// Get must not fail a summary or a search response.
func TestTeamAISummarySettingsService_Resolve_IgnoresTheFailClosedInstanceRead(t *testing.T) {
	instance := &fakeAISummaryInstance{values: aiSummaryInstanceValues(), getErr: errors.New("instance row unreadable")}
	svc, repo, providers := newAISummarySettingsServiceWithInstance(t, allowAllAuthz{}, nil, instance)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil)
	expectProviderCount(providers, 1)

	view, err := svc.Resolve(context.Background(), testTeamID)

	require.NoError(t, err)
	assert.Equal(t, aiSummaryInstanceValues().TeamValues(), view.Values)
	assert.Equal(t, models.AISummaryAvailability{Available: true, Enabled: true},
		svc.Availability(context.Background(), testTeamID))
}
