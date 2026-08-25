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

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

const (
	testCopyProviderSourceTeamID = "team-source"
	testCopyProviderID           = "provider-source-1"
)

// denyProviderTeamsAuthz denies authz.TeamUpdate on the named teams and allows
// every other, so a test can refuse exactly one side of a copy.
//
// Unlike the custom-types copy, a provider copy is NOT membership-only: it
// requires the destination's own create permission on both teams, so IsMember
// must never be consulted here.
type denyProviderTeamsAuthz struct {
	denied map[string]bool
	// checked records every team asked about, in order, so a test can assert
	// the destination is evaluated first.
	checked *[]string
}

func (d denyProviderTeamsAuthz) Can(
	_ context.Context, _, teamID string, perm authz.Permission,
) error {
	if perm != authz.TeamUpdate {
		panic("denyProviderTeamsAuthz: a provider copy must require authz.TeamUpdate, got " + string(perm))
	}
	if d.checked != nil {
		*d.checked = append(*d.checked, teamID)
	}
	if d.denied[teamID] {
		return ErrPermissionDenied
	}
	return nil
}

func (d denyProviderTeamsAuthz) IsMember(_ context.Context, _, _ string) error {
	panic("denyProviderTeamsAuthz: a provider copy must not fall back to a membership-only check")
}

func (d denyProviderTeamsAuthz) CanActOnResource(
	_ context.Context, _, _, _ string, _, _ authz.Permission,
) error {
	panic("denyProviderTeamsAuthz: unexpected CanActOnResource call")
}

func (d denyProviderTeamsAuthz) Authorize(
	_ context.Context, _, _ string, _ authz.Permission,
) (models.TeamMemberRole, error) {
	panic("denyProviderTeamsAuthz: unexpected Authorize call")
}

func newCopyProviderService(
	t *testing.T, authorizer AuthorizationServiceInterface, audit *recordingAuditService,
) (*ModelProviderService, *repomocks.MockModelProviderRepository) {
	t.Helper()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	repo := repomocks.NewMockModelProviderRepository(t)
	return NewModelProviderService(repo, enc, localDevProviderConfig(), authorizer, audit), repo
}

func copyProviderParams() CopyModelProviderParams {
	return CopyModelProviderParams{
		TeamID:           testProviderTeamID,
		SourceTeamID:     testCopyProviderSourceTeamID,
		SourceProviderID: testCopyProviderID,
		UserID:           testProviderUserID,
	}
}

func sourceProviderRow() *models.ModelProvider {
	sourceTeamID := testCopyProviderSourceTeamID
	baseURL := "https://api.openai.com/v1"
	ciphertext := "ZW5jcnlwdGVkLWFwaS1rZXk="
	return &models.ModelProvider{
		ID:              testCopyProviderID,
		UserID:          "someone-else",
		TeamID:          &sourceTeamID,
		Name:            "OpenAI GPT-4o",
		ProviderType:    ProviderTypeOpenAICompatible,
		Model:           "gpt-4o-mini",
		IsDefault:       true,
		BaseURL:         &baseURL,
		APIKeyEncrypted: &ciphertext,
		Configuration:   `{"temperature":0.7}`,
	}
}

func TestModelProviderService_CopyFromTeam_CarriesCiphertextAndLandsNonDefault(t *testing.T) {
	audit := &recordingAuditService{}
	svc, repo := newCopyProviderService(t, permissiveProviderAuthz{}, audit)
	source := sourceProviderRow()

	repo.EXPECT().GetByID(mock.Anything, testCopyProviderSourceTeamID, testCopyProviderID).
		Return(source, nil)
	repo.EXPECT().ListNames(mock.Anything, testProviderTeamID).Return([]string{"Anthropic"}, nil)
	repo.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, p *models.ModelProvider) {
			p.ID = "dst-provider-1" // the destination's own id, as the repository assigns it
		}).Return(nil)

	copied, err := svc.CopyFromTeam(context.Background(), copyProviderParams())
	require.NoError(t, err)

	// The credential moved as ciphertext — byte-identical, never decrypted.
	require.NotNil(t, copied.APIKeyEncrypted)
	assert.Equal(t, *source.APIKeyEncrypted, *copied.APIKeyEncrypted)

	// Non-default regardless of the source, or the INSERT would race the
	// partial unique index idx_model_providers_team_default.
	assert.False(t, copied.IsDefault, "a copy must never land as the destination's default")

	assert.Equal(t, "dst-provider-1", copied.ID)
	assert.Equal(t, "OpenAI GPT-4o", copied.Name, "no collision, so the source name is kept")
	require.NotNil(t, copied.TeamID)
	assert.Equal(t, testProviderTeamID, *copied.TeamID)
	assert.Equal(t, testProviderUserID, copied.UserID, "the copier owns the copy, not the source's author")
	assert.Equal(t, source.ProviderType, copied.ProviderType)
	assert.Equal(t, source.Model, copied.Model)
	require.NotNil(t, copied.BaseURL)
	assert.Equal(t, *source.BaseURL, *copied.BaseURL)
	assert.Equal(t, source.Configuration, copied.Configuration)
}

func TestModelProviderService_CopyFromTeam_WritesOneAuditRowWithBothIDs(t *testing.T) {
	audit := &recordingAuditService{}
	svc, repo := newCopyProviderService(t, permissiveProviderAuthz{}, audit)

	repo.EXPECT().GetByID(mock.Anything, testCopyProviderSourceTeamID, testCopyProviderID).
		Return(sourceProviderRow(), nil)
	repo.EXPECT().ListNames(mock.Anything, testProviderTeamID).Return(nil, nil)
	repo.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, p *models.ModelProvider) { p.ID = "dst-provider-1" }).
		Return(nil)

	_, err := svc.CopyFromTeam(context.Background(), copyProviderParams())
	require.NoError(t, err)

	require.Len(t, audit.records, 1, "one copy action writes exactly one audit row")
	entry := audit.records[0]
	assert.Equal(t, testProviderTeamID, entry.TeamID)
	assert.Equal(t, testCopyProviderSourceTeamID, entry.SourceTeamID)
	assert.Equal(t, models.SettingsAuditSurfaceModelProvider, entry.Surface)
	assert.Equal(t, testProviderUserID, entry.ActorUserID)
	// One-to-one, unlike the custom-types copy: both ids are known.
	assert.Equal(t, testCopyProviderID, entry.SourceResourceID)
	assert.Equal(t, "dst-provider-1", entry.CreatedResourceID)
	assert.Equal(t, true, entry.Detail["has_api_key"])

	// Whatever else the detail carries, it must not carry the credential.
	for key, value := range entry.Detail {
		if text, ok := value.(string); ok {
			assert.NotContains(t, text, "ZW5jcnlwdGVk",
				"audit detail %q must never carry the API key ciphertext", key)
		}
	}
}

func TestModelProviderService_CopyFromTeam_AuditFailureFailsTheCopy(t *testing.T) {
	audit := &recordingAuditService{err: errors.New("audit storage down")}
	svc, repo := newCopyProviderService(t, permissiveProviderAuthz{}, audit)

	repo.EXPECT().GetByID(mock.Anything, testCopyProviderSourceTeamID, testCopyProviderID).
		Return(sourceProviderRow(), nil)
	repo.EXPECT().ListNames(mock.Anything, testProviderTeamID).Return(nil, nil)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

	_, err := svc.CopyFromTeam(context.Background(), copyProviderParams())

	// A copy whose audit did not land must not report success: the row is the
	// compensating control for moving a credential between teams.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to record model provider copy")
}

func TestModelProviderService_CopyFromTeam_ProviderWithoutKeyCopiesFine(t *testing.T) {
	audit := &recordingAuditService{}
	svc, repo := newCopyProviderService(t, permissiveProviderAuthz{}, audit)

	source := sourceProviderRow()
	source.APIKeyEncrypted = nil

	repo.EXPECT().GetByID(mock.Anything, testCopyProviderSourceTeamID, testCopyProviderID).
		Return(source, nil)
	repo.EXPECT().ListNames(mock.Anything, testProviderTeamID).Return(nil, nil)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

	copied, err := svc.CopyFromTeam(context.Background(), copyProviderParams())
	require.NoError(t, err)
	assert.Nil(t, copied.APIKeyEncrypted)
	assert.Equal(t, false, audit.records[0].Detail["has_api_key"])
}

func TestModelProviderService_CopyFromTeam_NameCollisionAppendsCopySuffix(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		want     string
	}{
		{"free", []string{"Anthropic"}, "OpenAI GPT-4o"},
		{"taken once", []string{"OpenAI GPT-4o"}, "OpenAI GPT-4o (copy)"},
		{
			"taken twice",
			[]string{"OpenAI GPT-4o", "OpenAI GPT-4o (copy)"},
			"OpenAI GPT-4o (copy 2)",
		},
		{
			"taken three times",
			[]string{"OpenAI GPT-4o", "OpenAI GPT-4o (copy)", "OpenAI GPT-4o (copy 2)"},
			"OpenAI GPT-4o (copy 3)",
		},
		{
			// A gap is reused rather than skipped past — the numbering tracks
			// what is free, not how many copies were ever made.
			"gap in the sequence",
			[]string{"OpenAI GPT-4o", "OpenAI GPT-4o (copy 2)"},
			"OpenAI GPT-4o (copy)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newCopyProviderService(t, permissiveProviderAuthz{}, &recordingAuditService{})

			repo.EXPECT().GetByID(mock.Anything, testCopyProviderSourceTeamID, testCopyProviderID).
				Return(sourceProviderRow(), nil)
			repo.EXPECT().ListNames(mock.Anything, testProviderTeamID).Return(tc.existing, nil)
			repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

			copied, err := svc.CopyFromTeam(context.Background(), copyProviderParams())
			require.NoError(t, err)
			assert.Equal(t, tc.want, copied.Name)
		})
	}
}

func TestModelProviderService_CopyFromTeam_ClientNameWinsOverTheGeneratedOne(t *testing.T) {
	svc, repo := newCopyProviderService(t, permissiveProviderAuthz{}, &recordingAuditService{})

	repo.EXPECT().GetByID(mock.Anything, testCopyProviderSourceTeamID, testCopyProviderID).
		Return(sourceProviderRow(), nil)
	// No ListNames call: a caller-supplied name is used verbatim, so there is
	// nothing to disambiguate against. mockery fails the test if one happens.
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

	params := copyProviderParams()
	chosen := "OpenAI GPT-4o"
	params.Name = &chosen

	copied, err := svc.CopyFromTeam(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, chosen, copied.Name,
		"a name the caller chose is never silently rewritten to '(copy)'")
}

func TestModelProviderService_CopyFromTeam_SuffixKeepsTheNameWithinTheColumn(t *testing.T) {
	tests := []struct {
		name     string
		longName string
	}{
		{"ascii", strings.Repeat("a", modelProviderNameMaxLen)},
		// varchar(255) counts CHARACTERS, so a name of 255 multi-byte runes is
		// exactly at the limit even though it is 1020 bytes. Trimming by byte
		// length would over-trim it and could cut mid-rune into invalid UTF-8,
		// which Postgres rejects outright.
		{"multi-byte", strings.Repeat("日", modelProviderNameMaxLen)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newCopyProviderService(t, permissiveProviderAuthz{}, &recordingAuditService{})

			source := sourceProviderRow()
			source.Name = tc.longName

			repo.EXPECT().GetByID(mock.Anything, testCopyProviderSourceTeamID, testCopyProviderID).
				Return(source, nil)
			repo.EXPECT().ListNames(mock.Anything, testProviderTeamID).
				Return([]string{tc.longName}, nil)
			repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

			copied, err := svc.CopyFromTeam(context.Background(), copyProviderParams())
			require.NoError(t, err)

			// Postgres would truncate a longer name into one the collision check
			// never cleared, so the BASE is trimmed and the suffix survives whole.
			assert.LessOrEqual(t, utf8.RuneCountInString(copied.Name), modelProviderNameMaxLen)
			assert.True(t, utf8.ValidString(copied.Name), "trimming must not split a rune: %q", copied.Name)
			assert.True(t, strings.HasSuffix(copied.Name, " (copy)"), "got %q", copied.Name)
		})
	}
}

func TestModelProviderService_CopyFromTeam_ClientNameCollisionIsAlreadyExists(t *testing.T) {
	svc, repo := newCopyProviderService(t, permissiveProviderAuthz{}, &recordingAuditService{})

	repo.EXPECT().GetByID(mock.Anything, testCopyProviderSourceTeamID, testCopyProviderID).
		Return(sourceProviderRow(), nil)
	repo.EXPECT().Create(mock.Anything, mock.Anything).
		Return(errors.New(`pq: duplicate key value violates unique constraint "unique_team_model_provider_name"`))

	params := copyProviderParams()
	chosen := "Already Taken"
	params.Name = &chosen

	_, err := svc.CopyFromTeam(context.Background(), params)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrModelProviderAlreadyExists), "got: %v", err)
}

func TestModelProviderService_CopyFromTeam_AppliesOverrides(t *testing.T) {
	svc, repo := newCopyProviderService(t, permissiveProviderAuthz{}, &recordingAuditService{})

	repo.EXPECT().GetByID(mock.Anything, testCopyProviderSourceTeamID, testCopyProviderID).
		Return(sourceProviderRow(), nil)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

	params := copyProviderParams()
	name, providerType, model, baseURL := "Staging", "openai_compatible", "gpt-4o", ""
	configuration := map[string]interface{}{"temperature": 0.1}
	params.Name = &name
	params.ProviderType = &providerType
	params.Model = &model
	params.BaseURL = &baseURL
	params.Configuration = &configuration

	copied, err := svc.CopyFromTeam(context.Background(), params)
	require.NoError(t, err)

	assert.Equal(t, "Staging", copied.Name)
	assert.Equal(t, "openai_compatible", copied.ProviderType)
	assert.Equal(t, "gpt-4o", copied.Model)
	assert.Nil(t, copied.BaseURL, "an empty base_url override clears it rather than storing \"\"")
	assert.JSONEq(t, `{"temperature":0.1}`, copied.Configuration)
	// An override never reaches the credential: that is not a field the caller
	// can set on this endpoint at all.
	require.NotNil(t, copied.APIKeyEncrypted)
	assert.Equal(t, *sourceProviderRow().APIKeyEncrypted, *copied.APIKeyEncrypted)
}

func TestModelProviderService_CopyFromTeam_DeniesIdenticallyForEitherTeamAndChecksDestinationFirst(t *testing.T) {
	for _, deniedTeam := range []string{testProviderTeamID, testCopyProviderSourceTeamID} {
		t.Run("denied on "+deniedTeam, func(t *testing.T) {
			var checked []string
			authorizer := denyProviderTeamsAuthz{
				denied:  map[string]bool{deniedTeam: true},
				checked: &checked,
			}
			// No repository expectations at all: a denied copy must not read
			// the source row, or its existence becomes observable.
			svc, _ := newCopyProviderService(t, authorizer, &recordingAuditService{})

			_, err := svc.CopyFromTeam(context.Background(), copyProviderParams())
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrPermissionDenied), "got: %v", err)
			assert.NotContains(t, err.Error(), testCopyProviderSourceTeamID,
				"a denial must not name the source team")

			require.NotEmpty(t, checked)
			assert.Equal(t, testProviderTeamID, checked[0],
				"the destination is authorized first, so a caller entitled to "+
					"neither learns nothing about the source")
		})
	}
}

func TestModelProviderService_CopyFromTeam_RejectsBadSource(t *testing.T) {
	tests := []struct {
		name         string
		sourceTeamID string
		want         error
	}{
		{"missing source", "", ErrCopySourceRequired},
		{"source is destination", testProviderTeamID, ErrCopySourceIsDestination},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newCopyProviderService(t, permissiveProviderAuthz{}, &recordingAuditService{})

			params := copyProviderParams()
			params.SourceTeamID = tc.sourceTeamID

			_, err := svc.CopyFromTeam(context.Background(), params)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tc.want), "got: %v", err)
		})
	}
}

func TestModelProviderService_CopyFromTeam_UnknownSourceProviderIsNotFound(t *testing.T) {
	svc, repo := newCopyProviderService(t, permissiveProviderAuthz{}, &recordingAuditService{})

	repo.EXPECT().GetByID(mock.Anything, testCopyProviderSourceTeamID, testCopyProviderID).
		Return(nil, repositories.ErrModelProviderNotFound)

	_, err := svc.CopyFromTeam(context.Background(), copyProviderParams())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrModelProviderNotFound), "got: %v", err)
}

func TestModelProviderService_CopyFromTeam_NilAuthzFailsClosed(t *testing.T) {
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	svc := NewModelProviderService(
		repomocks.NewMockModelProviderRepository(t), enc, localDevProviderConfig(),
		nil, &recordingAuditService{},
	)

	_, copyErr := svc.CopyFromTeam(context.Background(), copyProviderParams())
	require.Error(t, copyErr)
	assert.True(t, errors.Is(copyErr, ErrPermissionDenied), "got: %v", copyErr)
}
