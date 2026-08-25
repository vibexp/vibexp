package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// copyNameSuffix is appended to a source provider's name when the destination
// team already holds it: " (copy)", then " (copy 2)", " (copy 3)", …
const copyNameSuffix = "copy"

// copyNameMaxAttempts bounds the disambiguation loop. The candidates come from
// a snapshot of the destination's names, so the loop is finite by construction;
// the bound only stops a pathological team (hundreds of literal "X (copy N)"
// rows) from spinning, and failing loudly beats inventing "X (copy 741)".
const copyNameMaxAttempts = 100

// modelProviderNameMaxLen mirrors model_providers.name — varchar(255). A
// suffix pushing past it would be truncated by Postgres into a name that no
// longer matches what the collision check cleared, so the BASE is trimmed
// instead and the suffix always survives intact.
const modelProviderNameMaxLen = 255

// CopyFromTeam copies one provider out of another team into this one (#830,
// epic #827).
//
// The credential moves as ciphertext: the source row's api_key_encrypted is
// written straight to the new row and is never decrypted, never held in
// plaintext, and never touches a request or response body. That is the whole
// reason this is a server-side endpoint — API keys are write-only over the API,
// so a client copying a provider by hand cannot bring the key with it.
//
// Authorization goes through AuthorizeCrossTeamCopy with authz.TeamUpdate, the
// same permission the destination's own create path requires, evaluated on BOTH
// teams with the destination first — so a caller entitled to neither cannot use
// the response to probe whether the source team exists.
func (mps *ModelProviderService) CopyFromTeam(
	ctx context.Context, params CopyModelProviderParams,
) (*models.ModelProvider, error) {
	if mps == nil || mps.repo == nil {
		return nil, fmt.Errorf("ModelProviderService is nil")
	}
	if mps.authz == nil {
		return nil, fmt.Errorf("%w: authorization service is not configured", ErrPermissionDenied)
	}

	if err := AuthorizeCrossTeamCopy(
		ctx, mps.authz, params.UserID, params.TeamID, params.SourceTeamID, authz.TeamUpdate,
	); err != nil {
		return nil, err
	}

	// Tenant-scoped by construction: a provider id that is not in the source
	// team simply does not resolve, so this cannot reach across into a third.
	source, err := mps.repo.GetByID(ctx, params.SourceTeamID, params.SourceProviderID)
	if err != nil {
		if errors.Is(err, repositories.ErrModelProviderNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrModelProviderNotFound, params.SourceProviderID)
		}
		return nil, fmt.Errorf("failed to read the source model provider: %w", err)
	}

	name, err := mps.resolveCopyName(ctx, params, source.Name)
	if err != nil {
		return nil, err
	}

	configuration, err := copyConfiguration(params.Configuration, source.Configuration)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	provider := &models.ModelProvider{
		UserID:       params.UserID,
		TeamID:       &params.TeamID,
		Name:         name,
		ProviderType: valueOr(params.ProviderType, source.ProviderType),
		Model:        valueOr(params.Model, source.Model),
		// Always non-default. Create writes is_default straight into the INSERT,
		// so a true here would hit the partial unique index
		// idx_model_providers_team_default before any SetDefault could run —
		// and a copy silently displacing the destination's chosen default is
		// not something the caller asked for either way.
		IsDefault: false,
		BaseURL:   copyBaseURL(params.BaseURL, source.BaseURL),
		// Ciphertext, verbatim. Not decrypted, not re-encrypted: one
		// instance-wide key means the destination can read it as-is, and a
		// decrypt/re-encrypt round trip would put the key in memory for no gain.
		APIKeyEncrypted: source.APIKeyEncrypted,
		Configuration:   configuration,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if createErr := mps.repo.Create(ctx, provider); createErr != nil {
		// Reachable despite the collision check above: another writer can take
		// the name in between, and a caller-supplied name is never
		// disambiguated in the first place.
		if isDuplicateProviderError(createErr) {
			return nil, fmt.Errorf("%w: %s", ErrModelProviderAlreadyExists, name)
		}
		return nil, fmt.Errorf("failed to create the copied model provider: %w", createErr)
	}

	if auditErr := mps.recordCopy(ctx, params, source, provider); auditErr != nil {
		return nil, auditErr
	}

	return provider, nil
}

// resolveCopyName decides what the copy is called.
//
// A caller-supplied name wins outright and is used verbatim — the caller has
// seen the destination's providers and made a choice, so silently renaming it
// would be worse than the 409 a collision earns. Only a name INHERITED from the
// source is disambiguated, because the caller never chose it.
func (mps *ModelProviderService) resolveCopyName(
	ctx context.Context, params CopyModelProviderParams, sourceName string,
) (string, error) {
	if params.Name != nil {
		return *params.Name, nil
	}

	existing, err := mps.repo.ListNames(ctx, params.TeamID)
	if err != nil {
		return "", fmt.Errorf("failed to list the destination team's provider names: %w", err)
	}

	taken := make(map[string]struct{}, len(existing))
	for _, name := range existing {
		taken[name] = struct{}{}
	}

	if _, clash := taken[sourceName]; !clash {
		return sourceName, nil
	}

	for attempt := 1; attempt <= copyNameMaxAttempts; attempt++ {
		candidate := copyNameCandidate(sourceName, attempt)
		if _, clash := taken[candidate]; !clash {
			return candidate, nil
		}
	}

	return "", fmt.Errorf(
		"%w: %s (and %d generated variants of it)",
		ErrModelProviderAlreadyExists, sourceName, copyNameMaxAttempts,
	)
}

// copyNameCandidate builds the nth disambiguated name: attempt 1 is
// "<base> (copy)", attempt 2 "<base> (copy 2)", and so on — the numbering a
// reader expects, where the unnumbered form IS the first copy.
func copyNameCandidate(base string, attempt int) string {
	suffix := " (" + copyNameSuffix + ")"
	if attempt > 1 {
		suffix = " (" + copyNameSuffix + " " + strconv.Itoa(attempt) + ")"
	}

	if overflow := len(base) + len(suffix) - modelProviderNameMaxLen; overflow > 0 {
		base = strings.TrimRight(base[:max(len(base)-overflow, 0)], " ")
	}
	return base + suffix
}

// recordCopy writes the single audit row for this copy. Unlike the custom-types
// copy — one action, many rows, ids in Detail — a provider copy is one-to-one,
// so both SourceResourceID and CreatedResourceID are populated.
//
// A failure here fails the copy. The row is the compensating control for
// moving a credential between teams, and reporting success for a copy whose
// audit did not land would defeat the point of having it.
func (mps *ModelProviderService) recordCopy(
	ctx context.Context,
	params CopyModelProviderParams,
	source, created *models.ModelProvider,
) error {
	if _, err := mps.audit.Record(ctx, TeamSettingsAuditRecord{
		TeamID:            params.TeamID,
		ActorUserID:       params.UserID,
		Surface:           models.SettingsAuditSurfaceModelProvider,
		SourceTeamID:      params.SourceTeamID,
		SourceResourceID:  source.ID,
		CreatedResourceID: created.ID,
		Detail: map[string]interface{}{
			"source_name":   source.Name,
			"created_name":  created.Name,
			"provider_type": created.ProviderType,
			"model":         created.Model,
			// Whether a credential moved is the fact worth auditing; the
			// credential itself obviously is not.
			"has_api_key": created.APIKeyEncrypted != nil && *created.APIKeyEncrypted != "",
		},
	}); err != nil {
		return fmt.Errorf("failed to record model provider copy: %w", err)
	}
	return nil
}

// copyConfiguration marshals an override, or keeps the source's stored JSON.
func copyConfiguration(override *map[string]interface{}, sourceConfiguration string) (string, error) {
	if override == nil {
		if sourceConfiguration == "" {
			return "{}", nil
		}
		return sourceConfiguration, nil
	}

	configBytes, err := json.Marshal(*override)
	if err != nil {
		return "", fmt.Errorf("failed to marshal configuration: %w", err)
	}
	return string(configBytes), nil
}

// copyBaseURL applies an override, treating an explicit empty string as "store
// no base URL" — the spec documents null and "" as the same instruction.
func copyBaseURL(override, sourceBaseURL *string) *string {
	if override == nil {
		return sourceBaseURL
	}
	if *override == "" {
		return nil
	}
	return override
}

// valueOr returns the override when one was sent, else the source's value.
func valueOr(override *string, sourceValue string) string {
	if override == nil {
		return sourceValue
	}
	return *override
}
