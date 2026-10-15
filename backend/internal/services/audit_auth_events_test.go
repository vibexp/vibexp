package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	repo_mocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// Auth & security events recorded by the services (#1514).

type auditEvents struct {
	mu  sync.Mutex
	got []audit.Event
}

func (a *auditEvents) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.got = append(a.got, e)
}

func (a *auditEvents) only(t *testing.T) audit.Event {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	require.Len(t, a.got, 1, "exactly one audit event")
	return a.got[0]
}

func TestAuthService_RevokeUserSessions_Audits(t *testing.T) {
	users := repo_mocks.NewMockUserRepository(t)
	users.On("BumpSessionEpoch", context.Background(), "user-1").Return(int64(4), nil)
	logger, _ := logtest.New()
	events := &auditEvents{}
	svc := NewAuthService(users, nil, staticIDPResolver{}, nil, logger, nil, WithAuditRecorder(events))

	epoch, err := svc.RevokeUserSessions(context.Background(), "user-1")

	require.NoError(t, err)
	assert.EqualValues(t, 4, epoch)
	e := events.only(t)
	assert.Equal(t, models.AuditOpAuthSessionsRevoked, e.Operation)
	assert.Equal(t, auditResourceTypeUser, e.ResourceType)
	assert.Equal(t, "user-1", e.ResourceID)
	assert.EqualValues(t, 4, e.Metadata["session_epoch"])
	assert.Nil(t, e.Actor, "the actor is the caller from the context")
}

func TestAuthService_RevokeUserSessions_FailureRecordsNothing(t *testing.T) {
	users := repo_mocks.NewMockUserRepository(t)
	users.On("BumpSessionEpoch", context.Background(), "user-1").Return(int64(0), assert.AnError)
	logger, _ := logtest.New()
	events := &auditEvents{}
	svc := NewAuthService(users, nil, staticIDPResolver{}, nil, logger, nil, WithAuditRecorder(events))

	_, err := svc.RevokeUserSessions(context.Background(), "user-1")

	require.Error(t, err)
	assert.Empty(t, events.got)
}

func TestAuthService_LinkByVerifiedEmail_Audits(t *testing.T) {
	verifiedAt := time.Now().Add(-time.Hour)
	account := &models.User{
		ID: "user-1", Email: "ada@example.com", Name: "Ada", EmailVerifiedAt: &verifiedAt,
		IDPProvider: strPtr("google"), IDPSubject: strPtr("g-1"),
	}
	f := newLinkFixture(t, account, nil, nil).holdsIdentity("google", "g-1")
	events := &auditEvents{}
	f.svc.audit = events

	_, _, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: true})

	require.NoError(t, err)
	e := events.only(t)
	assert.Equal(t, models.AuditOpAuthIdentityLinked, e.Operation)
	require.NotNil(t, e.Actor)
	require.NotNil(t, e.Actor.UserID)
	assert.Equal(t, "user-1", *e.Actor.UserID, "the sign-in route is public, so the account is named")
	assert.Equal(t, "ada@example.com", e.Actor.Email)
	assert.Equal(t, models.AuditSourceWeb, e.Source)
	assert.Equal(t, "user-1", e.ResourceID)
	assert.Equal(t, map[string]any{"provider": "corp-sso"}, e.Metadata)
}

func TestAuthService_LinkRefused_RecordsNothing(t *testing.T) {
	account := &models.User{ID: "user-1", Email: "ada@example.com", Name: "Ada"}
	f := newLinkFixture(t, account, nil, nil)
	events := &auditEvents{}
	f.svc.audit = events

	_, _, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: false})

	require.ErrorIs(t, err, ErrEmailNotVerified)
	assert.Empty(t, events.got)
}

func TestAPIAccessService_RevokeAPIAccess_Audits(t *testing.T) {
	ctx := context.Background()
	repo := repo_mocks.NewMockAPIAccessRepository(t)
	repo.On("RevokeAll", ctx, "user-1").
		Return(&models.APIAccessRevocation{TokensValidAfter: time.Now(), APIKeys: 2, OAuthGrants: 5}, nil)
	logger, _ := logtest.New()
	events := &auditEvents{}

	_, err := NewAPIAccessService(repo, logger, events).RevokeAPIAccess(ctx, "user-1")

	require.NoError(t, err)
	e := events.only(t)
	assert.Equal(t, models.AuditOpAuthAPIAccessRevoked, e.Operation)
	assert.Equal(t, "user-1", e.ResourceID)
	assert.EqualValues(t, 2, e.Metadata["api_keys"])
	assert.EqualValues(t, 5, e.Metadata["oauth_grants"])
}

func TestAPIAccessService_RevokeAPIAccess_FailureRecordsNothing(t *testing.T) {
	ctx := context.Background()
	repo := repo_mocks.NewMockAPIAccessRepository(t)
	repo.On("RevokeAll", ctx, "user-1").Return(nil, assert.AnError)
	logger, _ := logtest.New()
	events := &auditEvents{}

	_, err := NewAPIAccessService(repo, logger, events).RevokeAPIAccess(ctx, "user-1")

	require.Error(t, err)
	assert.Empty(t, events.got)
}
