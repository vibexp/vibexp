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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
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

func aiSummaryInstanceConfig() config.AISummaryConfig {
	return config.AISummaryConfig{
		Enabled:           true,
		TopN:              5,
		MaxTopN:           8,
		PerDocumentChars:  8000,
		TotalContextChars: 32000,
		MaxOutputTokens:   800,
		RequestTimeout:    60 * time.Second,
		Style:             models.AISummaryStyleBalanced,
	}
}

// aiSummaryTeamProfile is deliberately different from the instance config in
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
	repo := repomocks.NewMockTeamAISummarySettingsRepository(t)
	providers := repomocks.NewMockModelProviderRepository(t)
	handler := slog.Handler(slog.DiscardHandler)
	if logs != nil {
		handler = slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	return services.NewTeamAISummarySettingsService(
		repo, providers, authzSvc, aiSummaryInstanceConfig(), slog.New(handler)), repo, providers
}

// expectProviderOwnedByTeam stubs the tenancy lookup Update runs on a non-nil
// model_provider_id.
func expectProviderOwnedByTeam(providers *repomocks.MockModelProviderRepository, teamID, providerID string) {
	providers.EXPECT().GetByID(mock.Anything, teamID, providerID).
		Return(&models.ModelProvider{ID: providerID, TeamID: &teamID}, nil)
}

func TestTeamAISummarySettingsService_Resolve_NoRowReportsInstanceSource(t *testing.T) {
	svc, repo, _ := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil)

	view, err := svc.Resolve(context.Background(), testTeamID)

	require.NoError(t, err)
	assert.Equal(t, models.TeamAISummarySettingsSourceInstance, view.Source)
	assert.Equal(t, view.InstanceDefaults, view.Values,
		"with no override the effective values ARE the instance defaults")
	assert.True(t, view.Values.Enabled)
	assert.Equal(t, 5, view.Values.TopN)
	assert.Equal(t, models.AISummaryStyleBalanced, view.Values.Style)
	assert.Nil(t, view.Values.ModelProviderID, "the instance has no opinion on which provider to use")
	assert.Equal(t, 8, view.MaxTopN)
}

func TestTeamAISummarySettingsService_Resolve_StoredRowReportsTeamSource(t *testing.T) {
	svc, repo, _ := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	providerID := "provider-9"
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(&models.TeamAISummarySettings{
		TeamID:          testTeamID,
		Enabled:         false,
		ModelProviderID: &providerID,
		TopN:            8,
		Style:           models.AISummaryStyleDetailed,
		MaxOutputTokens: 1200,
	}, nil)

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
	assert.Equal(t, 8, view.MaxTopN)
}

// The resolver FAILS OPEN: a settings read failure degrades the tuning rather
// than failing the request that asked for a summary.
func TestTeamAISummarySettingsService_Resolve_RepositoryErrorFailsOpen(t *testing.T) {
	var logs bytes.Buffer
	svc, repo, _ := newAISummarySettingsService(t, allowAllAuthz{}, &logs)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, errors.New("connection refused"))

	view, err := svc.Resolve(context.Background(), testTeamID)

	require.NoError(t, err, "a settings read failure must not surface as an error")
	assert.Equal(t, models.TeamAISummarySettingsSourceInstance, view.Source)
	assert.Equal(t, view.InstanceDefaults, view.Values)
	assertAISummaryWarnLogged(t, logs.String(), testTeamID)
}

// assertAISummaryWarnLogged pins the observability contract: the fail-open path
// must be greppable, so it logs at warn and carries team_id. Logging it at debug
// would hide a real outage behind silently-default tuning.
func assertAISummaryWarnLogged(t *testing.T, output, teamID string) {
	t.Helper()
	require.NotEmpty(t, output, "fail-open must emit a log line")

	var found bool
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["level"] == "WARN" && entry["team_id"] == teamID {
			found = true
		}
	}
	assert.True(t, found, "expected a WARN log carrying team_id=%s, got: %s", teamID, output)
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
	svc, repo, _ := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil)

	view, err := svc.Get(context.Background(), testTeamID)

	require.NoError(t, err)
	assert.Equal(t, models.TeamAISummarySettingsSourceInstance, view.Source)
	assert.Equal(t, view.InstanceDefaults, view.Values)
	assert.Equal(t, 8, view.MaxTopN)
}

func TestTeamAISummarySettingsService_Get_StoredRowReportsTeamSource(t *testing.T) {
	svc, repo, _ := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(&models.TeamAISummarySettings{
		TeamID:          testTeamID,
		TopN:            8,
		Style:           models.AISummaryStyleConcise,
		MaxOutputTokens: 300,
	}, nil)

	view, err := svc.Get(context.Background(), testTeamID)

	require.NoError(t, err)
	assert.Equal(t, models.TeamAISummarySettingsSourceTeam, view.Source)
	assert.Equal(t, models.AISummaryStyleConcise, view.Values.Style)
	assert.Equal(t, models.AISummaryStyleBalanced, view.InstanceDefaults.Style,
		"instance_defaults must keep reporting the deployment values, not the team's")
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
	expectProviderOwnedByTeam(providers, testTeamID, aiSummaryTestProviderID)
	repo.EXPECT().Upsert(mock.Anything, mock.MatchedBy(func(s *models.TeamAISummarySettings) bool {
		return s.TeamID == testTeamID && s.TopN == 8 && s.Style == models.AISummaryStyleDetailed
	})).Return(nil)

	view, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, aiSummaryTeamProfile())

	require.NoError(t, err)
	assert.Equal(t, models.TeamAISummarySettingsSourceTeam, view.Source)
	assert.Equal(t, 8, view.Values.TopN)
	assert.Equal(t, 8, view.MaxTopN, "the cap still comes from the instance config")
}

// max_top_n is instance-owned: a team may tune top_n only INSIDE it.
func TestTeamAISummarySettingsService_Update_RejectsTopNAboveInstanceCap(t *testing.T) {
	// No repo expectations: a rejected profile must never reach storage.
	svc, _, _ := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	values := aiSummaryTeamProfile()
	values.TopN = aiSummaryInstanceConfig().MaxTopN + 1

	_, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, values)

	assert.ErrorIs(t, err, services.ErrInvalidAISummarySettings)
	assert.Contains(t, err.Error(), "top_n")
}

// The cap itself is inclusive, matching the storage CHECK.
func TestTeamAISummarySettingsService_Update_AcceptsTopNAtTheCap(t *testing.T) {
	svc, repo, providers := newAISummarySettingsService(t, allowAllAuthz{}, nil)
	expectProviderOwnedByTeam(providers, testTeamID, aiSummaryTestProviderID)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)

	values := aiSummaryTeamProfile()
	values.TopN = aiSummaryInstanceConfig().MaxTopN

	_, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, values)

	assert.NoError(t, err)
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
	expectProviderOwnedByTeam(providers, testTeamID, aiSummaryTestProviderID)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(errors.New("boom"))

	_, err := svc.Update(context.Background(), testAISummaryUserID, testTeamID, aiSummaryTeamProfile())

	assert.Error(t, err, "unlike Resolve, a WRITE must never fail open — the caller must know it did not save")
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
