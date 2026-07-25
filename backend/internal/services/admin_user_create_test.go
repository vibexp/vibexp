package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/pkg/events"
)

// stubPublisher records what was published and can be made to fail.
type stubPublisher struct {
	published []events.Event
	err       error
}

func (p *stubPublisher) Publish(_ context.Context, event events.Event) error {
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, event)
	return nil
}

const createdUserID = "new-user-1"

func newCreateService(
	t *testing.T,
) (AdminServiceInterface, *repomocks.MockAdminRepository, *repomocks.MockUserRepository, *stubPublisher) {
	t.Helper()
	adminRepo := repomocks.NewMockAdminRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	pub := &stubPublisher{}
	return NewAdminService(adminRepo, userRepo, pub), adminRepo, userRepo, pub
}

// createdDetail is what the follow-up read returns.
func createdDetail() *models.AdminUserDetail {
	return &models.AdminUserDetail{
		ID:     createdUserID,
		Email:  "new.user@example.com",
		Name:   "New User",
		Status: models.UserStatusActive,
	}
}

// TestCreateUser_PublishesUserCreated is the heart of this issue.
//
// The personal "Private Workspace" team, its owner membership,
// users.default_team_id and the default project are ALL created by
// TeamCreationListener in response to `user.created`. TeamService.CreateDefaultTeam
// has no other production caller. So publishing that event is not an optional
// notification — it IS the provisioning, and an implementation that skips it
// produces an account with no workspace and no project.
func TestCreateUser_PublishesUserCreated(t *testing.T) {
	svc, adminRepo, userRepo, pub := newCreateService(t)

	userRepo.On("GetByEmail", mock.Anything, "new.user@example.com").
		Return(nil, repositories.ErrUserNotFound)
	userRepo.On("Create", mock.Anything, mock.MatchedBy(func(u *models.User) bool {
		// Email normalized, IdP identity deliberately unset until first sign-in.
		return u.Email == "new.user@example.com" && u.Name == "New User" &&
			u.GoogleID == nil && u.IDPSubject == nil && u.Status == models.UserStatusActive
	})).Run(func(args mock.Arguments) {
		args.Get(1).(*models.User).ID = createdUserID
	}).Return(nil)
	adminRepo.On("GetUserDetail", mock.Anything, createdUserID).Return(createdDetail(), nil)

	got, err := svc.CreateUser(context.Background(), AdminUserCreateRequest{
		Email: "  New.User@Example.com  ", Name: "  New User  ",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, createdUserID, got.ID)

	// The event, with the payload the listener reads.
	require.Len(t, pub.published, 1, "exactly one user.created event must be published")
	event := pub.published[0]
	assert.Equal(t, events.EventTypeUserCreated, event.Type())
	payload, ok := event.Payload().(*events.UserCreatedPayload)
	require.True(t, ok, "the listener casts to *UserCreatedPayload; a different payload silently no-ops it")
	assert.Equal(t, createdUserID, payload.UserID)
	assert.Equal(t, "new.user@example.com", payload.Email)
	assert.Equal(t, "New User", payload.Name)
}

// TestCreateUser_NormalizesEmailAndName pins the trimming/lowercasing, since the
// uniqueness constraint and every later lookup depend on the stored form.
func TestCreateUser_NormalizesEmailAndName(t *testing.T) {
	svc, adminRepo, userRepo, _ := newCreateService(t)

	userRepo.On("GetByEmail", mock.Anything, "mixed.case@example.com").
		Return(nil, repositories.ErrUserNotFound)
	var stored *models.User
	userRepo.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		stored = args.Get(1).(*models.User)
		stored.ID = createdUserID
	}).Return(nil)
	adminRepo.On("GetUserDetail", mock.Anything, createdUserID).Return(createdDetail(), nil)

	_, err := svc.CreateUser(context.Background(), AdminUserCreateRequest{
		Email: "\tMixed.Case@EXAMPLE.com \n", Name: "  Spaced Name  ",
	})
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "mixed.case@example.com", stored.Email)
	assert.Equal(t, "Spaced Name", stored.Name)
}

// TestCreateUser_DuplicateEmail covers both duplicate routes: the pre-check and
// the unique-constraint violation. The second matters because two concurrent
// creates can both pass the pre-check, and only the constraint serializes them.
func TestCreateUser_DuplicateEmail(t *testing.T) {
	t.Run("caught by the pre-check", func(t *testing.T) {
		svc, _, userRepo, pub := newCreateService(t)
		userRepo.On("GetByEmail", mock.Anything, "taken@example.com").
			Return(&models.User{ID: "existing"}, nil)

		// Create has NO expectation: nothing may be written.
		got, err := svc.CreateUser(context.Background(), AdminUserCreateRequest{
			Email: "taken@example.com", Name: "Someone",
		})
		assert.Nil(t, got)
		var takenErr *ErrAdminUserEmailTaken
		require.ErrorAs(t, err, &takenErr)
		assert.Equal(t, "taken@example.com", takenErr.Email)
		assert.Empty(t, pub.published, "no event for a rejected create")
	})

	t.Run("caught by the unique constraint after a racing insert", func(t *testing.T) {
		svc, _, userRepo, pub := newCreateService(t)
		userRepo.On("GetByEmail", mock.Anything, "taken@example.com").
			Return(nil, repositories.ErrUserNotFound)
		userRepo.On("Create", mock.Anything, mock.Anything).
			Return(fmt.Errorf("%w: taken@example.com", repositories.ErrUserEmailTaken))

		got, err := svc.CreateUser(context.Background(), AdminUserCreateRequest{
			Email: "taken@example.com", Name: "Someone",
		})
		assert.Nil(t, got)
		var takenErr *ErrAdminUserEmailTaken
		require.ErrorAs(t, err, &takenErr, "a unique violation must still be a 409, not a 500")
		assert.Empty(t, pub.published)
	})
}

// TestCreateUser_PublishFailureRollsBackTheUser is the compensating-action test.
//
// Publish is asynchronous and DROPS the event when its channel is full, returning
// an error. AuthService merely logs that, which is how sign-up can leave an
// unprovisioned user behind. The admin path must not: with no event, nothing will
// ever create this user's workspace, so the row is removed and the request fails
// rather than handing an admin a broken account.
func TestCreateUser_PublishFailureRollsBackTheUser(t *testing.T) {
	adminRepo := repomocks.NewMockAdminRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	pub := &stubPublisher{err: errors.New("event channel full, event dropped")}
	svc := NewAdminService(adminRepo, userRepo, pub)

	userRepo.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound)
	userRepo.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*models.User).ID = createdUserID
	}).Return(nil)
	// The decisive expectation: the row we just inserted is deleted again.
	userRepo.On("DeleteByID", mock.Anything, createdUserID).Return(nil).Once()

	got, err := svc.CreateUser(context.Background(), AdminUserCreateRequest{
		Email: "new.user@example.com", Name: "New User",
	})
	assert.Nil(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to provision the new user")
	userRepo.AssertExpectations(t)
}

// TestCreateUser_PublishAndRollbackBothFail must surface BOTH failures, because
// this is the one case that does leave an orphaned row an operator has to clean
// up by hand — the message has to say so.
func TestCreateUser_PublishAndRollbackBothFail(t *testing.T) {
	adminRepo := repomocks.NewMockAdminRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	pub := &stubPublisher{err: errors.New("bus not running")}
	svc := NewAdminService(adminRepo, userRepo, pub)

	userRepo.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound)
	userRepo.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*models.User).ID = createdUserID
	}).Return(nil)
	userRepo.On("DeleteByID", mock.Anything, createdUserID).Return(errors.New("db down"))

	_, err := svc.CreateUser(context.Background(), AdminUserCreateRequest{
		Email: "new.user@example.com", Name: "New User",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bus not running", "the original cause must survive")
	assert.Contains(t, err.Error(), "failed to roll back", "and the failed cleanup must be visible")
	assert.Contains(t, err.Error(), createdUserID, "naming the row an operator has to clean up")
}

// TestCreateUser_UnwiredDependencies: a read-only wiring reports a clear error
// rather than a nil-pointer panic.
func TestCreateUser_UnwiredDependencies(t *testing.T) {
	svc := newReadOnlyAdminService(repomocks.NewMockAdminRepository(t))

	got, err := svc.CreateUser(context.Background(), AdminUserCreateRequest{
		Email: "new.user@example.com", Name: "New User",
	})
	assert.Nil(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not wired")
}

// TestCreateUser_LookupFailurePropagates keeps a transient lookup error from
// being mistaken for "email is free".
func TestCreateUser_LookupFailurePropagates(t *testing.T) {
	svc, _, userRepo, pub := newCreateService(t)
	userRepo.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, errors.New("db down"))

	_, err := svc.CreateUser(context.Background(), AdminUserCreateRequest{
		Email: "new.user@example.com", Name: "New User",
	})
	require.Error(t, err)
	assert.Empty(t, pub.published)
}

// TestAdminUserEmailTakenMessage pins the 409 detail an admin reads.
func TestAdminUserEmailTakenMessage(t *testing.T) {
	err := &ErrAdminUserEmailTaken{Email: "taken@example.com"}
	assert.Contains(t, err.Error(), "taken@example.com")
	assert.Contains(t, err.Error(), "already exists")
}
