package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	"github.com/vibexp/vibexp/pkg/events"
)

const (
	testUserID    = "11111111-1111-1111-1111-111111111111"
	testTeamID    = "22222222-2222-2222-2222-222222222222"
	testProjectID = "33333333-3333-3333-3333-333333333333"
	testKeyID     = "44444444-4444-4444-4444-444444444444"
	testRequestID = "req-123"
)

var errLookup = errors.New("lookup failed")

// syncSubmitter runs a task inline, so a test sees the persisted row as soon as
// Record returns.
type syncSubmitter struct{ submitted int }

func (s *syncSubmitter) Submit(task func()) {
	s.submitted++
	task()
}

// dropCounter records the reasons it was asked to count.
type dropCounter struct {
	mu      sync.Mutex
	reasons []string
}

func (d *dropCounter) RecordAuditEventDropped(_ context.Context, reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reasons = append(d.reasons, reason)
}

func (d *dropCounter) got() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.reasons...)
}

// captureFunc adapts a func to ClientInfoCapturer.
type captureFunc func(context.Context) bool

func (f captureFunc) CaptureClientInfo(ctx context.Context) bool { return f(ctx) }

// harness is a recorder over strict mocks and a synchronous submitter. A mock
// that receives a call no test arranged fails the test, which is how the
// "caller-provided names are not looked up" cases are proven.
type harness struct {
	recorder  *AsyncRecorder
	repo      *mocks.MockAuditEventRepository
	users     *mocks.MockUserRepository
	apiKeys   *mocks.MockAPIKeyRepository
	teams     *mocks.MockTeamRepository
	projects  *mocks.MockProjectRepository
	submitter *syncSubmitter
	drops     *dropCounter
	logs      *bytes.Buffer
	stored    []models.AuditEvent
}

func newHarness(t *testing.T, capture ClientInfoCapturer) *harness {
	t.Helper()
	h := &harness{
		repo:      mocks.NewMockAuditEventRepository(t),
		users:     mocks.NewMockUserRepository(t),
		apiKeys:   mocks.NewMockAPIKeyRepository(t),
		teams:     mocks.NewMockTeamRepository(t),
		projects:  mocks.NewMockProjectRepository(t),
		submitter: &syncSubmitter{},
		drops:     &dropCounter{},
		logs:      &bytes.Buffer{},
	}
	h.recorder = newRecorder(RecorderDeps{
		Repo:     h.repo,
		Users:    h.users,
		APIKeys:  h.apiKeys,
		Teams:    h.teams,
		Projects: h.projects,
		Capture:  capture,
		Drops:    h.drops,
		Logger:   slog.New(slog.NewTextHandler(h.logs, nil)),
	}, h.submitter)
	return h
}

// expectInsert stores every inserted row on the harness.
func (h *harness) expectInsert() {
	h.repo.EXPECT().Insert(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, e *models.AuditEvent) error {
			h.stored = append(h.stored, *e)
			return nil
		})
}

// expectUser answers the actor lookup.
func (h *harness) expectUser(email, name string) {
	h.users.EXPECT().GetByID(mock.Anything, testUserID).
		Return(&models.User{ID: testUserID, Email: email, Name: name}, nil)
}

// only returns the one stored row.
func (h *harness) only(t *testing.T) models.AuditEvent {
	t.Helper()
	require.Len(t, h.stored, 1)
	return h.stored[0]
}

// requestCtx is an HTTP request's context: it has a request id.
func requestCtx() context.Context {
	return context.WithValue(context.Background(), contextkeys.RequestID, testRequestID)
}

// userRequestCtx is a request authenticated as testUserID by authType.
func userRequestCtx(authType string) context.Context {
	ctx := context.WithValue(requestCtx(), contextkeys.UserID, testUserID)
	return context.WithValue(ctx, contextkeys.AuthType, authType)
}

func ptr(s string) *string { return &s }

func TestRecord_SourceFromWriteOrigin(t *testing.T) {
	cases := []struct {
		name          string
		source        string
		assistant     string
		wantSource    models.AuditSource
		wantAssistant *string
	}{
		{"web", resourceaccess.SourceWeb, "", models.AuditSourceWeb, nil},
		{"cli", resourceaccess.SourceCLI, "", models.AuditSourceCLI, nil},
		{"mcp with the client's name", resourceaccess.SourceMCP, "  Claude Code  ", models.AuditSourceMCP, ptr("Claude Code")},
		{"api", resourceaccess.SourceAPI, "", models.AuditSourceAPI, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, nil)
			h.expectUser("ada@example.com", "Ada")
			h.expectInsert()

			// The origin wins over what the auth type alone would say.
			ctx := resourceaccess.WithWriteOrigin(userRequestCtx("cookie"), tc.source, tc.assistant)
			h.recorder.Record(ctx, Event{Operation: models.AuditOpPromptUpdate})

			got := h.only(t)
			assert.Equal(t, tc.wantSource, got.Source)
			assert.Equal(t, tc.wantAssistant, got.AssistantName)
		})
	}
}

func TestRecord_SourceFallback(t *testing.T) {
	cases := []struct {
		name       string
		ctx        func() context.Context
		wantSource models.AuditSource
		wantActor  models.AuditActorType
	}{
		{"cookie is web", func() context.Context { return userRequestCtx("cookie") },
			models.AuditSourceWeb, models.AuditActorUser},
		{"an API key is api", func() context.Context { return userRequestCtx("api_key") },
			models.AuditSourceAPI, models.AuditActorUser},
		{"an API key from the CLI is cli", func() context.Context {
			return context.WithValue(userRequestCtx("api_key"), contextkeys.UserAgent, "VibeXP-CLI/0.9.0")
		}, models.AuditSourceCLI, models.AuditActorUser},
		{"an unauthenticated request is web and anonymous", requestCtx,
			models.AuditSourceWeb, models.AuditActorAnonymous},
		{"no request is system", context.Background,
			models.AuditSourceSystem, models.AuditActorSystem},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, nil)
			if tc.wantActor == models.AuditActorUser {
				h.expectUser("ada@example.com", "Ada")
			}
			h.expectInsert()

			h.recorder.Record(tc.ctx(), Event{Operation: models.AuditOpAuthLogin})

			got := h.only(t)
			assert.Equal(t, tc.wantSource, got.Source)
			assert.Equal(t, tc.wantActor, got.ActorType)
			assert.Nil(t, got.AssistantName)
		})
	}
}

func TestRecord_ExplicitSourceWins(t *testing.T) {
	h := newHarness(t, nil)
	h.expectInsert()

	h.recorder.Record(requestCtx(), Event{Operation: models.AuditOpAuthLogin, Source: models.AuditSourceSystem})

	assert.Equal(t, models.AuditSourceSystem, h.only(t).Source)
}

func TestRecord_ContextFields(t *testing.T) {
	h := newHarness(t, nil)
	h.expectUser("ada@example.com", "Ada Lovelace")
	h.apiKeys.EXPECT().GetNamesByIDs(mock.Anything, testUserID, []string{testKeyID}).
		Return(map[string]string{testKeyID: "laptop key"}, nil)
	h.expectInsert()

	ctx := userRequestCtx("api_key")
	ctx = context.WithValue(ctx, contextkeys.APIKeyID, testKeyID)
	ctx = context.WithValue(ctx, contextkeys.ClientIP, "203.0.113.7")
	ctx = context.WithValue(ctx, contextkeys.UserAgent, "curl/8.0")

	h.recorder.Record(ctx, Event{
		Operation:    models.AuditOpArtifactDelete,
		ResourceType: "artifact",
		ResourceID:   "res-1",
		ResourceName: "Release notes",
	})

	got := h.only(t)
	assert.Equal(t, models.AuditOpArtifactDelete, got.Operation)
	assert.Equal(t, models.AuditOutcomeSuccess, got.Outcome, "the outcome defaults to success")
	assert.Equal(t, ptr(testUserID), got.ActorUserID)
	assert.Equal(t, "ada@example.com", got.ActorEmail, "the email is looked up")
	assert.Equal(t, "Ada Lovelace", got.ActorName, "the name is looked up")
	assert.Equal(t, models.AuditActorUser, got.ActorType)
	assert.Equal(t, ptr(testKeyID), got.APIKeyID)
	assert.Equal(t, ptr("laptop key"), got.APIKeyName)
	assert.Equal(t, ptr("203.0.113.7"), got.IP)
	assert.Equal(t, ptr("curl/8.0"), got.UserAgent)
	assert.Equal(t, ptr(testRequestID), got.RequestID)
	assert.Equal(t, ptr("artifact"), got.ResourceType)
	assert.Equal(t, ptr("res-1"), got.ResourceID)
	assert.Equal(t, ptr("Release notes"), got.ResourceName)
	assert.Nil(t, got.TeamID)
	assert.Nil(t, got.ProjectID)
	assert.JSONEq(t, `{}`, string(got.Metadata))
	assert.Empty(t, got.ID, "the database assigns the id")
	assert.True(t, got.CreatedAt.IsZero(), "the database assigns the time")
	assert.Empty(t, h.drops.got())
}

func TestRecord_EmailFromContextIsKept(t *testing.T) {
	h := newHarness(t, nil)
	h.expectUser("stored@example.com", "Ada")
	h.expectInsert()

	ctx := context.WithValue(userRequestCtx("cookie"), contextkeys.UserEmail, "ctx@example.com")
	h.recorder.Record(ctx, Event{Operation: models.AuditOpAuthLogout})

	got := h.only(t)
	assert.Equal(t, "ctx@example.com", got.ActorEmail)
	assert.Equal(t, "Ada", got.ActorName)
}

func TestRecord_UnknownAPIKeyStoresNoName(t *testing.T) {
	h := newHarness(t, nil)
	h.expectUser("ada@example.com", "Ada")
	h.apiKeys.EXPECT().GetNamesByIDs(mock.Anything, testUserID, []string{testKeyID}).
		Return(map[string]string{}, nil)
	h.expectInsert()

	ctx := context.WithValue(userRequestCtx("api_key"), contextkeys.APIKeyID, testKeyID)
	h.recorder.Record(ctx, Event{Operation: models.AuditOpAuthAPIKeyRevoked})

	got := h.only(t)
	assert.Equal(t, ptr(testKeyID), got.APIKeyID)
	assert.Nil(t, got.APIKeyName)
	assert.Empty(t, h.drops.got(), "a key that no longer exists is not a failure")
}

func TestRecord_ExplicitActorOverridesContext(t *testing.T) {
	t.Run("a failed sign-in for an unknown account", func(t *testing.T) {
		h := newHarness(t, nil)
		h.expectInsert()

		attempted := strings.Repeat("a", 400) + "@example.com"
		h.recorder.Record(requestCtx(), Event{
			Operation: models.AuditOpAuthLoginFailed,
			Outcome:   models.AuditOutcomeFailure,
			Actor:     &Actor{Email: attempted},
		})

		got := h.only(t)
		assert.Nil(t, got.ActorUserID)
		assert.Equal(t, models.AuditActorAnonymous, got.ActorType)
		assert.Equal(t, models.AuditOutcomeFailure, got.Outcome)
		assert.Len(t, got.ActorEmail, maxActorEmailBytes, "the attempted email is capped")
	})

	t.Run("a named actor replaces the context user and is not looked up", func(t *testing.T) {
		h := newHarness(t, nil)
		h.expectInsert()

		other := "55555555-5555-5555-5555-555555555555"
		h.recorder.Record(userRequestCtx("cookie"), Event{
			Operation: models.AuditOpTeamMemberRemoved,
			Actor:     &Actor{UserID: &other, Email: "grace@example.com", Name: "Grace"},
		})

		got := h.only(t)
		assert.Equal(t, ptr(other), got.ActorUserID)
		assert.Equal(t, "grace@example.com", got.ActorEmail)
		assert.Equal(t, "Grace", got.ActorName)
		assert.Equal(t, models.AuditActorUser, got.ActorType)
	})

	t.Run("a system actor outside a request", func(t *testing.T) {
		h := newHarness(t, nil)
		h.expectInsert()

		empty := ""
		h.recorder.Record(context.Background(), Event{
			Operation: models.AuditOpAuditSettingsUpdated,
			Actor:     &Actor{UserID: &empty, Name: "retention sweep"},
		})

		got := h.only(t)
		assert.Nil(t, got.ActorUserID, "an empty user id is no user id")
		assert.Equal(t, models.AuditActorSystem, got.ActorType)
		assert.Equal(t, "retention sweep", got.ActorName)
	})

	t.Run("an explicit type is kept", func(t *testing.T) {
		h := newHarness(t, nil)
		h.expectInsert()

		h.recorder.Record(requestCtx(), Event{
			Operation: models.AuditOpAuthMCPClientRegistered,
			Actor:     &Actor{Type: models.AuditActorSystem},
		})

		assert.Equal(t, models.AuditActorSystem, h.only(t).ActorType)
	})
}

func TestRecord_TeamAndProjectNames(t *testing.T) {
	t.Run("missing names are looked up", func(t *testing.T) {
		h := newHarness(t, nil)
		h.expectUser("ada@example.com", "Ada")
		h.teams.EXPECT().GetNamesByIDs(mock.Anything, []string{testTeamID}).
			Return(map[string]string{testTeamID: "Platform"}, nil)
		h.projects.EXPECT().GetNameByIDForAudit(mock.Anything, testProjectID).Return("vibexp", nil)
		h.expectInsert()

		h.recorder.Record(userRequestCtx("cookie"), Event{
			Operation: models.AuditOpPromptCreate,
			TeamID:    testTeamID,
			ProjectID: testProjectID,
		})

		got := h.only(t)
		assert.Equal(t, ptr(testTeamID), got.TeamID)
		assert.Equal(t, ptr("Platform"), got.TeamName)
		assert.Equal(t, ptr(testProjectID), got.ProjectID)
		assert.Equal(t, ptr("vibexp"), got.ProjectName)
	})

	t.Run("caller-provided names win and are not looked up", func(t *testing.T) {
		h := newHarness(t, nil)
		h.expectUser("ada@example.com", "Ada")
		h.expectInsert()

		h.recorder.Record(userRequestCtx("cookie"), Event{
			Operation:   models.AuditOpProjectDelete,
			TeamID:      testTeamID,
			TeamName:    "Given team",
			ProjectID:   testProjectID,
			ProjectName: "Given project",
		})

		got := h.only(t)
		assert.Equal(t, ptr("Given team"), got.TeamName)
		assert.Equal(t, ptr("Given project"), got.ProjectName)
	})

	t.Run("the project lookup does not need a member actor", func(t *testing.T) {
		h := newHarness(t, nil)
		h.projects.EXPECT().GetNameByIDForAudit(mock.Anything, testProjectID).Return("vibexp", nil)
		h.expectInsert()

		h.recorder.Record(context.Background(), Event{
			Operation: models.AuditOpProjectMigrated,
			ProjectID: testProjectID,
		})

		assert.Equal(t, ptr("vibexp"), h.only(t).ProjectName)
	})

	t.Run("a deleted team or project stores no name", func(t *testing.T) {
		h := newHarness(t, nil)
		h.teams.EXPECT().GetNamesByIDs(mock.Anything, []string{testTeamID}).Return(map[string]string{}, nil)
		h.projects.EXPECT().GetNameByIDForAudit(mock.Anything, testProjectID).Return("", nil)
		h.expectInsert()

		h.recorder.Record(context.Background(), Event{
			Operation: models.AuditOpTeamDeleted,
			TeamID:    testTeamID,
			ProjectID: testProjectID,
		})

		got := h.only(t)
		assert.Nil(t, got.TeamName)
		assert.Nil(t, got.ProjectName)
		assert.Empty(t, h.drops.got())
	})
}

func TestRecord_CaptureOffStoresNoClientInfo(t *testing.T) {
	var asked context.Context
	h := newHarness(t, captureFunc(func(ctx context.Context) bool {
		asked = ctx
		return false
	}))
	h.expectInsert()

	ctx := context.WithValue(requestCtx(), contextkeys.ClientIP, "203.0.113.7")
	ctx = context.WithValue(ctx, contextkeys.UserAgent, "curl/8.0")
	h.recorder.Record(ctx, Event{Operation: models.AuditOpAuthLoginFailed, Outcome: models.AuditOutcomeFailure})

	got := h.only(t)
	assert.Nil(t, got.IP)
	assert.Nil(t, got.UserAgent)
	assert.Equal(t, ptr(testRequestID), got.RequestID, "the request id is not client info")
	require.NotNil(t, asked)
	_, hasDeadline := asked.Deadline()
	assert.True(t, hasDeadline, "the setting is read on the task's time-bounded context")
}

func TestRecord_FieldCaps(t *testing.T) {
	h := newHarness(t, nil)
	h.expectInsert()

	ctx := context.WithValue(context.Background(), contextkeys.RequestID, strings.Repeat("r", 4096))
	ctx = context.WithValue(ctx, contextkeys.UserAgent, strings.Repeat("u", MaxUserAgentBytes+10))
	h.recorder.Record(ctx, Event{
		Operation:    models.AuditOpMemoryDelete,
		ResourceName: "x" + strings.Repeat("é", maxResourceNameBytes),
	})

	got := h.only(t)
	require.NotNil(t, got.RequestID)
	assert.Len(t, *got.RequestID, maxRequestIDBytes, "a client-sent X-Request-ID is capped")
	require.NotNil(t, got.UserAgent)
	assert.Len(t, *got.UserAgent, MaxUserAgentBytes)
	require.NotNil(t, got.ResourceName)
	assert.True(t, utf8.ValidString(*got.ResourceName), "the cut never splits a rune")
	assert.Equal(t, maxResourceNameBytes-1, len(*got.ResourceName))
}

func TestRecord_Metadata(t *testing.T) {
	t.Run("is serialized when Record is called", func(t *testing.T) {
		h := newHarness(t, nil)
		h.expectInsert()

		meta := map[string]any{"role": map[string]any{"old": "member", "new": "admin"}}
		h.recorder.Record(context.Background(), Event{Operation: models.AuditOpTeamMemberRoleChanged, Metadata: meta})
		meta["role"] = "mutated afterwards"

		assert.JSONEq(t, `{"role":{"old":"member","new":"admin"}}`, string(h.only(t).Metadata))
	})

	t.Run("an oversized document is replaced by a marker", func(t *testing.T) {
		h := newHarness(t, nil)
		h.expectInsert()

		h.recorder.Record(context.Background(), Event{
			Operation: models.AuditOpInstanceSettingsUpdated,
			Metadata:  map[string]any{"blob": strings.Repeat("z", maxMetadataBytes)},
		})

		assert.JSONEq(t, `{"truncated":true}`, string(h.only(t).Metadata))
		assert.Empty(t, h.drops.got(), "truncation is by design, not a failure")
	})

	t.Run("an unserializable document is stored as a marker and counted", func(t *testing.T) {
		h := newHarness(t, nil)
		h.expectInsert()

		h.recorder.Record(context.Background(), Event{
			Operation: models.AuditOpInstanceSettingsUpdated,
			Metadata:  map[string]any{"ch": make(chan int)},
		})

		assert.JSONEq(t, `{"unserializable":true}`, string(h.only(t).Metadata))
		assert.Equal(t, []string{DropReasonMetadata}, h.drops.got())
	})
}

func TestRecord_InvalidEventIsDroppedAndCounted(t *testing.T) {
	cases := map[string]Event{
		"operation": {Operation: "prompt.read"},
		"outcome":   {Operation: models.AuditOpPromptCreate, Outcome: "error"},
		"source":    {Operation: models.AuditOpPromptCreate, Source: "carrier-pigeon"},
		"actor":     {Operation: models.AuditOpPromptCreate, Actor: &Actor{Type: "robot"}},
	}
	for name, e := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, nil)

			assert.NotPanics(t, func() { h.recorder.Record(context.Background(), e) })

			assert.Zero(t, h.submitter.submitted, "nothing is queued")
			assert.Equal(t, []string{DropReasonInvalid}, h.drops.got())
			assert.Contains(t, h.logs.String(), "outside the vocabulary")
		})
	}
}

func TestRecord_FailuresAreLoggedCountedAndNeverReachTheCaller(t *testing.T) {
	t.Run("a repository error drops the event", func(t *testing.T) {
		h := newHarness(t, nil)
		h.repo.EXPECT().Insert(mock.Anything, mock.Anything).Return(errors.New("db down"))

		assert.NotPanics(t, func() {
			h.recorder.Record(context.Background(), Event{Operation: models.AuditOpAuditExport})
		})

		assert.Equal(t, []string{DropReasonPersist}, h.drops.got())
		assert.Contains(t, h.logs.String(), "failed to record audit event")
		assert.Contains(t, h.logs.String(), "db down")
	})

	t.Run("lookup errors keep the event and are counted once", func(t *testing.T) {
		h := newHarness(t, nil)
		h.users.EXPECT().GetByID(mock.Anything, testUserID).Return(nil, errLookup)
		h.apiKeys.EXPECT().GetNamesByIDs(mock.Anything, testUserID, []string{testKeyID}).Return(nil, errLookup)
		h.teams.EXPECT().GetNamesByIDs(mock.Anything, []string{testTeamID}).Return(nil, errLookup)
		h.projects.EXPECT().GetNameByIDForAudit(mock.Anything, testProjectID).Return("", errLookup)
		h.expectInsert()

		ctx := context.WithValue(userRequestCtx("api_key"), contextkeys.APIKeyID, testKeyID)
		h.recorder.Record(ctx, Event{Operation: models.AuditOpPromptUpdate, TeamID: testTeamID, ProjectID: testProjectID})

		got := h.only(t)
		assert.Equal(t, ptr(testUserID), got.ActorUserID)
		assert.Empty(t, got.ActorName)
		assert.Nil(t, got.APIKeyName)
		assert.Nil(t, got.TeamName)
		assert.Nil(t, got.ProjectName)
		assert.Equal(t, []string{DropReasonLookup}, h.drops.got())
		for _, what := range []string{"lookup=actor", "lookup=api_key", "lookup=team", "lookup=project"} {
			assert.Contains(t, h.logs.String(), what)
		}
	})

	t.Run("a user that no longer exists is not a failure", func(t *testing.T) {
		h := newHarness(t, nil)
		h.users.EXPECT().GetByID(mock.Anything, testUserID).Return(nil, repositories.ErrUserNotFound)
		h.expectInsert()

		h.recorder.Record(userRequestCtx("cookie"), Event{Operation: models.AuditOpAuthLogout})

		assert.Empty(t, h.only(t).ActorName)
		assert.Empty(t, h.drops.got())
	})

	t.Run("a panic in the task drops the event", func(t *testing.T) {
		h := newHarness(t, nil)
		h.repo.EXPECT().Insert(mock.Anything, mock.Anything).
			RunAndReturn(func(context.Context, *models.AuditEvent) error { panic("boom") })

		assert.NotPanics(t, func() {
			h.recorder.Record(context.Background(), Event{Operation: models.AuditOpAuthzDenied})
		})

		assert.Equal(t, []string{DropReasonPanic}, h.drops.got())
		assert.Contains(t, h.logs.String(), "boom")
	})

	t.Run("a panic while capturing never reaches the caller", func(t *testing.T) {
		h := newHarness(t, nil)

		assert.NotPanics(t, func() {
			h.recorder.Record(panickingContext{}, Event{Operation: models.AuditOpAuthzDenied})
		})

		assert.Zero(t, h.submitter.submitted)
		assert.Equal(t, []string{DropReasonPanic}, h.drops.got())
	})

	t.Run("no drop counter is fine", func(t *testing.T) {
		repo := mocks.NewMockAuditEventRepository(t)
		repo.EXPECT().Insert(mock.Anything, mock.Anything).Return(errors.New("db down"))
		r := newRecorder(RecorderDeps{Repo: repo, Logger: slog.New(slog.DiscardHandler)}, &syncSubmitter{})

		assert.NotPanics(t, func() {
			r.Record(context.Background(), Event{Operation: models.AuditOpAuditExport})
		})
	})
}

// panickingContext panics on every value lookup, standing in for any failure
// on the caller's goroutine.
type panickingContext struct{ context.Context }

func (panickingContext) Value(any) any { panic("context exploded") }

// TestNewRecorder_PersistsOnTheWorkerPool runs the real pool: Record returns
// before the row is written, and the row arrives from another goroutine with
// the defaults (capture on) applied.
func TestNewRecorder_PersistsOnTheWorkerPool(t *testing.T) {
	pool := events.NewWorkerPool(1)
	t.Cleanup(pool.Stop)

	inserted := make(chan models.AuditEvent, 1)
	repo := mocks.NewMockAuditEventRepository(t)
	repo.EXPECT().Insert(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, e *models.AuditEvent) error {
			_, hasDeadline := ctx.Deadline()
			assert.True(t, hasDeadline, "the insert runs on a time-bounded context")
			inserted <- *e
			return nil
		})

	r := NewRecorder(RecorderDeps{Repo: repo, Pool: pool})
	ctx := context.WithValue(requestCtx(), contextkeys.ClientIP, "198.51.100.1")
	ctx, cancel := context.WithCancel(ctx)
	r.Record(ctx, Event{Operation: models.AuditOpAuthLoginFailed, Outcome: models.AuditOutcomeFailure})
	cancel() // the request ending must not cancel the write

	select {
	case got := <-inserted:
		assert.Equal(t, ptr("198.51.100.1"), got.IP, "capture defaults to on")
		assert.Equal(t, models.AuditOpAuthLoginFailed, got.Operation)
		raw, err := json.Marshal(got.Metadata)
		require.NoError(t, err)
		assert.JSONEq(t, `{}`, string(raw))
	case <-time.After(5 * time.Second):
		t.Fatal("the event was never persisted")
	}
}

func TestAlwaysCapture(t *testing.T) {
	assert.True(t, AlwaysCapture{}.CaptureClientInfo(context.Background()))
}

func TestTruncateBytes(t *testing.T) {
	assert.Equal(t, "short", TruncateBytes("short", 10))
	assert.Equal(t, "abc", TruncateBytes("abcdef", 3))
	assert.Empty(t, TruncateBytes("é", 1), "a lone continuation byte backs off to nothing")
}

// TestRecord_NeverBlocksTheCaller pins the Recorder contract on the real
// worker pool: Record returns at once even when every worker is busy (the
// pool is non-blocking and falls back to a dedicated goroutine), and when the
// capture setting or the insert would block, because both run in persist,
// off the caller's goroutine.
func TestRecord_NeverBlocksTheCaller(t *testing.T) {
	const prompt = 2 * time.Second

	returnsPromptly := func(t *testing.T, r *AsyncRecorder) {
		t.Helper()
		done := make(chan struct{})
		go func() {
			r.Record(requestCtx(), Event{Operation: models.AuditOpAuthLogin})
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(prompt):
			t.Fatal("Record blocked its caller")
		}
	}

	t.Run("every worker busy", func(t *testing.T) {
		// Cleanups run last-in first-out: release the blocked task before the
		// pool waits for it to finish.
		pool := events.NewWorkerPool(1)
		t.Cleanup(pool.Stop)
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		pool.Submit(func() { <-release })

		inserted := make(chan struct{}, 1)
		repo := mocks.NewMockAuditEventRepository(t)
		repo.EXPECT().Insert(mock.Anything, mock.Anything).
			RunAndReturn(func(context.Context, *models.AuditEvent) error { inserted <- struct{}{}; return nil })
		r := NewRecorder(RecorderDeps{Repo: repo, Pool: pool, Logger: slog.New(slog.DiscardHandler)})

		returnsPromptly(t, r)
		select {
		case <-inserted:
		case <-time.After(prompt):
			t.Fatal("a saturated pool must still run the task")
		}
	})

	t.Run("the capture setting blocks", func(t *testing.T) {
		// Cleanups run last-in first-out: release the blocked task before the
		// pool waits for it to finish.
		pool := events.NewWorkerPool(1)
		t.Cleanup(pool.Stop)
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		repo := mocks.NewMockAuditEventRepository(t)
		repo.EXPECT().Insert(mock.Anything, mock.Anything).Return(nil).Maybe()
		r := NewRecorder(RecorderDeps{
			Repo: repo, Pool: pool, Logger: slog.New(slog.DiscardHandler),
			Capture: captureFunc(func(context.Context) bool { <-release; return true }),
		})

		returnsPromptly(t, r)
	})

	t.Run("the insert blocks", func(t *testing.T) {
		// Cleanups run last-in first-out: release the blocked task before the
		// pool waits for it to finish.
		pool := events.NewWorkerPool(1)
		t.Cleanup(pool.Stop)
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		repo := mocks.NewMockAuditEventRepository(t)
		repo.EXPECT().Insert(mock.Anything, mock.Anything).
			RunAndReturn(func(context.Context, *models.AuditEvent) error { <-release; return nil }).Maybe()
		r := NewRecorder(RecorderDeps{Repo: repo, Pool: pool, Logger: slog.New(slog.DiscardHandler)})

		returnsPromptly(t, r)
	})
}

// TestNewSyncRecorder_PersistsBeforeRecordReturns: the CLI's recorder writes
// the row on the caller's goroutine, so the row exists the moment Record
// returns, on a time-bounded context of its own.
func TestNewSyncRecorder_PersistsBeforeRecordReturns(t *testing.T) {
	var stored []models.AuditEvent
	repo := mocks.NewMockAuditEventRepository(t)
	repo.EXPECT().Insert(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, e *models.AuditEvent) error {
			_, hasDeadline := ctx.Deadline()
			assert.True(t, hasDeadline, "the insert runs on a time-bounded context")
			stored = append(stored, *e)
			return nil
		})

	NewSyncRecorder(RecorderDeps{Repo: repo}).Record(context.Background(), Event{
		Operation: models.AuditOpInstanceSettingsUpdated, Source: models.AuditSourceCLI,
	})

	require.Len(t, stored, 1)
	assert.Equal(t, models.AuditSourceCLI, stored[0].Source)
	assert.Equal(t, models.AuditActorSystem, stored[0].ActorType)
}

// TestNewSyncRecorder_FailureIsLoggedNotReturned: a failed insert is logged
// and counted, and Record still returns normally.
func TestNewSyncRecorder_FailureIsLoggedNotReturned(t *testing.T) {
	repo := mocks.NewMockAuditEventRepository(t)
	repo.EXPECT().Insert(mock.Anything, mock.Anything).Return(errors.New("db down"))
	logs := &bytes.Buffer{}
	drops := &dropCounter{}

	assert.NotPanics(t, func() {
		NewSyncRecorder(RecorderDeps{Repo: repo, Drops: drops, Logger: slog.New(slog.NewTextHandler(logs, nil))}).
			Record(context.Background(), Event{Operation: models.AuditOpInstanceSettingsUpdated})
	})

	assert.Equal(t, []string{DropReasonPersist}, drops.got())
	assert.Contains(t, logs.String(), "failed to record audit event")
}
