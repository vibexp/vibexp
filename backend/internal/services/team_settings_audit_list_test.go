package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// Read path of the settings audit log (#832). The write path's tests live in
// team_settings_audit_test.go; these cover the three things the read adds —
// the permission gate, page→offset arithmetic, and name resolution.

// gateAuthz records the exact (userID, teamID, permission) triple it was asked
// about and answers with err. A recording double rather than allowAllAuthz
// because WHICH permission is checked is the assertion: the acceptance criteria
// name team.settings.update specifically, and an allow-all double would pass
// just as happily if the service asked for a weaker one.
type gateAuthz struct {
	userID string
	teamID string
	perm   authz.Permission
	calls  int
	err    error
}

func (g *gateAuthz) Can(_ context.Context, userID, teamID string, perm authz.Permission) error {
	g.userID, g.teamID, g.perm = userID, teamID, perm
	g.calls++
	return g.err
}

func (g *gateAuthz) IsMember(_ context.Context, _, _ string) error {
	panic("gateAuthz: unexpected IsMember call — the audit read gates on a permission, not membership")
}

func (g *gateAuthz) CanActOnResource(_ context.Context, _, _, _ string, _, _ authz.Permission) error {
	panic("gateAuthz: unexpected CanActOnResource call")
}

func (g *gateAuthz) Authorize(
	_ context.Context, _, _ string, _ authz.Permission,
) (models.TeamMemberRole, error) {
	panic("gateAuthz: unexpected Authorize call")
}

type auditListFixture struct {
	svc   *TeamSettingsAuditService
	repo  *mocks.MockTeamSettingsAuditRepository
	users *mocks.MockUserRepository
	teams *mocks.MockTeamRepository
	gate  *gateAuthz
}

func newAuditListFixture(t *testing.T) *auditListFixture {
	t.Helper()
	f := &auditListFixture{
		repo:  mocks.NewMockTeamSettingsAuditRepository(t),
		users: mocks.NewMockUserRepository(t),
		teams: mocks.NewMockTeamRepository(t),
		gate:  &gateAuthz{},
	}
	f.svc = NewTeamSettingsAuditService(f.repo, f.gate, f.users, f.teams, slog.Default())
	return f
}

func auditEntry(id, actorID, sourceTeamID string) *models.TeamSettingsAudit {
	entry := &models.TeamSettingsAudit{
		ID:        id,
		TeamID:    "team-1",
		Surface:   models.SettingsAuditSurfaceModelProvider,
		CreatedAt: time.Now(),
	}
	if actorID != "" {
		entry.ActorUserID = &actorID
	}
	if sourceTeamID != "" {
		entry.SourceTeamID = &sourceTeamID
	}
	return entry
}

func TestTeamSettingsAuditService_ListAudit_RequiresTeamSettingsUpdate(t *testing.T) {
	f := newAuditListFixture(t)
	f.gate.err = ErrPermissionDenied

	page, err := f.svc.ListAudit(context.Background(), "user-1", "team-1", 1, 20)

	require.ErrorIs(t, err, ErrPermissionDenied)
	assert.Nil(t, page)
	// The gate must be the FIRST thing that happens: the mock repository is
	// constructed with no expectations, so any read at all would fail the test.
	assert.Equal(t, 1, f.gate.calls)
	assert.Equal(t, authz.TeamSettingsUpdate, f.gate.perm)
	assert.Equal(t, "user-1", f.gate.userID)
	assert.Equal(t, "team-1", f.gate.teamID)
}

func TestTeamSettingsAuditService_ListAudit_TranslatesPageToOffset(t *testing.T) {
	tests := []struct {
		name                            string
		page, limit                     int
		wantLimit, wantOffset           int
		wantPageEcho, wantPerPageEchoed int
	}{
		{"first page", 1, 20, 20, 0, 1, 20},
		{"third page", 3, 20, 20, 40, 3, 20},
		{"custom limit", 2, 5, 5, 5, 2, 5},
		{"page below one is clamped", 0, 20, 20, 0, 1, 20},
		{"negative page is clamped", -7, 20, 20, 0, 1, 20},
		{"limit below one falls back to the spec default", 1, 0, 20, 0, 1, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAuditListFixture(t)
			f.repo.EXPECT().
				ListByTeam(mock.Anything, "team-1", tt.wantLimit, tt.wantOffset).
				Return(nil, 0, nil)
			f.users.EXPECT().GetNamesByIDs(mock.Anything, mock.Anything).
				Return(map[string]string{}, nil)
			f.teams.EXPECT().GetNamesByIDs(mock.Anything, mock.Anything).
				Return(map[string]string{}, nil)

			page, err := f.svc.ListAudit(context.Background(), "user-1", "team-1", tt.page, tt.limit)

			require.NoError(t, err)
			assert.Equal(t, tt.wantPageEcho, page.Page)
			assert.Equal(t, tt.wantPerPageEchoed, page.PerPage)
			// An empty page must be an empty slice, never nil: the handler's
			// required `entries` array is built from it.
			assert.NotNil(t, page.Entries)
			assert.Empty(t, page.Entries)
		})
	}
}

func TestTeamSettingsAuditService_ListAudit_ResolvesNames(t *testing.T) {
	f := newAuditListFixture(t)
	entries := []*models.TeamSettingsAudit{
		auditEntry("a1", "user-9", "team-src"),
		auditEntry("a2", "user-9", "team-src"), // same pair, to prove deduplication
	}
	f.repo.EXPECT().ListByTeam(mock.Anything, "team-1", 20, 0).Return(entries, 2, nil)
	// Each id appears ONCE despite two rows naming it — one lookup per page,
	// not one per row.
	f.users.EXPECT().GetNamesByIDs(mock.Anything, []string{"user-9"}).
		Return(map[string]string{"user-9": "Ada Lovelace"}, nil)
	f.teams.EXPECT().GetNamesByIDs(mock.Anything, []string{"team-src"}).
		Return(map[string]string{"team-src": "Platform"}, nil)

	page, err := f.svc.ListAudit(context.Background(), "user-1", "team-1", 1, 20)

	require.NoError(t, err)
	require.Len(t, page.Entries, 2)
	assert.Equal(t, 2, page.TotalCount)
	for _, view := range page.Entries {
		require.NotNil(t, view.ActorName)
		assert.Equal(t, "Ada Lovelace", *view.ActorName)
		require.NotNil(t, view.SourceTeamName)
		assert.Equal(t, "Platform", *view.SourceTeamName)
	}
}

// The criterion the whole endpoint turns on: a deleted source team, and a
// deleted actor, must render — not error, and not be silently dropped.
func TestTeamSettingsAuditService_ListAudit_DeletedActorAndSourceTeam(t *testing.T) {
	f := newAuditListFixture(t)
	entries := []*models.TeamSettingsAudit{
		auditEntry("a1", "", "team-gone"), // actor_user_id was SET NULL on delete
		auditEntry("a2", "user-9", "team-gone"),
	}
	f.repo.EXPECT().ListByTeam(mock.Anything, "team-1", 20, 0).Return(entries, 2, nil)
	f.users.EXPECT().GetNamesByIDs(mock.Anything, []string{"user-9"}).
		Return(map[string]string{"user-9": "Ada Lovelace"}, nil)
	// The source team resolves to nothing — it has no foreign key precisely so
	// it can be deleted with the entry still standing.
	f.teams.EXPECT().GetNamesByIDs(mock.Anything, []string{"team-gone"}).
		Return(map[string]string{}, nil)

	page, err := f.svc.ListAudit(context.Background(), "user-1", "team-1", 1, 20)

	require.NoError(t, err, "a deleted source team must not fail the read")
	require.Len(t, page.Entries, 2, "neither entry may be dropped")

	assert.Nil(t, page.Entries[0].ActorName, "a null actor id resolves to no name")
	assert.Nil(t, page.Entries[0].SourceTeamName)
	assert.Nil(t, page.Entries[1].SourceTeamName, "a deleted source team resolves to no name")
	require.NotNil(t, page.Entries[1].ActorName)
	// The ids survive even when the names do not — they are the audit record.
	require.NotNil(t, page.Entries[1].Entry.SourceTeamID)
	assert.Equal(t, "team-gone", *page.Entries[1].Entry.SourceTeamID)
}

// A name lookup is a legibility aid, not the record. Its failure degrades to
// ids rather than denying the team their audit log.
func TestTeamSettingsAuditService_ListAudit_NameLookupFailureDegrades(t *testing.T) {
	for _, tt := range []struct {
		name      string
		failUsers bool
	}{
		{"actor lookup fails", true},
		{"source team lookup fails", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newAuditListFixture(t)
			entries := []*models.TeamSettingsAudit{auditEntry("a1", "user-9", "team-src")}
			f.repo.EXPECT().ListByTeam(mock.Anything, "team-1", 20, 0).Return(entries, 1, nil)

			boom := errors.New("name lookup exploded")
			if tt.failUsers {
				f.users.EXPECT().GetNamesByIDs(mock.Anything, mock.Anything).Return(nil, boom)
				f.teams.EXPECT().GetNamesByIDs(mock.Anything, mock.Anything).
					Return(map[string]string{"team-src": "Platform"}, nil)
			} else {
				f.users.EXPECT().GetNamesByIDs(mock.Anything, mock.Anything).
					Return(map[string]string{"user-9": "Ada Lovelace"}, nil)
				f.teams.EXPECT().GetNamesByIDs(mock.Anything, mock.Anything).Return(nil, boom)
			}

			page, err := f.svc.ListAudit(context.Background(), "user-1", "team-1", 1, 20)

			require.NoError(t, err, "a failed name lookup must not fail the page")
			require.Len(t, page.Entries, 1)
			if tt.failUsers {
				assert.Nil(t, page.Entries[0].ActorName)
				assert.NotNil(t, page.Entries[0].SourceTeamName)
			} else {
				assert.NotNil(t, page.Entries[0].ActorName)
				assert.Nil(t, page.Entries[0].SourceTeamName)
			}
		})
	}
}

func TestTeamSettingsAuditService_ListAudit_RepositoryErrorIsWrapped(t *testing.T) {
	f := newAuditListFixture(t)
	f.repo.EXPECT().ListByTeam(mock.Anything, "team-1", 20, 0).
		Return(nil, 0, errors.New("connection reset"))

	page, err := f.svc.ListAudit(context.Background(), "user-1", "team-1", 1, 20)

	require.Error(t, err)
	assert.Nil(t, page)
	assert.Contains(t, err.Error(), "TeamSettingsAuditService.ListAudit")
	assert.Contains(t, err.Error(), "connection reset")
}
