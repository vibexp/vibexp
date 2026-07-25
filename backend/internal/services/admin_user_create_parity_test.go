package services

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/pkg/events"
)

// This file proves #462's headline guarantee: a user created by an admin is
// provisioned exactly like a self-signup user.
//
// It does so WITHOUT a database, and the reason it can is the point. All of the
// provisioning — the "Private Workspace" personal team, the owner membership,
// users.default_team_id, the default project — lives in ONE place,
// events.TeamCreationListener, and that listener is a pure function of the
// `user.created` payload. TeamService.CreateDefaultTeam has no other production
// caller. So if the admin path drives the real listener through the real event
// bus, provisioning is identical BY CONSTRUCTION rather than by coincidence, and
// the only thing worth asserting is that the chain actually runs.
//
// A database-backed test would re-verify the listener's own behaviour, which its
// own tests already cover; it would not add a guarantee this does not give.

// recordingTeamCreator captures what the listener asks for.
type recordingTeamCreator struct {
	mu       sync.Mutex
	userIDs  []string
	team     *models.Team
	failWith error
}

func (r *recordingTeamCreator) CreateDefaultTeam(_ context.Context, userID string) (*models.Team, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.userIDs = append(r.userIDs, userID)
	if r.failWith != nil {
		return nil, r.failWith
	}
	return r.team, nil
}

func (r *recordingTeamCreator) calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.userIDs...)
}

// recordingProjectCreator captures default-project creation.
type recordingProjectCreator struct {
	mu      sync.Mutex
	userIDs []string
	teamIDs []string
}

func (r *recordingProjectCreator) CreateProject(
	userID, teamID string, _ *models.CreateProjectRequest,
) (*models.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.userIDs = append(r.userIDs, userID)
	r.teamIDs = append(r.teamIDs, teamID)
	return &models.Project{ID: "project-1"}, nil
}

func (r *recordingProjectCreator) calls() ([]string, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.userIDs...), append([]string(nil), r.teamIDs...)
}

// startRealEventChain wires a REAL event manager with the REAL
// TeamCreationListener, so nothing about provisioning is stubbed except the two
// leaf services it calls.
func startRealEventChain(
	t *testing.T,
) (events.EventPublisher, *recordingTeamCreator, *recordingProjectCreator) {
	t.Helper()

	teamCreator := &recordingTeamCreator{team: &models.Team{ID: "team-1", IsPersonal: true, Name: "Private Workspace"}}
	projectCreator := &recordingProjectCreator{}

	manager := events.NewEventManager(events.EventBusConfig{
		Config: events.Config{WorkerCount: 1, BufferSize: 8},
		Logger: slog.New(slog.DiscardHandler),
	})
	require.NoError(t, manager.Subscribe(
		events.NewTeamCreationListener(teamCreator, projectCreator, slog.New(slog.DiscardHandler)),
	))
	require.NoError(t, manager.Start())
	t.Cleanup(func() {
		if err := manager.Stop(); err != nil {
			t.Logf("failed to stop event manager: %v", err)
		}
	})

	return manager, teamCreator, projectCreator
}

// eventually waits for an asynchronously-dispatched effect. Publish hands the
// event to a channel and returns, so the provisioning happens after CreateUser
// has already returned — the same window a self-signup has.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestCreateUser_DrivesTheRealProvisioningListener is the parity guarantee.
func TestCreateUser_DrivesTheRealProvisioningListener(t *testing.T) {
	publisher, teamCreator, projectCreator := startRealEventChain(t)

	adminRepo := repomocks.NewMockAdminRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	svc := NewAdminService(adminRepo, userRepo, publisher)

	userRepo.On("GetByEmail", mock.Anything, "new.user@example.com").
		Return(nil, repositories.ErrUserNotFound)
	userRepo.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*models.User).ID = createdUserID
	}).Return(nil)
	adminRepo.On("GetUserDetail", mock.Anything, createdUserID).Return(createdDetail(), nil)

	got, err := svc.CreateUser(context.Background(), AdminUserCreateRequest{
		Email: "new.user@example.com", Name: "New User",
	})
	require.NoError(t, err)
	require.NotNil(t, got)

	// The personal team is requested for exactly the user we created.
	eventually(t, "the default team to be created", func() bool {
		return len(teamCreator.calls()) == 1
	})
	assert.Equal(t, []string{createdUserID}, teamCreator.calls())

	// And the default project, in that team.
	eventually(t, "the default project to be created", func() bool {
		users, _ := projectCreator.calls()
		return len(users) == 1
	})
	users, teams := projectCreator.calls()
	assert.Equal(t, []string{createdUserID}, users)
	assert.Equal(t, []string{"team-1"}, teams,
		"the default project must land in the personal team the listener just created")
}

// TestCreateUser_EventPayloadMatchesWhatTheListenerReads guards the one way the
// chain can break silently: TeamCreationListener casts the payload to
// *events.UserCreatedPayload and, on a mismatch, logs and returns nil WITHOUT
// provisioning anything. A wrong payload type would therefore leave the user
// unprovisioned while every HTTP-level test still passed.
func TestCreateUser_EventPayloadMatchesWhatTheListenerReads(t *testing.T) {
	adminRepo := repomocks.NewMockAdminRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	pub := &stubPublisher{}
	svc := NewAdminService(adminRepo, userRepo, pub)

	userRepo.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound)
	userRepo.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*models.User).ID = createdUserID
	}).Return(nil)
	adminRepo.On("GetUserDetail", mock.Anything, createdUserID).Return(createdDetail(), nil)

	_, err := svc.CreateUser(context.Background(), AdminUserCreateRequest{
		Email: "new.user@example.com", Name: "New User",
	})
	require.NoError(t, err)

	require.Len(t, pub.published, 1)
	// Same event type the sign-up path publishes...
	assert.Equal(t, events.EventTypeUserCreated, pub.published[0].Type())
	// ...and the exact payload type the listener type-asserts.
	payload, ok := pub.published[0].Payload().(*events.UserCreatedPayload)
	require.True(t, ok,
		"the listener casts to *UserCreatedPayload and silently no-ops on anything else")
	assert.Equal(t, createdUserID, payload.UserID)
}

// TestCreateUser_SucceedsEvenIfProvisioningLater Fails documents the boundary of
// the guarantee: once the event is accepted, CreateUser is done. A listener that
// fails afterwards is logged by the listener (it deliberately does not retry), and
// is identical to what happens for a self-signup — so the admin path is no worse,
// and this test pins that equivalence rather than pretending otherwise.
func TestCreateUser_SucceedsEvenIfProvisioningLaterFails(t *testing.T) {
	publisher, teamCreator, _ := startRealEventChain(t)
	teamCreator.failWith = assert.AnError

	adminRepo := repomocks.NewMockAdminRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	svc := NewAdminService(adminRepo, userRepo, publisher)

	userRepo.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound)
	userRepo.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*models.User).ID = createdUserID
	}).Return(nil)
	adminRepo.On("GetUserDetail", mock.Anything, createdUserID).Return(createdDetail(), nil)

	got, err := svc.CreateUser(context.Background(), AdminUserCreateRequest{
		Email: "new.user@example.com", Name: "New User",
	})
	require.NoError(t, err, "the event was accepted; later listener failure is out of this call's scope")
	require.NotNil(t, got)

	eventually(t, "the listener to have attempted provisioning", func() bool {
		return len(teamCreator.calls()) == 1
	})
}
