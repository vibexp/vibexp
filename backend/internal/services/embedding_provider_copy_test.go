package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

const (
	testEmbedCopySourceTeamID = "team-source"
	testEmbedCopyProviderID   = "embedding-provider-source-1"
	testEmbedCopySourceModel  = "mxbai-embed-large"
)

func newCopyEmbeddingService(t *testing.T, authorizer AuthorizationServiceInterface, audit *recordingAuditService) (
	*EmbeddingProviderService,
	*repomocks.MockEmbeddingProviderRepository,
	*repomocks.MockEmbeddingBackfillRepository,
) {
	t.Helper()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	repo := repomocks.NewMockEmbeddingProviderRepository(t)
	coverage := repomocks.NewMockEmbeddingBackfillRepository(t)
	svc := NewEmbeddingProviderService(repo, enc, localDevProviderConfig(), authorizer, audit, coverage)
	return svc, repo, coverage
}

func copyEmbeddingParams() CopyEmbeddingProviderParams {
	return CopyEmbeddingProviderParams{
		TeamID:           testProviderTeamID,
		SourceTeamID:     testEmbedCopySourceTeamID,
		SourceProviderID: testEmbedCopyProviderID,
		UserID:           testProviderUserID,
	}
}

func sourceEmbeddingProviderRow() *models.EmbeddingProvider {
	sourceTeamID := testEmbedCopySourceTeamID
	baseURL := "https://api.openai.com/v1"
	ciphertext := "ZW5jcnlwdGVkLWFwaS1rZXk="
	queryPrefix := "Represent this sentence for searching relevant passages: "
	documentPrefix := "passage: "

	return &models.EmbeddingProvider{
		ID:              testEmbedCopyProviderID,
		UserID:          "someone-else",
		TeamID:          &sourceTeamID,
		Name:            "mxbai on Ollama",
		ProviderType:    ProviderTypeOpenAICompatible,
		Model:           testEmbedCopySourceModel,
		ChunkSize:       512,
		ChunkOverlap:    64,
		Concurrency:     4,
		QueryPrefix:     &queryPrefix,
		DocumentPrefix:  &documentPrefix,
		IsDefault:       true,
		BaseURL:         &baseURL,
		APIKeyEncrypted: &ciphertext,
		Configuration:   `{"timeout":30}`,
	}
}

// destinationProviderRow builds a row as it exists in the DESTINATION team,
// used as the active-provider fixture GetActiveProvider hands back.
func destinationProviderRow(id, model string, isDefault bool) *models.EmbeddingProvider {
	teamID := testProviderTeamID
	return &models.EmbeddingProvider{
		ID: id, TeamID: &teamID, Name: id, Model: model,
		ProviderType: ProviderTypeOpenAICompatible, IsDefault: isDefault,
	}
}

// testEmbedCopyCreatedID is the id the destination's repository assigns to the
// inserted row — deliberately unlike the source's, so a test asserting on it
// cannot pass against a copy that returned the source row.
const testEmbedCopyCreatedID = "dst-provider-1"

// expectCreateAssigningID stands in for the repository's INSERT, assigning the
// destination's own id the way the real one does.
func expectCreateAssigningID(repo *repomocks.MockEmbeddingProviderRepository) {
	repo.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, p *models.EmbeddingProvider) { p.ID = testEmbedCopyCreatedID }).
		Return(nil)
}

func TestEmbeddingProviderService_CopyFromTeam_CarriesCiphertextTuningAndLandsNonDefault(t *testing.T) {
	audit := &recordingAuditService{}
	svc, repo, _ := newCopyEmbeddingService(t, permissiveProviderAuthz{}, audit)
	source := sourceEmbeddingProviderRow()

	repo.EXPECT().GetByID(mock.Anything, testEmbedCopySourceTeamID, testEmbedCopyProviderID).
		Return(source, nil)
	repo.EXPECT().ListNames(mock.Anything, testProviderTeamID).Return([]string{"something else"}, nil)
	// The destination already has a default of its own, so the copy changes
	// nothing about which provider is active.
	repo.EXPECT().GetActiveProvider(mock.Anything, testProviderTeamID).
		Return(destinationProviderRow("dst-default", "text-embedding-3-small", true), nil)
	expectCreateAssigningID(repo)

	result, err := svc.CopyFromTeam(context.Background(), copyEmbeddingParams())
	require.NoError(t, err)
	copied := result.Provider

	// The credential moved as ciphertext — byte-identical, never decrypted.
	require.NotNil(t, copied.APIKeyEncrypted)
	assert.Equal(t, *source.APIKeyEncrypted, *copied.APIKeyEncrypted)

	// Non-default regardless of the source, or the INSERT would race the
	// partial unique index idx_embedding_providers_team_default.
	assert.False(t, copied.IsDefault, "a copy must never land as the destination's default")
	assert.Equal(t, testEmbedCopyCreatedID, copied.ID)
	assert.Equal(t, testProviderTeamID, *copied.TeamID)
	assert.Equal(t, testProviderUserID, copied.UserID)

	// The tuning that makes the copy embed like its source rather than like a
	// freshly created provider on create-path defaults (1000/200/1, no prefixes).
	assert.Equal(t, source.ChunkSize, copied.ChunkSize)
	assert.Equal(t, source.ChunkOverlap, copied.ChunkOverlap)
	assert.Equal(t, source.Concurrency, copied.Concurrency)
	require.NotNil(t, copied.QueryPrefix)
	assert.Equal(t, *source.QueryPrefix, *copied.QueryPrefix)
	require.NotNil(t, copied.DocumentPrefix)
	assert.Equal(t, *source.DocumentPrefix, *copied.DocumentPrefix)
	assert.Equal(t, source.Configuration, copied.Configuration)

	require.Len(t, audit.records, 1)
	record := audit.records[0]
	assert.Equal(t, models.SettingsAuditSurfaceEmbeddingProvider, record.Surface)
	assert.Equal(t, testProviderTeamID, record.TeamID)
	assert.Equal(t, testEmbedCopySourceTeamID, record.SourceTeamID)
	assert.Equal(t, testEmbedCopyProviderID, record.SourceResourceID)
	assert.Equal(t, testEmbedCopyCreatedID, record.CreatedResourceID)
	assert.Equal(t, true, record.Detail["has_api_key"])
	assert.Equal(t, false, record.Detail["becomes_active"])
}

// TestEmbeddingProviderService_CopyFromTeam_NonDefaultCopyStillBecomesActive is
// the test the issue asks for by name, and the reason the whole activation
// verdict exists.
//
// The destination team HAS providers but NO default. GetActiveProvider resolves
// `is_default DESC, updated_at DESC`, so the freshly inserted copy — written
// non-default, like every copy — wins on recency and silently becomes the
// provider every future embedding is generated with. Nothing errors; search just
// gets worse for the 412 resources still embedded under the old model.
//
// Gating this warning on the is_default flag, which is the obvious thing to do,
// would report `becomes_active: false` here — precisely inverted.
func TestEmbeddingProviderService_CopyFromTeam_NonDefaultCopyStillBecomesActive(t *testing.T) {
	audit := &recordingAuditService{}
	svc, repo, coverage := newCopyEmbeddingService(t, permissiveProviderAuthz{}, audit)

	repo.EXPECT().GetByID(mock.Anything, testEmbedCopySourceTeamID, testEmbedCopyProviderID).
		Return(sourceEmbeddingProviderRow(), nil)
	repo.EXPECT().ListNames(mock.Anything, testProviderTeamID).Return(nil, nil)

	// Before: a provider exists, but NOTHING is flagged default — so the winner
	// is decided by updated_at alone. After: the copy, on recency.
	displaced := destinationProviderRow("dst-existing", "text-embedding-3-small", false)
	repo.EXPECT().GetActiveProvider(mock.Anything, testProviderTeamID).
		Return(displaced, nil).Once()
	expectCreateAssigningID(repo)
	repo.EXPECT().GetActiveProvider(mock.Anything, testProviderTeamID).
		Return(destinationProviderRow(testEmbedCopyCreatedID, testEmbedCopySourceModel, false), nil).Once()

	coverage.EXPECT().CountCoverage(mock.Anything, "text-embedding-3-small", testProviderTeamID).
		Return([]models.EmbeddingCoverageCount{
			{EntityType: "prompt", Total: 300, Embedded: 300},
			{EntityType: "artifact", Total: 120, Embedded: 112},
			{EntityType: "memory", Total: 40, Embedded: 0},
		}, nil)

	result, err := svc.CopyFromTeam(context.Background(), copyEmbeddingParams())
	require.NoError(t, err)

	assert.False(t, result.Provider.IsDefault, "the copy is non-default…")
	assert.True(t, result.Activation.BecomesActive, "…and is nevertheless the active provider")
	require.NotNil(t, result.Activation.DisplacedModel)
	assert.Equal(t, "text-embedding-3-small", *result.Activation.DisplacedModel)
	assert.Equal(t, int64(412), result.Activation.DisplacedEmbeddedResources)
	assert.True(t, result.Activation.ModelChanged)

	// The audit row carries the verdict too: "this copy took over the team's
	// search" is exactly what an audit trail is read for after the fact.
	require.Len(t, audit.records, 1)
	assert.Equal(t, true, audit.records[0].Detail["becomes_active"])
	assert.Equal(t, "text-embedding-3-small", audit.records[0].Detail["displaced_model"])
	assert.Equal(t, int64(412), audit.records[0].Detail["displaced_embedded_resources"])
}

// TestEmbeddingProviderService_CopyFromTeam_ActivationEdges covers the states
// around the trap: an empty destination (nothing displaced), a destination whose
// own default keeps winning, and a copy that takes over with the SAME model —
// which changes credentials, not vector space, so the stored vectors stay valid.
func TestEmbeddingProviderService_CopyFromTeam_ActivationEdges(t *testing.T) {
	tests := []struct {
		name             string
		before, after    *models.EmbeddingProvider
		beforeErr        error
		wantActive       bool
		wantDisplaced    *string
		wantModelChanged bool
		wantCoverage     bool
	}{
		{
			name:       "empty destination team displaces nothing",
			beforeErr:  repositories.ErrNoActiveEmbeddingProvider,
			after:      destinationProviderRow(testEmbedCopyCreatedID, testEmbedCopySourceModel, false),
			wantActive: true,
		},
		{
			name:       "the destination's own default keeps winning",
			before:     destinationProviderRow("dst-default", "text-embedding-3-small", true),
			after:      destinationProviderRow("dst-default", "text-embedding-3-small", true),
			wantActive: false,
		},
		{
			name:             "same model: active, but the vectors stay valid",
			before:           destinationProviderRow("dst-existing", testEmbedCopySourceModel, false),
			after:            destinationProviderRow(testEmbedCopyCreatedID, testEmbedCopySourceModel, false),
			wantActive:       true,
			wantDisplaced:    ptr(testEmbedCopySourceModel),
			wantModelChanged: false,
			wantCoverage:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, coverage := newCopyEmbeddingService(t, permissiveProviderAuthz{}, &recordingAuditService{})

			repo.EXPECT().GetByID(mock.Anything, testEmbedCopySourceTeamID, testEmbedCopyProviderID).
				Return(sourceEmbeddingProviderRow(), nil)
			repo.EXPECT().ListNames(mock.Anything, testProviderTeamID).Return(nil, nil)
			repo.EXPECT().GetActiveProvider(mock.Anything, testProviderTeamID).
				Return(tc.before, tc.beforeErr).Once()
			expectCreateAssigningID(repo)
			repo.EXPECT().GetActiveProvider(mock.Anything, testProviderTeamID).
				Return(tc.after, nil).Once()

			if tc.wantCoverage {
				coverage.EXPECT().CountCoverage(mock.Anything, mock.Anything, testProviderTeamID).
					Return([]models.EmbeddingCoverageCount{{EntityType: "prompt", Total: 5, Embedded: 5}}, nil)
			}

			result, err := svc.CopyFromTeam(context.Background(), copyEmbeddingParams())
			require.NoError(t, err)

			assert.Equal(t, tc.wantActive, result.Activation.BecomesActive)
			assert.Equal(t, tc.wantModelChanged, result.Activation.ModelChanged)
			if tc.wantDisplaced == nil {
				assert.Nil(t, result.Activation.DisplacedModel)
				assert.Zero(t, result.Activation.DisplacedEmbeddedResources)
			} else {
				require.NotNil(t, result.Activation.DisplacedModel)
				assert.Equal(t, *tc.wantDisplaced, *result.Activation.DisplacedModel)
			}
		})
	}
}

func TestEmbeddingProviderService_CopyFromTeam_AuthorizesBothTeamsDestinationFirst(t *testing.T) {
	tests := []struct {
		name   string
		denied map[string]bool
	}{
		{"destination denied", map[string]bool{testProviderTeamID: true}},
		{"source denied", map[string]bool{testEmbedCopySourceTeamID: true}},
		{"both denied", map[string]bool{testProviderTeamID: true, testEmbedCopySourceTeamID: true}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var checked []string
			// The repository mock is given no expectations at all: a denied copy
			// must not read, write or audit anything, and mockery fails if it does.
			svc, _, _ := newCopyEmbeddingService(t,
				denyProviderTeamsAuthz{denied: tc.denied, checked: &checked},
				&recordingAuditService{})

			_, err := svc.CopyFromTeam(context.Background(), copyEmbeddingParams())

			require.ErrorIs(t, err, ErrPermissionDenied)
			assert.NotContains(t, err.Error(), testEmbedCopySourceTeamID,
				"the denial must never name a team: it would confirm the source exists")
			require.NotEmpty(t, checked)
			assert.Equal(t, testProviderTeamID, checked[0], "the destination is evaluated first")
		})
	}
}

func TestEmbeddingProviderService_CopyFromTeam_RejectsAMeaninglessSource(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		wantErr error
	}{
		{"no source", "", ErrCopySourceRequired},
		{"source is the destination", testProviderTeamID, ErrCopySourceIsDestination},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := newCopyEmbeddingService(t, permissiveProviderAuthz{}, &recordingAuditService{})

			params := copyEmbeddingParams()
			params.SourceTeamID = tc.source

			_, err := svc.CopyFromTeam(context.Background(), params)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestEmbeddingProviderService_CopyFromTeam_DisambiguatesAnInheritedName(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		want     string
	}{
		{"free", []string{"other"}, "mxbai on Ollama"},
		{"taken once", []string{"mxbai on Ollama"}, "mxbai on Ollama (copy)"},
		{"taken twice", []string{"mxbai on Ollama", "mxbai on Ollama (copy)"}, "mxbai on Ollama (copy 2)"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := newCopyEmbeddingService(t, permissiveProviderAuthz{}, &recordingAuditService{})

			repo.EXPECT().GetByID(mock.Anything, testEmbedCopySourceTeamID, testEmbedCopyProviderID).
				Return(sourceEmbeddingProviderRow(), nil)
			repo.EXPECT().ListNames(mock.Anything, testProviderTeamID).Return(tc.existing, nil)
			repo.EXPECT().GetActiveProvider(mock.Anything, testProviderTeamID).
				Return(nil, repositories.ErrNoActiveEmbeddingProvider)
			expectCreateAssigningID(repo)

			result, err := svc.CopyFromTeam(context.Background(), copyEmbeddingParams())
			require.NoError(t, err)
			assert.Equal(t, tc.want, result.Provider.Name)
		})
	}
}

// TestEmbeddingProviderService_CopyFromTeam_ACallerSuppliedNameIsUsedVerbatim:
// the caller has seen the destination's providers and chosen, so a collision
// earns a 409 rather than a silent rename — and ListNames is never even read.
func TestEmbeddingProviderService_CopyFromTeam_ACallerSuppliedNameIsUsedVerbatim(t *testing.T) {
	svc, repo, _ := newCopyEmbeddingService(t, permissiveProviderAuthz{}, &recordingAuditService{})

	repo.EXPECT().GetByID(mock.Anything, testEmbedCopySourceTeamID, testEmbedCopyProviderID).
		Return(sourceEmbeddingProviderRow(), nil)
	repo.EXPECT().GetActiveProvider(mock.Anything, testProviderTeamID).
		Return(nil, repositories.ErrNoActiveEmbeddingProvider)
	expectCreateAssigningID(repo)

	params := copyEmbeddingParams()
	chosen := "mxbai (staging)"
	params.Name = &chosen

	result, err := svc.CopyFromTeam(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, chosen, result.Provider.Name)
}

// TestEmbeddingProviderService_CopyFromTeam_SuffixKeepsTheNameWithinTheColumn
// mirrors #830: embedding_providers.name is varchar(255) counted in CHARACTERS,
// so trimming by byte length would over-trim a multi-byte name and could cut
// mid-rune into invalid UTF-8 that Postgres rejects outright.
func TestEmbeddingProviderService_CopyFromTeam_SuffixKeepsTheNameWithinTheColumn(t *testing.T) {
	for _, tc := range []struct{ name, longName string }{
		{"ascii", strings.Repeat("a", providerNameMaxLen)},
		{"multi-byte", strings.Repeat("日", providerNameMaxLen)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := newCopyEmbeddingService(t, permissiveProviderAuthz{}, &recordingAuditService{})

			source := sourceEmbeddingProviderRow()
			source.Name = tc.longName

			repo.EXPECT().GetByID(mock.Anything, testEmbedCopySourceTeamID, testEmbedCopyProviderID).
				Return(source, nil)
			repo.EXPECT().ListNames(mock.Anything, testProviderTeamID).Return([]string{tc.longName}, nil)
			repo.EXPECT().GetActiveProvider(mock.Anything, testProviderTeamID).
				Return(nil, repositories.ErrNoActiveEmbeddingProvider)
			expectCreateAssigningID(repo)

			result, err := svc.CopyFromTeam(context.Background(), copyEmbeddingParams())
			require.NoError(t, err)

			name := result.Provider.Name
			assert.LessOrEqual(t, utf8.RuneCountInString(name), providerNameMaxLen)
			assert.True(t, utf8.ValidString(name), "trimming must not split a rune: %q", name)
			assert.True(t, strings.HasSuffix(name, " (copy)"), "got %q", name)
		})
	}
}

func TestEmbeddingProviderService_CopyFromTeam_MissingSourceProviderIsNotFound(t *testing.T) {
	svc, repo, _ := newCopyEmbeddingService(t, permissiveProviderAuthz{}, &recordingAuditService{})

	repo.EXPECT().GetByID(mock.Anything, testEmbedCopySourceTeamID, testEmbedCopyProviderID).
		Return(nil, repositories.ErrEmbeddingProviderNotFound)

	_, err := svc.CopyFromTeam(context.Background(), copyEmbeddingParams())
	require.ErrorIs(t, err, ErrProviderNotFound)
}

// TestEmbeddingProviderService_CopyFromTeam_AStorageErrorIsNotAMissingProvider:
// unlike GetEmbeddingProvider, which collapses every read failure to not-found,
// the copy keeps a storage error a storage error — a 404 for an unreachable
// database would send the caller hunting for a provider that exists.
func TestEmbeddingProviderService_CopyFromTeam_AStorageErrorIsNotAMissingProvider(t *testing.T) {
	svc, repo, _ := newCopyEmbeddingService(t, permissiveProviderAuthz{}, &recordingAuditService{})

	repo.EXPECT().GetByID(mock.Anything, testEmbedCopySourceTeamID, testEmbedCopyProviderID).
		Return(nil, errors.New("connection refused"))

	_, err := svc.CopyFromTeam(context.Background(), copyEmbeddingParams())
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrProviderNotFound)
}

// TestEmbeddingProviderService_CopyFromTeam_AFailedAuditFailsTheCopy: the row is
// the compensating control for moving a credential between teams, so reporting
// success for a copy whose audit did not land would defeat having it.
func TestEmbeddingProviderService_CopyFromTeam_AFailedAuditFailsTheCopy(t *testing.T) {
	audit := &recordingAuditService{err: errors.New("audit table unavailable")}
	svc, repo, _ := newCopyEmbeddingService(t, permissiveProviderAuthz{}, audit)

	repo.EXPECT().GetByID(mock.Anything, testEmbedCopySourceTeamID, testEmbedCopyProviderID).
		Return(sourceEmbeddingProviderRow(), nil)
	repo.EXPECT().ListNames(mock.Anything, testProviderTeamID).Return(nil, nil)
	repo.EXPECT().GetActiveProvider(mock.Anything, testProviderTeamID).
		Return(nil, repositories.ErrNoActiveEmbeddingProvider)
	expectCreateAssigningID(repo)

	_, err := svc.CopyFromTeam(context.Background(), copyEmbeddingParams())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audit")
}

// TestEmbeddingProviderService_CopyFromTeam_AppliesOverrides pins that a sent
// override replaces the source's value, and that an explicit empty string clears
// a nullable field rather than storing "".
func TestEmbeddingProviderService_CopyFromTeam_AppliesOverrides(t *testing.T) {
	svc, repo, _ := newCopyEmbeddingService(t, permissiveProviderAuthz{}, &recordingAuditService{})

	repo.EXPECT().GetByID(mock.Anything, testEmbedCopySourceTeamID, testEmbedCopyProviderID).
		Return(sourceEmbeddingProviderRow(), nil)
	repo.EXPECT().GetActiveProvider(mock.Anything, testProviderTeamID).
		Return(nil, repositories.ErrNoActiveEmbeddingProvider)
	expectCreateAssigningID(repo)

	params := copyEmbeddingParams()
	params.Name = ptr("renamed")
	params.Model = ptr("text-embedding-3-large")
	params.ChunkSize = ptr(2048)
	params.ChunkOverlap = ptr(0)
	params.Concurrency = ptr(1)
	params.QueryPrefix = ptr("")    // explicit clear
	params.DocumentPrefix = ptr("") // explicit clear
	params.BaseURL = ptr("")        // explicit clear
	params.Configuration = &map[string]interface{}{"timeout": 5}

	result, err := svc.CopyFromTeam(context.Background(), params)
	require.NoError(t, err)
	copied := result.Provider

	assert.Equal(t, "renamed", copied.Name)
	assert.Equal(t, "text-embedding-3-large", copied.Model)
	assert.Equal(t, 2048, copied.ChunkSize)
	assert.Equal(t, 0, copied.ChunkOverlap)
	assert.Equal(t, 1, copied.Concurrency)
	assert.Nil(t, copied.QueryPrefix, `an explicit "" clears the prefix rather than storing it`)
	assert.Nil(t, copied.DocumentPrefix)
	assert.Nil(t, copied.BaseURL)
	assert.JSONEq(t, `{"timeout":5}`, copied.Configuration)
}

func ptr[T any](v T) *T { return &v }
