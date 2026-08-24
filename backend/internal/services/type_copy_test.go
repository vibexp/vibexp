package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

const testCopySourceTeamID = "team-2"

// recordingAuditService captures the entries a copy writes. Hand-written rather
// than generated: internal/services/mocks imports internal/services, so a test
// in package services cannot import it without an import cycle (the same reason
// allowAllAuthz exists).
type recordingAuditService struct {
	records []TeamSettingsAuditRecord
	err     error
}

func (r *recordingAuditService) Record(
	_ context.Context, record TeamSettingsAuditRecord,
) (*models.TeamSettingsAudit, error) {
	r.records = append(r.records, record)
	if r.err != nil {
		return nil, r.err
	}
	return &models.TeamSettingsAudit{TeamID: record.TeamID, Surface: record.Surface}, nil
}

// denyTeamsAuthz denies membership for the named teams and allows every other,
// so a test can refuse exactly one side of a copy.
type denyTeamsAuthz struct {
	denied map[string]bool
	// checked records every team asked about, in order, so a test can assert
	// the destination is evaluated first.
	checked *[]string
}

func (d denyTeamsAuthz) IsMember(_ context.Context, _, teamID string) error {
	if d.checked != nil {
		*d.checked = append(*d.checked, teamID)
	}
	if d.denied[teamID] {
		return ErrPermissionDenied
	}
	return nil
}

func (d denyTeamsAuthz) Can(_ context.Context, _, _ string, _ authz.Permission) error {
	panic("denyTeamsAuthz: a membership-only copy must not consult the permission matrix")
}

func (d denyTeamsAuthz) CanActOnResource(
	_ context.Context, _, _, _ string, _, _ authz.Permission,
) error {
	panic("denyTeamsAuthz: unexpected CanActOnResource call")
}

func (d denyTeamsAuthz) Authorize(
	_ context.Context, _, _ string, _ authz.Permission,
) (models.TeamMemberRole, error) {
	panic("denyTeamsAuthz: unexpected Authorize call")
}

func newCopyTypeService(
	t *testing.T, authorizer AuthorizationServiceInterface, audit *recordingAuditService,
) (*TypeService, *repomocks.MockTypeRepository) {
	t.Helper()
	repo := repomocks.NewMockTypeRepository(t)
	return NewTypeService(repo, authorizer, audit, slog.New(slog.DiscardHandler)), repo
}

func copyParams() CopyTypesParams {
	return CopyTypesParams{
		TeamID:       testTypeTeamID,
		SourceTeamID: testCopySourceTeamID,
		UserID:       testTypeUserID,
	}
}

func TestTypeService_CopyFromTeam_AddsMissingSkipsExistingExcludesSystem(t *testing.T) {
	audit := &recordingAuditService{}
	svc, repo := newCopyTypeService(t, allowAllAuthz{}, audit)

	repo.EXPECT().List(mock.Anything, testCopySourceTeamID, "artifacts").Return([]models.Type{
		// A global default. It exists in the destination too, so copying it
		// would report the whole system set as skipped.
		{ID: "sys-1", Slug: "general", Name: "General", ResourceType: "artifacts", IsSystem: true},
		{ID: "src-1", Slug: "bug-report", Name: "Bug report", ResourceType: "artifacts"},
		{ID: "src-2", Slug: "runbook", Name: "Runbook", ResourceType: "artifacts"},
	}, nil)

	repo.EXPECT().GetBySlug(mock.Anything, testTypeTeamID, "artifacts", "bug-report").
		Return(nil, repositories.ErrTypeNotFound)
	repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(tp *models.Type) bool {
		return tp.Slug == "bug-report" && tp.TeamID == testTypeTeamID &&
			tp.Name == "Bug report" && tp.CreatedBy == testTypeUserID
	})).Run(func(_ context.Context, tp *models.Type) {
		tp.ID = "dst-1" // the destination's own id, as the repository assigns it
	}).Return(nil)

	// Already taken in the destination — a skip, never an error.
	repo.EXPECT().GetBySlug(mock.Anything, testTypeTeamID, "artifacts", "runbook").
		Return(&models.Type{ID: "dst-existing", Slug: "runbook"}, nil)

	result, err := svc.CopyFromTeam(context.Background(), copyParams())
	require.NoError(t, err)

	require.Len(t, result.Added, 1)
	assert.Equal(t, "bug-report", result.Added[0].Slug)
	// The response must carry the DESTINATION's row, not the source's, or the
	// client would render a type id that belongs to another team.
	assert.Equal(t, "dst-1", result.Added[0].ID)
	assert.Equal(t, testTypeTeamID, result.Added[0].TeamID)

	require.Len(t, result.Skipped, 1)
	assert.Equal(t, SkippedType{ResourceType: "artifacts", Slug: "runbook"}, result.Skipped[0])

	require.Len(t, audit.records, 1, "one copy action writes exactly one audit row")
	entry := audit.records[0]
	assert.Equal(t, testTypeTeamID, entry.TeamID)
	assert.Equal(t, testCopySourceTeamID, entry.SourceTeamID)
	assert.Equal(t, models.SettingsAuditSurfaceCustomTypes, entry.Surface)
	assert.Equal(t, testTypeUserID, entry.ActorUserID)
	assert.Equal(t, []string{"dst-1"}, entry.Detail["added_ids"])
	assert.Equal(t, []string{"bug-report"}, entry.Detail["added_slugs"])
	assert.Equal(t, []string{"runbook"}, entry.Detail["skipped_slugs"])
}

func TestTypeService_CopyFromTeam_ConcurrentCreateIsSkipped(t *testing.T) {
	audit := &recordingAuditService{}
	svc, repo := newCopyTypeService(t, allowAllAuthz{}, audit)

	repo.EXPECT().List(mock.Anything, testCopySourceTeamID, "artifacts").Return([]models.Type{
		{ID: "src-1", Slug: "bug-report", Name: "Bug report", ResourceType: "artifacts"},
	}, nil)
	repo.EXPECT().GetBySlug(mock.Anything, testTypeTeamID, "artifacts", "bug-report").
		Return(nil, repositories.ErrTypeNotFound)
	// The pre-check is not a lock: another writer got there first.
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(repositories.ErrTypeAlreadyExists)

	result, err := svc.CopyFromTeam(context.Background(), copyParams())
	require.NoError(t, err, "ErrTypeAlreadyExists per item must never fail the whole copy")
	assert.Empty(t, result.Added)
	require.Len(t, result.Skipped, 1)
	assert.Equal(t, "bug-report", result.Skipped[0].Slug)
}

func TestTypeService_CopyFromTeam_EmptySourceStillAudits(t *testing.T) {
	audit := &recordingAuditService{}
	svc, repo := newCopyTypeService(t, allowAllAuthz{}, audit)

	repo.EXPECT().List(mock.Anything, testCopySourceTeamID, "artifacts").
		Return([]models.Type{}, nil)

	result, err := svc.CopyFromTeam(context.Background(), copyParams())
	require.NoError(t, err)
	// Non-nil by construction, so the handler serializes [] and never null.
	assert.NotNil(t, result.Added)
	assert.NotNil(t, result.Skipped)
	assert.Empty(t, result.Added)
	assert.Len(t, audit.records, 1)
}

func TestTypeService_CopyFromTeam_DeniesBothSidesIdentically(t *testing.T) {
	denials := []struct {
		name   string
		denied string
	}{
		{"destination", testTypeTeamID},
		{"source", testCopySourceTeamID},
	}

	var messages []string
	for _, tc := range denials {
		t.Run(tc.name, func(t *testing.T) {
			audit := &recordingAuditService{}
			svc, _ := newCopyTypeService(t,
				denyTeamsAuthz{denied: map[string]bool{tc.denied: true}}, audit)

			_, err := svc.CopyFromTeam(context.Background(), copyParams())
			require.ErrorIs(t, err, ErrPermissionDenied)
			assert.NotContains(t, err.Error(), tc.denied,
				"the error must not name the team, or it leaks which one the caller lacks")
			assert.Empty(t, audit.records, "a denied copy writes no audit entry")
			messages = append(messages, err.Error())
		})
	}

	require.Len(t, messages, 2)
	assert.Equal(t, messages[0], messages[1],
		"a non-member of the source and of the destination must be indistinguishable")
}

func TestTypeService_CopyFromTeam_ChecksDestinationBeforeSource(t *testing.T) {
	var checked []string
	audit := &recordingAuditService{}
	// Deny BOTH: only destination-first ordering explains a single check.
	svc, _ := newCopyTypeService(t, denyTeamsAuthz{
		denied:  map[string]bool{testTypeTeamID: true, testCopySourceTeamID: true},
		checked: &checked,
	}, audit)

	_, err := svc.CopyFromTeam(context.Background(), copyParams())
	require.ErrorIs(t, err, ErrPermissionDenied)
	assert.Equal(t, []string{testTypeTeamID}, checked,
		"the destination is evaluated first and short-circuits")
}

func TestTypeService_CopyFromTeam_RejectsBadSource(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   error
	}{
		{"empty", "", ErrCopySourceRequired},
		{"same as destination", testTypeTeamID, ErrCopySourceIsDestination},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			audit := &recordingAuditService{}
			svc, _ := newCopyTypeService(t, allowAllAuthz{}, audit)

			params := copyParams()
			params.SourceTeamID = tc.source
			_, err := svc.CopyFromTeam(context.Background(), params)
			assert.ErrorIs(t, err, tc.want)
			assert.Empty(t, audit.records)
		})
	}
}

func TestTypeService_CopyFromTeam_AuditFailureFailsTheCopy(t *testing.T) {
	sentinel := errors.New("audit storage down")
	audit := &recordingAuditService{err: sentinel}
	svc, repo := newCopyTypeService(t, allowAllAuthz{}, audit)

	repo.EXPECT().List(mock.Anything, testCopySourceTeamID, "artifacts").Return([]models.Type{
		{ID: "src-1", Slug: "bug-report", Name: "Bug report", ResourceType: "artifacts"},
	}, nil)
	repo.EXPECT().GetBySlug(mock.Anything, testTypeTeamID, "artifacts", "bug-report").
		Return(nil, repositories.ErrTypeNotFound)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

	_, err := svc.CopyFromTeam(context.Background(), copyParams())
	require.ErrorIs(t, err, sentinel,
		"the audit entry is the compensating control; a copy whose audit did not land must not report success")
}

func TestTypeService_CopyFromTeam_PropagatesListFailure(t *testing.T) {
	sentinel := errors.New("boom")
	audit := &recordingAuditService{}
	svc, repo := newCopyTypeService(t, allowAllAuthz{}, audit)

	repo.EXPECT().List(mock.Anything, testCopySourceTeamID, "artifacts").Return(nil, sentinel)

	_, err := svc.CopyFromTeam(context.Background(), copyParams())
	assert.ErrorIs(t, err, sentinel)
	assert.Empty(t, audit.records)
}

func TestCopyableResourceTypes_CoversEveryCustomisableResource(t *testing.T) {
	got := copyableResourceTypes()
	assert.Len(t, got, len(resourceTypeDefaultSlug),
		"a resource that adopts custom types must be copied too — derive, never hardcode")
	assert.Contains(t, got, "artifacts")
	// Fixed order, so one copy of the same two teams always reports the same
	// result and writes the same audit detail.
	assert.IsIncreasing(t, append([]string{""}, got...))
}
