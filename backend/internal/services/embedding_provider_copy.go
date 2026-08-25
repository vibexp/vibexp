package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// CopyFromTeam copies one embedding provider out of another team into this one
// (#831, epic #827).
//
// It is the same ciphertext-to-ciphertext copy as the model-provider one (#830)
// with one addition that is the whole reason this endpoint is not a thin clone:
// it reports what the copy did to the destination team's SEARCH.
//
// Every resource a team owns lives in one global vector(1024) HNSW index, and
// nothing in the system objects if half of them were embedded with one model and
// half with another. The result is not an error — it is quietly worse search. So
// the danger is not "the copy became the default": the active provider is
// resolved as `ORDER BY is_default DESC, updated_at DESC LIMIT 1`, which means a
// copy written NON-default still takes over the moment the destination team has
// no default set. Gating a warning on the default flag would therefore miss
// precisely the common case.
//
// The verdict is measured, not predicted: the active provider is read before and
// after the insert, through the same repository method the embedding pipeline
// itself uses. Re-deriving that ORDER BY in Go would be a second copy of the rule
// and free to drift from the one that matters.
func (eps *EmbeddingProviderService) CopyFromTeam(
	ctx context.Context, params CopyEmbeddingProviderParams,
) (*CopyEmbeddingProviderResult, error) {
	if err := eps.copyPreconditions(); err != nil {
		return nil, err
	}

	if err := AuthorizeCrossTeamCopy(
		ctx, eps.authz, params.UserID, params.TeamID, params.SourceTeamID, authz.TeamUpdate,
	); err != nil {
		return nil, err
	}

	source, err := eps.loadCopySource(ctx, params)
	if err != nil {
		return nil, err
	}

	name, err := eps.resolveCopyName(ctx, params, source.Name)
	if err != nil {
		return nil, err
	}

	configuration, err := copyConfiguration(params.Configuration, source.Configuration)
	if err != nil {
		return nil, err
	}

	// Read the active provider BEFORE the insert. Anything derived from it later
	// describes the state the copy displaced.
	displaced, err := eps.activeProvider(ctx, params.TeamID)
	if err != nil {
		return nil, err
	}

	provider := buildEmbeddingProviderCopy(params, source, name, configuration)

	if createErr := eps.repo.Create(ctx, provider); createErr != nil {
		// Reachable despite the collision check above: another writer can take
		// the name in between, and a caller-supplied name is never
		// disambiguated in the first place.
		if isDuplicateProviderError(createErr) {
			return nil, fmt.Errorf("%w: %s", ErrProviderAlreadyExists, name)
		}
		return nil, fmt.Errorf("failed to create the copied embedding provider: %w", createErr)
	}

	activation, err := eps.resolveActivation(ctx, params.TeamID, provider, displaced)
	if err != nil {
		return nil, err
	}

	if auditErr := eps.recordCopy(ctx, params, source, provider, activation); auditErr != nil {
		return nil, auditErr
	}

	return &CopyEmbeddingProviderResult{Provider: provider, Activation: activation}, nil
}

// copyPreconditions fails closed on a service that was constructed without the
// collaborators the copy needs — a wiring bug, not a caller error.
func (eps *EmbeddingProviderService) copyPreconditions() error {
	if eps == nil || eps.repo == nil {
		return fmt.Errorf("EmbeddingProviderService is nil")
	}
	if eps.authz == nil {
		return fmt.Errorf("%w: authorization service is not configured", ErrPermissionDenied)
	}
	return nil
}

// loadCopySource reads the row being copied out of the SOURCE team.
//
// Tenant-scoped by construction: a provider id that is not in the source team
// simply does not resolve, so this cannot reach across into a third.
func (eps *EmbeddingProviderService) loadCopySource(
	ctx context.Context, params CopyEmbeddingProviderParams,
) (*models.EmbeddingProvider, error) {
	source, err := eps.repo.GetByID(ctx, params.SourceTeamID, params.SourceProviderID)
	if err == nil {
		return source, nil
	}
	// Unlike GetEmbeddingProvider, which collapses every read failure to
	// not-found, a storage error here stays a storage error: a copy that
	// reported 404 for an unreachable database would send the caller looking
	// for a provider that exists.
	if errors.Is(err, repositories.ErrEmbeddingProviderNotFound) {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, params.SourceProviderID)
	}
	return nil, fmt.Errorf("failed to read the source embedding provider: %w", err)
}

// buildEmbeddingProviderCopy assembles the destination's row from the source's,
// applying whichever overrides the caller sent.
func buildEmbeddingProviderCopy(
	params CopyEmbeddingProviderParams,
	source *models.EmbeddingProvider,
	name, configuration string,
) *models.EmbeddingProvider {
	now := time.Now()
	return &models.EmbeddingProvider{
		UserID:       params.UserID,
		TeamID:       &params.TeamID,
		Name:         name,
		ProviderType: valueOr(params.ProviderType, source.ProviderType),
		Model:        valueOr(params.Model, source.Model),
		// Chunk sizing, concurrency and the instruction prefixes are part of how
		// the model was tuned, not incidental settings: a copy that silently reset
		// them to the create-path defaults would embed differently from the source
		// while claiming to be the same provider.
		ChunkSize:      intValueOr(params.ChunkSize, source.ChunkSize),
		ChunkOverlap:   intValueOr(params.ChunkOverlap, source.ChunkOverlap),
		Concurrency:    intValueOr(params.Concurrency, source.Concurrency),
		QueryPrefix:    copyOptionalString(params.QueryPrefix, source.QueryPrefix),
		DocumentPrefix: copyOptionalString(params.DocumentPrefix, source.DocumentPrefix),
		// Always non-default. Create writes is_default straight into the INSERT,
		// so a true here would hit the partial unique index
		// idx_embedding_providers_team_default before any SetDefault could run —
		// and a copy silently displacing the destination's chosen default is not
		// something the caller asked for either way. Note this does NOT make the
		// copy inert; see resolveActivation.
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
}

// resolveActivation answers the question the endpoint exists for: did this copy
// just take over the destination team's embedding, and at whose expense?
//
// It re-reads the active provider after the insert rather than re-implementing
// `is_default DESC, updated_at DESC` — so the verdict is by definition the one
// the embedding pipeline will act on.
func (eps *EmbeddingProviderService) resolveActivation(
	ctx context.Context,
	teamID string,
	created *models.EmbeddingProvider,
	displaced *models.EmbeddingProvider,
) (CopyEmbeddingProviderActivation, error) {
	nowActive, err := eps.activeProvider(ctx, teamID)
	if err != nil {
		return CopyEmbeddingProviderActivation{}, err
	}

	activation := CopyEmbeddingProviderActivation{
		BecomesActive: nowActive != nil && nowActive.ID == created.ID,
	}
	if !activation.BecomesActive || displaced == nil {
		return activation, nil
	}

	model := displaced.Model
	activation.DisplacedModel = &model
	// A copy whose model matches the one it displaced changes credentials or
	// endpoint, not vector space: the stored embeddings remain comparable, so
	// there is nothing to re-embed and nothing to warn about.
	activation.ModelChanged = displaced.Model != created.Model

	count, err := eps.countEmbeddedWith(ctx, teamID, displaced.Model)
	if err != nil {
		return CopyEmbeddingProviderActivation{}, err
	}
	activation.DisplacedEmbeddedResources = count

	return activation, nil
}

// activeProvider reads the team's effective provider, mapping "the team has
// none" to (nil, nil) — an empty destination is an ordinary state for a copy,
// not a failure.
func (eps *EmbeddingProviderService) activeProvider(
	ctx context.Context, teamID string,
) (*models.EmbeddingProvider, error) {
	provider, err := eps.repo.GetActiveProvider(ctx, teamID)
	if err != nil {
		if errors.Is(err, repositories.ErrNoActiveEmbeddingProvider) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to resolve the destination team's active embedding provider: %w", err)
	}
	return provider, nil
}

// countEmbeddedWith totals the team's resources that hold an embedding under
// modelID, reusing the coverage counter the embedding-status endpoint reports
// from — same "has an embedding for this model" predicate as the missing-only
// backfill, so the number a warning quotes agrees with the number a subsequent
// re-embed would actually regenerate.
func (eps *EmbeddingProviderService) countEmbeddedWith(
	ctx context.Context, teamID, modelID string,
) (int64, error) {
	if eps.coverageRepo == nil || modelID == "" {
		return 0, nil
	}

	counts, err := eps.coverageRepo.CountCoverage(ctx, modelID, teamID)
	if err != nil {
		return 0, fmt.Errorf("failed to count resources embedded with the displaced model: %w", err)
	}

	var total int64
	for _, c := range counts {
		total += c.Embedded
	}
	return total, nil
}

// resolveCopyName decides what the copy is called, delegating the naming rule
// itself to the shared resolveCopyName in cross_team_copy.go so the two provider
// surfaces number and trim identically.
func (eps *EmbeddingProviderService) resolveCopyName(
	ctx context.Context, params CopyEmbeddingProviderParams, sourceName string,
) (string, error) {
	var existing []string
	if params.Name == nil {
		var err error
		existing, err = eps.repo.ListNames(ctx, params.TeamID)
		if err != nil {
			return "", fmt.Errorf("failed to list the destination team's provider names: %w", err)
		}
	}

	name, err := resolveCopyName(params.Name, sourceName, existing)
	if err != nil {
		if errors.Is(err, ErrCopyNameExhausted) {
			return "", fmt.Errorf("%w: %s (and %d generated variants of it)",
				ErrProviderAlreadyExists, sourceName, copyNameMaxAttempts)
		}
		return "", err
	}
	return name, nil
}

// recordCopy writes the single audit row for this copy. A provider copy is
// one-to-one, so both SourceResourceID and CreatedResourceID are populated.
//
// A failure here fails the copy. The row is the compensating control for moving
// a credential between teams, and reporting success for a copy whose audit did
// not land would defeat the point of having it. The activation verdict is
// recorded alongside, because "this copy silently took over the team's search"
// is exactly the kind of after-the-fact question an audit trail is read for.
func (eps *EmbeddingProviderService) recordCopy(
	ctx context.Context,
	params CopyEmbeddingProviderParams,
	source, created *models.EmbeddingProvider,
	activation CopyEmbeddingProviderActivation,
) error {
	detail := map[string]interface{}{
		"source_name":   source.Name,
		"created_name":  created.Name,
		"provider_type": created.ProviderType,
		"model":         created.Model,
		// Whether a credential moved is the fact worth auditing; the credential
		// itself obviously is not.
		"has_api_key":    created.APIKeyEncrypted != nil && *created.APIKeyEncrypted != "",
		"becomes_active": activation.BecomesActive,
	}
	if activation.DisplacedModel != nil {
		detail["displaced_model"] = *activation.DisplacedModel
		detail["displaced_embedded_resources"] = activation.DisplacedEmbeddedResources
	}

	if _, err := eps.audit.Record(ctx, TeamSettingsAuditRecord{
		TeamID:            params.TeamID,
		ActorUserID:       params.UserID,
		Surface:           models.SettingsAuditSurfaceEmbeddingProvider,
		SourceTeamID:      params.SourceTeamID,
		SourceResourceID:  source.ID,
		CreatedResourceID: created.ID,
		Detail:            detail,
	}); err != nil {
		return fmt.Errorf("failed to record embedding provider copy: %w", err)
	}
	return nil
}

// intValueOr returns the override when one was sent, else the source's value.
func intValueOr(override *int, sourceValue int) int {
	if override == nil {
		return sourceValue
	}
	return *override
}

// copyOptionalString applies a nullable override, treating an explicit empty
// string as "store nothing" — the spec documents null and "" as the same
// instruction for both instruction prefixes.
func copyOptionalString(override, sourceValue *string) *string {
	if override == nil {
		return sourceValue
	}
	if *override == "" {
		return nil
	}
	return override
}
