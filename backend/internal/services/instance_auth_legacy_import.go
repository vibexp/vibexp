package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// The upgrade bridge for config.yaml's deprecated auth provider and allowlist
// keys (#1232, epic #1230): auth.providers / auth.provider, auth.google,
// auth.github, auth.oidc and auth.access_allowlist. Sign-in providers and the
// access allowlist move into the database, so an install upgrading with login
// configured in config.yaml (or its docker env) would otherwise end up with no
// providers and open access once the runtime reads the database (#1234,
// #1235). At boot the keys are imported once, when the database holds none;
// afterwards the database is authoritative and the keys are ignored.
//
// The per-boot deprecation warning is not logged here: the config loader
// collects it for every populated legacy key (Config.DeprecationWarnings), so it
// fires even when this bridge has nothing left to do. The keys become fatal
// no earlier than the second minor release after this one (#1240).
//
// Nothing here is fatal: an unreadable table, an encryption, validation or
// repository failure each log and boot continues with what is stored.

// legacyAuthImportFailedMsg is logged when importing the providers fails.
const legacyAuthImportFailedMsg = "Failed to import the config.yaml auth providers"

// legacyAllowlistImportFailedMsg is logged when importing the allowlist fails.
const legacyAllowlistImportFailedMsg = "Failed to import the config.yaml auth.access_allowlist"

// Skip reasons reported (and logged) for a legacy provider that is not imported.
const (
	legacyAuthSkipUnknown    = "unrecognized provider name; it is not enabled today either"
	legacyAuthSkipNotEnabled = "credentials are configured but the provider is not listed in " +
		"auth.providers / auth.provider, so it is not enabled today either"
	legacyAuthSkipIncompleteOAuth = "client_id and client_secret are both required; " +
		"the provider is skipped at startup today"
	legacyAuthSkipIncompleteOIDC = "issuer_url, client_id and client_secret are all required; " +
		"the provider cannot start today"
)

// LegacyAuthImportDeps are the collaborators of ImportLegacyAuthConfig.
type LegacyAuthImportDeps struct {
	ProviderRepo  repositories.InstanceAuthProviderRepository
	AllowlistRepo repositories.InstanceAuthAllowlistRepository
	Enc           EncryptionServiceInterface
	Logger        *slog.Logger
}

// LegacyAuthImportResult reports what ImportLegacyAuthConfig did, for tests.
type LegacyAuthImportResult struct {
	// ProvidersImported are the slugs this run stored.
	ProvidersImported []string
	// ProvidersSkipped maps each legacy provider that was not imported to why.
	// It is filled only when the import ran (no provider was stored).
	ProvidersSkipped map[string]string
	// ProvidersIgnored is true when legacy providers are populated but the
	// database already holds providers.
	ProvidersIgnored bool
	// AllowlistImported is true when this run stored the allowlist.
	AllowlistImported bool
	// AllowlistIgnored is true when the legacy allowlist is populated but the
	// database already holds one.
	AllowlistIgnored bool
}

// legacyAuthCandidate is one provider ready to import. secret is the plaintext
// client secret, encrypted just before the insert so no plaintext ever sits in
// a model that could reach a log line or an audit snapshot.
type legacyAuthCandidate struct {
	provider *models.InstanceAuthProvider
	secret   string
}

// legacyAuthProvidersPopulated reports whether a provider is enabled or any
// provider credential is set.
func legacyAuthProvidersPopulated(auth config.AuthConfig) bool {
	if len(auth.LegacyEnabledProviderNames()) > 0 {
		return true
	}
	return anyNonBlank(
		auth.LegacyGoogle.ClientID, auth.LegacyGoogle.ClientSecret,
		auth.LegacyGitHub.ClientID, auth.LegacyGitHub.ClientSecret,
		auth.LegacyOIDC.IssuerURL, auth.LegacyOIDC.ClientID, auth.LegacyOIDC.ClientSecret,
	)
}

// legacyAuthAllowlistPopulated reports whether the allowlist has a non-blank
// entry. Blank entries (AUTH_ALLOWED_DOMAINS=",") are ignored, as the sign-in
// check ignores them.
func legacyAuthAllowlistPopulated(auth config.AuthConfig) bool {
	return anyNonBlank(auth.LegacyAccessAllowlist.Domains...) || anyNonBlank(auth.LegacyAccessAllowlist.Emails...)
}

func anyNonBlank(values ...string) bool {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// ImportLegacyAuthConfig runs the boot-time bridge once: it imports the legacy
// providers when no provider is stored, and the legacy allowlist when none is
// stored, and logs what it imported, skipped or ignored.
//
// Concurrent replicas are safe by construction: each repository write takes
// the shared auth settings version lock and re-checks emptiness under it, so
// only the first replica stores anything, and only its writes are audited.
func ImportLegacyAuthConfig(
	ctx context.Context, deps LegacyAuthImportDeps, auth config.AuthConfig,
) LegacyAuthImportResult {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var result LegacyAuthImportResult
	importLegacyAuthProviders(ctx, deps, logger, auth, &result)
	importLegacyAuthAllowlist(ctx, deps, logger, auth, &result)
	return result
}

// importLegacyAuthProviders imports the enabled, complete legacy providers as
// one all-or-nothing set when no provider is stored.
func importLegacyAuthProviders(
	ctx context.Context, deps LegacyAuthImportDeps, logger *slog.Logger, auth config.AuthConfig,
	result *LegacyAuthImportResult,
) {
	if !legacyAuthProvidersPopulated(auth) {
		return
	}

	stored, err := deps.ProviderRepo.List(ctx)
	if err != nil {
		logger.Error("Failed to read the instance auth providers; skipping the config.yaml auth provider import",
			"error", err)
		return
	}
	if len(stored) > 0 {
		result.ProvidersIgnored = true
		logger.Warn("The config.yaml auth provider keys are ignored: the sign-in providers stored in the " +
			"database are authoritative")
		return
	}

	candidates, skipped := legacyAuthProviderCandidates(auth)
	result.ProvidersSkipped = skipped
	for _, name := range sortedKeys(skipped) {
		logger.Warn("A config.yaml auth provider was not imported", "provider", name, "reason", skipped[name])
	}
	if len(candidates) == 0 {
		return
	}

	providers, err := encryptLegacyAuthCandidates(deps.Enc, candidates)
	if err != nil {
		logger.Error(legacyAuthImportFailedMsg, "error", err)
		return
	}
	inserted, err := deps.ProviderRepo.InsertIfEmpty(ctx, providers)
	if err != nil {
		logger.Error(legacyAuthImportFailedMsg, "error", err)
		return
	}
	if !inserted {
		// Another replica imported first, or an admin saved in between: what is
		// stored wins, and its writer owns the audit entries.
		logger.Info("Sign-in providers were stored concurrently; the config.yaml auth providers were not imported")
		return
	}

	for _, p := range providers {
		result.ProvidersImported = append(result.ProvidersImported, p.Slug)
	}
	logger.Info("Imported the config.yaml auth providers into the database", "providers", result.ProvidersImported)
}

// legacyAuthProviderCandidates maps the legacy keys onto the provider rows
// to import, and the reason each other legacy provider is left out. It is pure:
// no encryption, no I/O.
//
// A provider is imported exactly when it is enabled today and complete enough
// that today's startup would build it, so the login screen does not change:
// the same names, today's labels (idp.DefaultDisplayName), and today's
// alphabetical picker order (idp.Registry.Enabled) as sort_order. An OIDC
// provider's discovery is deliberately not run: it would put an unbounded
// network call on the boot path.
func legacyAuthProviderCandidates(auth config.AuthConfig) ([]legacyAuthCandidate, map[string]string) {
	skipped := map[string]string{}
	enabled := auth.LegacyEnabledProviderNames()
	sort.Strings(enabled)

	isEnabled := make(map[string]bool, len(enabled))
	candidates := make([]legacyAuthCandidate, 0, len(enabled))
	for _, name := range enabled {
		isEnabled[name] = true
		candidate, reason := mapLegacyAuthProvider(auth, name)
		if reason != "" {
			skipped[name] = reason
			continue
		}
		candidate.provider.SortOrder = len(candidates)
		candidates = append(candidates, candidate)
	}

	for _, known := range []models.InstanceAuthProviderType{
		models.InstanceAuthProviderGitHub, models.InstanceAuthProviderGoogle, models.InstanceAuthProviderOIDC,
	} {
		if !isEnabled[string(known)] && legacyAuthProviderHasCredentials(auth, known) {
			skipped[string(known)] = legacyAuthSkipNotEnabled
		}
	}
	return candidates, skipped
}

// mapLegacyAuthProvider maps one enabled provider name; a non-empty reason says
// why it is not imported.
func mapLegacyAuthProvider(auth config.AuthConfig, name string) (legacyAuthCandidate, string) {
	var clientID, secret, issuer string
	providerType := models.InstanceAuthProviderType(name)
	switch providerType {
	case models.InstanceAuthProviderGoogle:
		clientID, secret = auth.LegacyGoogle.ClientID, auth.LegacyGoogle.ClientSecret
	case models.InstanceAuthProviderGitHub:
		clientID, secret = auth.LegacyGitHub.ClientID, auth.LegacyGitHub.ClientSecret
	case models.InstanceAuthProviderOIDC:
		clientID, secret, issuer = auth.LegacyOIDC.ClientID, auth.LegacyOIDC.ClientSecret, auth.LegacyOIDC.IssuerURL
	default:
		return legacyAuthCandidate{}, legacyAuthSkipUnknown
	}

	clientID, issuer = strings.TrimSpace(clientID), strings.TrimSpace(issuer)
	if clientID == "" || !hasLegacySecret(secret) {
		return legacyAuthCandidate{}, legacyAuthIncompleteReason(providerType)
	}

	provider := &models.InstanceAuthProvider{
		Type:        providerType,
		Slug:        name,
		DisplayName: idp.DefaultDisplayName(idp.ProviderName(name)),
		Enabled:     true,
		ClientID:    clientID,
	}
	if providerType == models.InstanceAuthProviderOIDC {
		if issuer == "" {
			return legacyAuthCandidate{}, legacyAuthSkipIncompleteOIDC
		}
		provider.IssuerURL = &issuer
	}
	if err := ValidateInstanceAuthProvider(*provider); err != nil {
		return legacyAuthCandidate{}, "invalid: " + err.Error()
	}
	// The secret is taken verbatim (it may legitimately carry spaces); only a
	// blank one is treated as absent.
	return legacyAuthCandidate{provider: provider, secret: secret}, ""
}

func legacyAuthIncompleteReason(providerType models.InstanceAuthProviderType) string {
	if providerType == models.InstanceAuthProviderOIDC {
		return legacyAuthSkipIncompleteOIDC
	}
	return legacyAuthSkipIncompleteOAuth
}

// legacyAuthProviderHasCredentials reports whether any credential of a
// provider kind is set.
func legacyAuthProviderHasCredentials(auth config.AuthConfig, providerType models.InstanceAuthProviderType) bool {
	switch providerType {
	case models.InstanceAuthProviderGoogle:
		return anyNonBlank(auth.LegacyGoogle.ClientID, auth.LegacyGoogle.ClientSecret)
	case models.InstanceAuthProviderGitHub:
		return anyNonBlank(auth.LegacyGitHub.ClientID, auth.LegacyGitHub.ClientSecret)
	default:
		return anyNonBlank(auth.LegacyOIDC.IssuerURL, auth.LegacyOIDC.ClientID, auth.LegacyOIDC.ClientSecret)
	}
}

// encryptLegacyAuthCandidates encrypts every candidate's secret into its
// provider row. Any failure fails the whole set: it is imported all or nothing.
func encryptLegacyAuthCandidates(
	enc EncryptionServiceInterface, candidates []legacyAuthCandidate,
) ([]*models.InstanceAuthProvider, error) {
	providers := make([]*models.InstanceAuthProvider, 0, len(candidates))
	for _, c := range candidates {
		secret := c.secret
		encrypted, err := encryptLegacySecret(enc, &secret)
		if err != nil {
			return nil, fmt.Errorf("provider %s: %w", c.provider.Slug, err)
		}
		c.provider.ClientSecretEncrypted = encrypted
		providers = append(providers, c.provider)
	}
	return providers, nil
}

// importLegacyAuthAllowlist imports the legacy allowlist, normalized and
// validated like an admin save, when none is stored. An invalid entry is
// dropped (and logged) rather than failing the whole list: it can never match
// a user today, and rejecting the list would leave nothing stored — which is
// open access once the stored allowlist is enforced (#1235).
func importLegacyAuthAllowlist(
	ctx context.Context, deps LegacyAuthImportDeps, logger *slog.Logger, auth config.AuthConfig,
	result *LegacyAuthImportResult,
) {
	if !legacyAuthAllowlistPopulated(auth) {
		return
	}

	_, err := deps.AllowlistRepo.Get(ctx)
	switch {
	case err == nil:
		result.AllowlistIgnored = true
		logger.Warn("The config.yaml auth.access_allowlist is ignored: the access allowlist stored in the " +
			"database is authoritative")
		return
	case !errors.Is(err, repositories.ErrInstanceAuthAllowlistNotFound):
		logger.Error("Failed to read the instance auth allowlist; skipping the config.yaml auth.access_allowlist import",
			"error", err)
		return
	}

	legacy := auth.LegacyAccessAllowlist
	domains := keepValidLegacyAllowlistEntries(logger, "domains", legacy.Domains, isInstanceAuthDomain)
	emails := keepValidLegacyAllowlistEntries(logger, "emails", legacy.Emails, isInstanceAuthEmail)
	if len(domains) == 0 && len(emails) == 0 {
		// Importing nothing is not neutral: once the stored allowlist is enforced
		// (#1235), none stored means open access.
		logger.Error("The config.yaml auth.access_allowlist has no valid entry; nothing was imported, and sign-in " +
			"becomes open to everyone once the database allowlist is enforced. Set the allowlist under " +
			"Admin → Settings → Authentication")
		return
	}
	// The kept entries are valid, so this only normalizes and de-duplicates.
	domains, emails, err = ValidateInstanceAuthAllowlist(domains, emails)
	if err != nil {
		logger.Error(legacyAllowlistImportFailedMsg, "error", err)
		return
	}

	inserted, err := deps.AllowlistRepo.InsertIfAbsent(ctx, &models.InstanceAuthAllowlist{
		Domains: domains, Emails: emails,
	})
	if err != nil {
		logger.Error(legacyAllowlistImportFailedMsg, "error", err)
		return
	}
	if !inserted {
		logger.Info("The access allowlist was stored concurrently; the config.yaml auth.access_allowlist " +
			"was not imported")
		return
	}
	result.AllowlistImported = true
	logger.Info("Imported the config.yaml auth.access_allowlist into the database",
		"domains", len(domains), "emails", len(emails))
}

// keepValidLegacyAllowlistEntries returns the entries of one legacy allowlist
// list that the admin API would accept, logging each one it drops. Blank
// entries are skipped silently, as the sign-in check skips them.
func keepValidLegacyAllowlistEntries(
	logger *slog.Logger, field string, entries []string, valid func(string) bool,
) []string {
	kept := make([]string, 0, len(entries))
	for _, entry := range entries {
		value := strings.ToLower(strings.TrimSpace(entry))
		if value == "" {
			continue
		}
		if !valid(value) {
			logger.Warn("An invalid config.yaml auth.access_allowlist entry was not imported",
				"list", field, "entry", entry)
			continue
		}
		kept = append(kept, value)
	}
	return kept
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
