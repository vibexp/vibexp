package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// wayInPolicy is the stored policy row, or none.
type wayInPolicy struct {
	repositories.InstanceAuthPolicyRepository
	policy *models.InstanceAuthPolicy
	err    error
}

func (p wayInPolicy) Get(context.Context) (*models.InstanceAuthPolicy, error) {
	if p.err != nil {
		return nil, p.err
	}
	if p.policy == nil {
		return nil, repositories.ErrInstanceAuthPolicyNotFound
	}
	return p.policy, nil
}

// wayInCredentials answers the admin count and records what it was asked.
type wayInCredentials struct {
	repositories.UserCredentialRepository
	admins     int
	err        error
	askedEmail [][]string
}

func (c *wayInCredentials) CountUsableForAdmins(_ context.Context, rootEmails []string) (int, error) {
	c.askedEmail = append(c.askedEmail, rootEmails)
	return c.admins, c.err
}

func TestAuthWayIn(t *testing.T) {
	ctx := context.Background()
	on := &models.InstanceAuthPolicy{PasswordLoginEnabled: true, RegistrationMode: models.RegistrationModeOpen}
	off := &models.InstanceAuthPolicy{RegistrationMode: models.RegistrationModeOpen}
	enabled := []*models.InstanceAuthProvider{{Slug: "off"}, {Slug: "sso", Enabled: true}}
	disabled := []*models.InstanceAuthProvider{{Slug: "off"}}

	for name, tc := range map[string]struct {
		providers    []*models.InstanceAuthProvider
		policy       *models.InstanceAuthPolicy
		admins       int
		wantPassword bool
		wantAny      bool
	}{
		"nothing configured":                            {},
		"a provider is enabled":                         {providers: enabled, wantAny: true},
		"only disabled providers":                       {providers: disabled},
		"password login on, an admin has a password":    {policy: on, admins: 1, wantPassword: true, wantAny: true},
		"password login on, no admin has a password":    {policy: on},
		"password login off, an admin has a password":   {policy: off, admins: 2},
		"no stored policy, an admin has a password":     {admins: 1},
		"both a provider and a working password login":  {providers: enabled, policy: on, admins: 1, wantPassword: true, wantAny: true},
		"a provider, and password login that is unused": {providers: enabled, policy: on, wantAny: true},
	} {
		t.Run(name, func(t *testing.T) {
			credentials := &wayInCredentials{admins: tc.admins}
			wayIn := NewAuthWayIn(AuthWayInDeps{
				Providers: &setupProviderRows{rows: tc.providers}, Policies: wayInPolicy{policy: tc.policy},
				Credentials: credentials, RootAdminEmails: func() []string { return []string{"root@example.com"} },
			})

			password, err := wayIn.PasswordWorks(ctx)
			require.NoError(t, err)
			assert.Equal(t, tc.wantPassword, password, "password login is a working way in")
			anyWay, err := wayIn.Any(ctx)
			require.NoError(t, err)
			assert.Equal(t, tc.wantAny, anyWay, "some way in works")
			for _, asked := range credentials.askedEmail {
				assert.Equal(t, []string{"root@example.com"}, asked, "the configured root admins are who is counted")
			}
		})
	}

	t.Run("an enabled provider answers without reading the policy or the credentials", func(t *testing.T) {
		wayIn := NewAuthWayIn(AuthWayInDeps{
			Providers: &setupProviderRows{rows: enabled},
			Policies:  wayInPolicy{err: errors.New("must not be read")},
		})

		anyWay, err := wayIn.Any(ctx)

		require.NoError(t, err)
		assert.True(t, anyWay)
	})

	t.Run("a read failure is an error, never an answer", func(t *testing.T) {
		boom := errors.New("pq: down")
		for name, deps := range map[string]AuthWayInDeps{
			"providers": {Providers: &setupProviderRows{err: boom}, Policies: wayInPolicy{}},
			"policy":    {Providers: &setupProviderRows{}, Policies: wayInPolicy{err: boom}},
			"credentials": {Providers: &setupProviderRows{}, Policies: wayInPolicy{policy: on},
				Credentials: &wayInCredentials{err: boom}, RootAdminEmails: func() []string { return nil }},
		} {
			_, err := NewAuthWayIn(deps).Any(ctx)
			require.ErrorIs(t, err, boom, name)
		}
	})
}
