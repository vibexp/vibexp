package server

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// Admin user mutations in the audit log (#1515).

type adminAuditContainer struct {
	BaseMockContainer
	admin    *servicesmocks.MockAdminServiceInterface
	accounts *servicesmocks.MockAdminAccountServiceInterface
	users    *repomocks.MockUserRepository
}

func (c *adminAuditContainer) AdminService() services.AdminServiceInterface { return c.admin }
func (c *adminAuditContainer) AdminAccountService() services.AdminAccountServiceInterface {
	return c.accounts
}
func (c *adminAuditContainer) UserRepository() repositories.UserRepository { return c.users }

type adminAuditFixture struct {
	admin    *adminStrictServer
	svc      *servicesmocks.MockAdminServiceInterface
	accounts *servicesmocks.MockAdminAccountServiceInterface
	users    *repomocks.MockUserRepository
	rec      *auditCapture
	userID   uuid.UUID
	ctx      context.Context
}

func newAdminAuditFixture(t *testing.T) *adminAuditFixture {
	t.Helper()
	f := &adminAuditFixture{
		svc:      servicesmocks.NewMockAdminServiceInterface(t),
		accounts: servicesmocks.NewMockAdminAccountServiceInterface(t),
		users:    repomocks.NewMockUserRepository(t),
		userID:   uuid.New(),
		ctx:      context.WithValue(context.Background(), contextKeyUserID, "admin-1"),
	}
	srv := newServerWithNullLogger(t)
	srv.container = &adminAuditContainer{admin: f.svc, accounts: f.accounts, users: f.users}
	f.rec = withAudit(srv)
	f.admin = &adminStrictServer{s: srv}
	return f
}

func (f *adminAuditFixture) detail(name string) *models.AdminUserDetail {
	return &models.AdminUserDetail{ID: f.userID.String(), Email: "grace@example.com", Name: name}
}

func TestAuditAdmin_UserCreatedWithTeams(t *testing.T) {
	f := newAdminAuditFixture(t)
	teamID := uuid.New()
	f.svc.EXPECT().CreateUser(mock.Anything, mock.Anything).Return(f.detail("Grace"), nil)
	f.accounts.EXPECT().SetUpNewUser(mock.Anything, "admin-1", f.userID.String()).
		Return(services.AdminUserSetup{Delivery: "none"})
	teams := []admingen.AdminUserTeamAssignment{{TeamId: teamID, Role: "admin"}}

	_, err := f.admin.CreateAdminUser(f.ctx, admingen.CreateAdminUserRequestObject{
		Body: &admingen.CreateAdminUserJSONRequestBody{Email: "grace@example.com", Teams: &teams},
	})

	require.NoError(t, err)
	got := f.rec.events()
	require.Len(t, got, 2, "the user, and the membership it gained")
	assert.Equal(t, models.AuditOpAdminUserCreated, got[0].event.Operation)
	assert.Equal(t, f.userID.String(), got[0].event.ResourceID)
	assert.Equal(t, []string{teamID.String()}, got[0].event.Metadata["teams"])
	assert.Equal(t, models.AuditOpTeamMemberAdded, got[1].event.Operation)
	assert.Equal(t, teamID.String(), got[1].event.TeamID)
	assert.Equal(t, "admin", got[1].event.Metadata["role"])
}

func TestAuditAdmin_UserUpdatedRecordsOldAndNewName(t *testing.T) {
	f := newAdminAuditFixture(t)
	f.users.EXPECT().GetByID(mock.Anything, f.userID.String()).Return(&models.User{Name: "Grace H"}, nil)
	f.svc.EXPECT().UpdateUserName(mock.Anything, f.userID.String(), "Grace Hopper").Return(f.detail("Grace Hopper"), nil)

	_, err := f.admin.UpdateAdminUser(f.ctx, admingen.UpdateAdminUserRequestObject{
		Id: f.userID, Body: &admingen.UpdateAdminUserJSONRequestBody{Name: "Grace Hopper"},
	})

	require.NoError(t, err)
	got := f.rec.only(t)
	assert.Equal(t, models.AuditOpAdminUserUpdated, got.event.Operation)
	assert.Equal(t, map[string]any{"old": "Grace H", "new": "Grace Hopper"}, got.event.Metadata["name"])
}

func TestAuditAdmin_SuspendAndReactivate(t *testing.T) {
	f := newAdminAuditFixture(t)
	f.svc.EXPECT().SuspendUser(mock.Anything, "admin-1", f.userID.String(), mock.Anything).Return(f.detail("Grace"), nil)
	f.svc.EXPECT().ReactivateUser(mock.Anything, f.userID.String()).Return(f.detail("Grace"), nil)

	_, err := f.admin.SuspendAdminUser(f.ctx, admingen.SuspendAdminUserRequestObject{Id: f.userID})
	require.NoError(t, err)
	_, err = f.admin.ReactivateAdminUser(f.ctx, admingen.ReactivateAdminUserRequestObject{Id: f.userID})
	require.NoError(t, err)

	got := f.rec.events()
	require.Len(t, got, 2)
	assert.Equal(t, models.AuditOpAdminUserSuspended, got[0].event.Operation)
	assert.Equal(t, models.AuditOpAdminUserUnsuspended, got[1].event.Operation)
	assert.Equal(t, "grace@example.com", got[1].event.ResourceName)
}

func TestAuditAdmin_UserDeleted(t *testing.T) {
	f := newAdminAuditFixture(t)
	f.svc.EXPECT().DeleteUser(mock.Anything, "admin-1", f.userID.String(), mock.Anything).
		Return("grace@example.com", true, nil)

	_, err := f.admin.DeleteAdminUser(f.ctx, admingen.DeleteAdminUserRequestObject{Id: f.userID})

	require.NoError(t, err)
	got := f.rec.only(t)
	assert.Equal(t, models.AuditOpAdminUserDeleted, got.event.Operation)
	assert.Equal(t, "grace@example.com", got.event.ResourceName, "the email the service returns, the row being gone")
}

func TestAuditAdmin_RefusedActionRecordsNothing(t *testing.T) {
	f := newAdminAuditFixture(t)
	f.svc.EXPECT().DeleteUser(mock.Anything, "admin-1", f.userID.String(), mock.Anything).Return("", false, nil)

	_, err := f.admin.DeleteAdminUser(f.ctx, admingen.DeleteAdminUserRequestObject{Id: f.userID})

	require.Error(t, err)
	assert.Empty(t, f.rec.events())
}

func TestAuditAdmin_PanickingRecorderNeverChangesTheResponse(t *testing.T) {
	f := newAdminAuditFixture(t)
	f.admin.s.container = auditingContainer{Container: f.admin.s.container.(auditingContainer).Container, rec: panickyRecorder{}}
	f.svc.EXPECT().SuspendUser(mock.Anything, "admin-1", f.userID.String(), mock.Anything).Return(f.detail("Grace"), nil)

	resp, err := f.admin.SuspendAdminUser(f.ctx, admingen.SuspendAdminUserRequestObject{Id: f.userID})

	require.NoError(t, err)
	assert.IsType(t, admingen.SuspendAdminUser200JSONResponse{}, resp)
}

func TestAdminUserNameForAudit_SkippedWithoutRecorder(t *testing.T) {
	srv := newServerWithNullLogger(t)
	srv.container = &BaseMockContainer{}
	assert.Empty(t, srv.adminUserNameForAudit(context.Background(), "u-1"))
}
