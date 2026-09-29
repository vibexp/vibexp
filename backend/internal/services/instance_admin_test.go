package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// fakeInstanceAdminGrants is an in-memory InstanceAdminRepository. It records
// the actor of every write so tests can pin who the audit entry names.
type fakeInstanceAdminGrants struct {
	repositories.InstanceAdminRepository
	granted  map[string]bool
	actors   []string
	err      error
	writeErr error
}

func newFakeInstanceAdminGrants(userIDs ...string) *fakeInstanceAdminGrants {
	f := &fakeInstanceAdminGrants{granted: map[string]bool{}}
	for _, id := range userIDs {
		f.granted[id] = true
	}
	return f
}

func (f *fakeInstanceAdminGrants) IsGranted(_ context.Context, userID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.granted[userID], nil
}

func (f *fakeInstanceAdminGrants) Grant(_ context.Context, userID string, grantedBy *string) (bool, error) {
	if f.writeErr != nil {
		return false, f.writeErr
	}
	if f.granted[userID] {
		return false, nil
	}
	f.granted[userID] = true
	f.actors = append(f.actors, *grantedBy)
	return true, nil
}

func (f *fakeInstanceAdminGrants) Revoke(_ context.Context, userID string, actorUserID *string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if !f.granted[userID] {
		return repositories.ErrInstanceAdminNotFound
	}
	delete(f.granted, userID)
	f.actors = append(f.actors, *actorUserID)
	return nil
}

// fakeInstanceAdminUsers is an in-memory UserRepository keyed by id.
type fakeInstanceAdminUsers struct {
	repositories.UserRepository
	users map[string]*models.User
	err   error
}

func (f *fakeInstanceAdminUsers) GetByID(_ context.Context, userID string) (*models.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	u, ok := f.users[userID]
	if !ok {
		return nil, repositories.ErrUserNotFound
	}
	return u, nil
}

const (
	iaRootID      = "root-id"
	iaDBAdminID   = "db-admin-id"
	iaPlainID     = "plain-id"
	iaSuspendedID = "suspended-id"
	iaRoot2ID     = "root2-id"
)

func newInstanceAdminFixture(grants *fakeInstanceAdminGrants) (*InstanceAdminService, *fakeInstanceAdminUsers) {
	users := &fakeInstanceAdminUsers{users: map[string]*models.User{
		iaRootID:      {ID: iaRootID, Email: "Root@Example.com", Status: models.UserStatusActive},
		iaRoot2ID:     {ID: iaRoot2ID, Email: "root2@example.com", Status: models.UserStatusActive},
		iaDBAdminID:   {ID: iaDBAdminID, Email: "delegate@example.com", Status: models.UserStatusActive},
		iaPlainID:     {ID: iaPlainID, Email: "plain@example.com", Status: models.UserStatusActive},
		iaSuspendedID: {ID: iaSuspendedID, Email: "gone@example.com", Status: models.UserStatusSuspended},
	}}
	svc := NewInstanceAdminService(
		[]string{"  root@example.com ", "", "   ", "ROOT2@example.com"},
		grants, users, slog.New(slog.DiscardHandler),
	)
	return svc, users
}

func TestInstanceAdminService_IsRootAdmin(t *testing.T) {
	svc, _ := newInstanceAdminFixture(newFakeInstanceAdminGrants())

	tests := []struct {
		email string
		want  bool
	}{
		{"root@example.com", true},
		{"ROOT@EXAMPLE.COM", true},
		{"  root@example.com  ", true},
		{"root2@example.com", true},
		{"delegate@example.com", false},
		{"", false},
		{"   ", false},
	}
	for _, tc := range tests {
		t.Run(tc.email, func(t *testing.T) {
			assert.Equal(t, tc.want, svc.IsRootAdmin(tc.email))
		})
	}

	empty := NewInstanceAdminService(nil, newFakeInstanceAdminGrants(), &fakeInstanceAdminUsers{},
		slog.New(slog.DiscardHandler))
	assert.False(t, empty.IsRootAdmin("root@example.com"), "an empty list has no root admin")
}

func TestInstanceAdminService_IsInstanceAdmin(t *testing.T) {
	grants := newFakeInstanceAdminGrants(iaDBAdminID, iaSuspendedID)
	svc, users := newInstanceAdminFixture(grants)

	tests := []struct {
		name   string
		userID string
		want   bool
	}{
		{"root admin", iaRootID, true},
		{"active DB-granted admin", iaDBAdminID, true},
		{"suspended DB-granted admin", iaSuspendedID, false},
		{"plain user", iaPlainID, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.IsInstanceAdmin(context.Background(), users.users[tc.userID])
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("nil user", func(t *testing.T) {
		got, err := svc.IsInstanceAdmin(context.Background(), nil)
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("grant lookup error is returned", func(t *testing.T) {
		grants.err = errors.New("db down")
		defer func() { grants.err = nil }()
		got, err := svc.IsInstanceAdmin(context.Background(), users.users[iaDBAdminID])
		require.Error(t, err)
		assert.False(t, got)
	})

	t.Run("root admin survives a grant lookup error", func(t *testing.T) {
		grants.err = errors.New("db down")
		defer func() { grants.err = nil }()
		got, err := svc.IsInstanceAdmin(context.Background(), users.users[iaRootID])
		require.NoError(t, err)
		assert.True(t, got)
	})
}

func TestInstanceAdminService_GrantInstanceAdmin(t *testing.T) {
	ctx := context.Background()

	t.Run("root admin grants, and a re-grant is an idempotent no-op", func(t *testing.T) {
		grants := newFakeInstanceAdminGrants()
		svc, users := newInstanceAdminFixture(grants)

		granted, err := svc.GrantInstanceAdmin(ctx, iaRootID, iaPlainID)
		require.NoError(t, err)
		assert.True(t, granted)
		assert.Equal(t, []string{iaRootID}, grants.actors, "the audit entry names the root admin")

		isAdmin, err := svc.IsInstanceAdmin(ctx, users.users[iaPlainID])
		require.NoError(t, err)
		assert.True(t, isAdmin)

		granted, err = svc.GrantInstanceAdmin(ctx, iaRootID, iaPlainID)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Len(t, grants.actors, 1, "a re-grant writes no second audit entry")
	})

	actorRejections := []struct {
		name    string
		actorID string
	}{
		{"DB-granted admin", iaDBAdminID},
		{"plain user", iaPlainID},
		{"unknown actor", "nobody"},
	}
	for _, tc := range actorRejections {
		t.Run("rejects a non-root actor: "+tc.name, func(t *testing.T) {
			grants := newFakeInstanceAdminGrants(iaDBAdminID)
			svc, _ := newInstanceAdminFixture(grants)

			_, err := svc.GrantInstanceAdmin(ctx, tc.actorID, iaPlainID)
			var notRoot *ErrInstanceAdminNotRoot
			require.ErrorAs(t, err, &notRoot)
			assert.False(t, grants.granted[iaPlainID])
		})
	}

	t.Run("rejects a root target", func(t *testing.T) {
		grants := newFakeInstanceAdminGrants()
		svc, _ := newInstanceAdminFixture(grants)

		_, err := svc.GrantInstanceAdmin(ctx, iaRootID, iaRoot2ID)
		var isRoot *ErrInstanceAdminTargetIsRoot
		require.ErrorAs(t, err, &isRoot)
		assert.Equal(t, "root2@example.com", isRoot.Email)
		assert.Empty(t, grants.granted)
	})

	targetRejections := []struct {
		name     string
		targetID string
		reason   string
	}{
		{"unknown target", "nobody", instanceAdminTargetUnknown},
		{"suspended target", iaSuspendedID, instanceAdminTargetSuspended},
	}
	for _, tc := range targetRejections {
		t.Run("rejects an "+tc.name, func(t *testing.T) {
			grants := newFakeInstanceAdminGrants()
			svc, _ := newInstanceAdminFixture(grants)

			_, err := svc.GrantInstanceAdmin(ctx, iaRootID, tc.targetID)
			var invalid *ErrInstanceAdminTargetInvalid
			require.ErrorAs(t, err, &invalid)
			assert.Equal(t, tc.reason, invalid.Reason)
			assert.Empty(t, grants.granted)
		})
	}

	t.Run("a target deleted before the write is invalid", func(t *testing.T) {
		grants := newFakeInstanceAdminGrants()
		grants.writeErr = repositories.ErrUserNotFound
		svc, _ := newInstanceAdminFixture(grants)

		_, err := svc.GrantInstanceAdmin(ctx, iaRootID, iaPlainID)
		var invalid *ErrInstanceAdminTargetInvalid
		require.ErrorAs(t, err, &invalid)
	})

	t.Run("a repository error is propagated", func(t *testing.T) {
		grants := newFakeInstanceAdminGrants()
		grants.writeErr = errors.New("db down")
		svc, _ := newInstanceAdminFixture(grants)

		_, err := svc.GrantInstanceAdmin(ctx, iaRootID, iaPlainID)
		require.ErrorIs(t, err, grants.writeErr)
	})

	t.Run("a user lookup error is propagated", func(t *testing.T) {
		svc, users := newInstanceAdminFixture(newFakeInstanceAdminGrants())
		users.err = errors.New("db down")

		_, err := svc.GrantInstanceAdmin(ctx, iaRootID, iaPlainID)
		require.ErrorIs(t, err, users.err)
	})
}

func TestInstanceAdminService_RevokeInstanceAdmin(t *testing.T) {
	ctx := context.Background()

	t.Run("root admin revokes, and the next check denies", func(t *testing.T) {
		grants := newFakeInstanceAdminGrants(iaDBAdminID)
		svc, users := newInstanceAdminFixture(grants)

		require.NoError(t, svc.RevokeInstanceAdmin(ctx, iaRootID, iaDBAdminID))
		assert.Equal(t, []string{iaRootID}, grants.actors, "the audit entry names the root admin")

		isAdmin, err := svc.IsInstanceAdmin(ctx, users.users[iaDBAdminID])
		require.NoError(t, err)
		assert.False(t, isAdmin)
	})

	t.Run("a suspended DB admin can be revoked", func(t *testing.T) {
		grants := newFakeInstanceAdminGrants(iaSuspendedID)
		svc, _ := newInstanceAdminFixture(grants)

		require.NoError(t, svc.RevokeInstanceAdmin(ctx, iaRootID, iaSuspendedID))
		assert.False(t, grants.granted[iaSuspendedID])
	})

	t.Run("rejects a non-root actor", func(t *testing.T) {
		grants := newFakeInstanceAdminGrants(iaDBAdminID, iaPlainID)
		svc, _ := newInstanceAdminFixture(grants)

		for _, actor := range []string{iaDBAdminID, iaPlainID, iaSuspendedID} {
			err := svc.RevokeInstanceAdmin(ctx, actor, iaPlainID)
			var notRoot *ErrInstanceAdminNotRoot
			require.ErrorAs(t, err, &notRoot, actor)
		}
		assert.True(t, grants.granted[iaPlainID])
	})

	t.Run("a repository error is propagated", func(t *testing.T) {
		grants := newFakeInstanceAdminGrants(iaDBAdminID)
		grants.writeErr = errors.New("db down")
		svc, _ := newInstanceAdminFixture(grants)

		require.ErrorIs(t, svc.RevokeInstanceAdmin(ctx, iaRootID, iaDBAdminID), grants.writeErr)
	})

	t.Run("a user lookup error is propagated", func(t *testing.T) {
		svc, users := newInstanceAdminFixture(newFakeInstanceAdminGrants(iaDBAdminID))
		users.err = errors.New("db down")

		require.ErrorIs(t, svc.RevokeInstanceAdmin(ctx, iaRootID, iaDBAdminID), users.err)
	})

	t.Run("rejects revoking a root admin", func(t *testing.T) {
		svc, _ := newInstanceAdminFixture(newFakeInstanceAdminGrants())

		err := svc.RevokeInstanceAdmin(ctx, iaRootID, iaRoot2ID)
		var isRoot *ErrInstanceAdminTargetIsRoot
		require.ErrorAs(t, err, &isRoot)
	})

	for _, target := range []string{iaPlainID, "nobody"} {
		t.Run("a user without a grant is not granted: "+target, func(t *testing.T) {
			svc, _ := newInstanceAdminFixture(newFakeInstanceAdminGrants())

			err := svc.RevokeInstanceAdmin(ctx, iaRootID, target)
			var notGranted *ErrInstanceAdminNotGranted
			require.ErrorAs(t, err, &notGranted)
			assert.Equal(t, target, notGranted.UserID)
		})
	}
}

func TestInstanceAdminErrors_Messages(t *testing.T) {
	assert.Contains(t, (&ErrInstanceAdminNotRoot{}).Error(), "root instance admin")
	assert.Contains(t, (&ErrInstanceAdminTargetIsRoot{Email: "a@b.c"}).Error(), "a@b.c")
	assert.Contains(t, (&ErrInstanceAdminTargetInvalid{UserID: "u", Reason: "r"}).Error(), "u")
	assert.Contains(t, (&ErrInstanceAdminNotGranted{UserID: "u"}).Error(), "u")
}
