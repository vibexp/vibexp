package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// authRecoveryToggleAttempts bounds how often SetProviderEnabled re-reads a
// provider after losing a compare-and-set to a concurrent settings write.
const authRecoveryToggleAttempts = 3

// AuthRecoveryDeps are the collaborators of the auth recovery service.
type AuthRecoveryDeps struct {
	Providers repositories.InstanceAuthProviderRepository
	Allowlist repositories.InstanceAuthAllowlistRepository
	Setup     repositories.InstanceAuthSetupRepository
	Version   repositories.InstanceAuthSettingsVersionRepository
	// WayIn tells a provider toggle whether password login still works once no
	// provider is enabled. Nil means it is not asked.
	WayIn AuthWayIn
	// RootPasswords sets a root admin's password (#1438). Built without an
	// event publisher: the CLI has no server, so it cannot create accounts.
	RootPasswords *RootAdminPasswordService
	// BaseURL is frontend.base_url, the origin the setup URL is built on.
	BaseURL string
	// Recorder records the root-admin password change in the instance audit
	// log (#1535). The settings changes reach it through the instance settings
	// observer instead. Nil records nothing.
	Recorder audit.Recorder
}

// AuthRecoveryService is the break-glass path out of an authentication
// lockout (#1237): the operations behind `vibexp admin auth`, which run against
// the database with no server. It writes through the same audited repositories
// the API uses, so every effective change bumps the shared auth settings
// version — a running server picks it up on its next resolve — and is audited
// with no actor and source "cli".
//
// It deliberately has none of the API's save guards: disabling the last enabled
// provider is allowed, because unblocking a locked-out instance is the point.
type AuthRecoveryService struct {
	providers repositories.InstanceAuthProviderRepository
	allowlist repositories.InstanceAuthAllowlistRepository
	setup     repositories.InstanceAuthSetupRepository
	version   repositories.InstanceAuthSettingsVersionRepository
	rootPw    *RootAdminPasswordService
	wayIn     AuthWayIn
	baseURL   string
	recorder  audit.Recorder

	// Injectable for tests.
	now func() time.Time
}

// NewAuthRecoveryService creates the auth recovery service.
func NewAuthRecoveryService(deps AuthRecoveryDeps) *AuthRecoveryService {
	return &AuthRecoveryService{
		providers: deps.Providers,
		allowlist: deps.Allowlist,
		setup:     deps.Setup,
		version:   deps.Version,
		rootPw:    deps.RootPasswords,
		wayIn:     deps.WayIn,
		baseURL:   deps.BaseURL,
		recorder:  deps.Recorder,
		now:       time.Now,
	}
}

// ProviderToggle is the outcome of SetProviderEnabled.
type ProviderToggle struct {
	// Changed is false when the provider was already in the requested state,
	// in which case nothing was written or audited.
	Changed bool
	// NoneEnabled reports that no identity provider is enabled after the call.
	// The instance is then in setup mode unless PasswordLoginWorks.
	NoneEnabled bool
	// PasswordLoginWorks reports, when NoneEnabled, that email and password
	// sign-in is still a working way in (#1438): the instance is NOT in setup
	// mode, and re-arming setup would only open a hole.
	PasswordLoginWorks bool
}

// audited marks ctx so the repositories record the write as made by the CLI.
func (s *AuthRecoveryService) audited(ctx context.Context) context.Context {
	return repositories.WithAuditSource(ctx, repositories.AuditSourceCLI)
}

// ListProviders returns every stored identity provider, enabled or not.
func (s *AuthRecoveryService) ListProviders(ctx context.Context) ([]*models.InstanceAuthProvider, error) {
	rows, err := s.providers.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list identity providers: %w", err)
	}
	return rows, nil
}

// SetProviderEnabled enables or disables the provider with slug. It returns
// repositories.ErrInstanceAuthProviderNotFound for an unknown slug.
//
// The repository writes the whole row, so the toggle is a compare-and-set
// against the shared auth settings version read before the row: a concurrent
// edit (a secret rotation, say) is re-read rather than overwritten.
func (s *AuthRecoveryService) SetProviderEnabled(
	ctx context.Context, slug string, enabled bool,
) (ProviderToggle, error) {
	changed, err := s.toggleProvider(ctx, slug, enabled)
	if err != nil {
		return ProviderToggle{}, err
	}
	rows, err := s.ListProviders(ctx)
	if err != nil {
		return ProviderToggle{}, err
	}
	noneEnabled := true
	for _, row := range rows {
		if row.Enabled {
			noneEnabled = false
			break
		}
	}
	toggle := ProviderToggle{Changed: changed, NoneEnabled: noneEnabled}
	if noneEnabled && s.wayIn != nil {
		if toggle.PasswordLoginWorks, err = s.wayIn.PasswordWorks(ctx); err != nil {
			// The toggle itself is done and audited; say so with the failure.
			return toggle, fmt.Errorf("the provider was changed, but whether password sign-in still works "+
				"could not be read: %w", err)
		}
	}
	return toggle, nil
}

// toggleProvider reports whether it changed the provider's enabled state.
func (s *AuthRecoveryService) toggleProvider(ctx context.Context, slug string, enabled bool) (bool, error) {
	for attempt := 1; ; attempt++ {
		version, err := s.version.Get(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to read the auth settings version: %w", err)
		}
		provider, err := s.providers.GetBySlug(ctx, slug)
		if err != nil {
			return false, fmt.Errorf("failed to read identity provider %q: %w", slug, err)
		}
		if provider.Enabled == enabled {
			return false, nil
		}
		provider.Enabled = enabled
		err = s.providers.Update(s.audited(ctx), provider, nil, &version)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, repositories.ErrInstanceSettingsVersionConflict) || attempt == authRecoveryToggleAttempts {
			return false, fmt.Errorf("failed to update identity provider %q: %w", slug, err)
		}
	}
}

// ClearAllowlist removes the access allowlist, reverting the instance to open
// access. It reports false, having written nothing, when none is stored.
func (s *AuthRecoveryService) ClearAllowlist(ctx context.Context) (bool, error) {
	cleared, err := s.allowlist.DeleteAudited(s.audited(ctx), nil)
	if err != nil {
		return false, fmt.Errorf("failed to clear the access allowlist: %w", err)
	}
	return cleared, nil
}

// RearmSetup forces setup mode on and returns a fresh setup URL. The previous
// token and every outstanding setup session stop working.
func (s *AuthRecoveryService) RearmSetup(ctx context.Context) (string, error) {
	token, err := forceRearmSetup(s.audited(ctx), s.setup, s.now())
	if err != nil {
		return "", err
	}
	return SetupURL(s.baseURL, token), nil
}

// SetRootAdminPassword gives the root admin with email a password, and with
// enableLogin switches password login on (audited with source "cli"). It is the
// way back into a password-only instance nobody can sign in to (#1438). The
// account must exist: see RootAdminPasswordService.Set for what it checks and
// leaves behind.
//
// A stored password is recorded as auth.password.set, or auth.password.changed
// when it replaced one, on the account it changed. That includes a call that
// failed AFTER storing it (ending the sessions, switching login on): the
// credential did change, so the log must show it, with the error returned all
// the same. Switching password login on is a settings change of its own,
// recorded by the observer.
func (s *AuthRecoveryService) SetRootAdminPassword(
	ctx context.Context, email, plaintext string, enableLogin bool,
) (*RootAdminPasswordResult, error) {
	result, err := s.rootPw.Set(s.audited(ctx), email, plaintext, enableLogin)
	if result != nil && result.PasswordStored {
		s.recordRootAdminPassword(ctx, result, err)
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

// recordRootAdminPassword records a stored root-admin password. setErr is the
// error of a call that failed after storing it, nil otherwise.
func (s *AuthRecoveryService) recordRootAdminPassword(
	ctx context.Context, result *RootAdminPasswordResult, setErr error,
) {
	op := models.AuditOpAuthLocalCredentialSet
	if result.PasswordReplaced {
		op = models.AuditOpAuthLocalCredentialChanged
	}
	metadata := map[string]any{"login_switched_on": result.LoginSwitchedOn}
	if setErr != nil {
		metadata["completed"] = false
	}
	audit.SafeRecord(ctx, s.recorder, audit.Event{
		Operation:    op,
		ResourceType: models.AuditResourceTypeUser, ResourceID: result.UserID, ResourceName: result.Email,
		Metadata: metadata,
	})
}
