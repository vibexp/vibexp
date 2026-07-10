package services

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// EmbeddingStatusServiceInterface computes derived embedding coverage for a team.
type EmbeddingStatusServiceInterface interface {
	// GetCoverage reports, per entity type, how many of the team's embeddable
	// entities have an embedding under the team's active provider model vs. are still
	// pending. A team with no active provider is reported as all-pending (0%), not an
	// error.
	GetCoverage(ctx context.Context, teamID string) (*models.EmbeddingCoverageResponse, error)
}

// EmbeddingStatusService derives embedding coverage by resolving a team's active
// provider model and diffing the source tables against the embeddings table. It owns
// no state of its own — "pending" is total − embedded and the percentage is computed
// on read (per the epic decision that status is derived, not tracked).
type EmbeddingStatusService struct {
	providerRepo repositories.EmbeddingProviderRepository
	coverageRepo repositories.EmbeddingBackfillRepository
	logger       *slog.Logger
}

var _ EmbeddingStatusServiceInterface = (*EmbeddingStatusService)(nil)

// NewEmbeddingStatusService creates a new EmbeddingStatusService.
func NewEmbeddingStatusService(
	providerRepo repositories.EmbeddingProviderRepository,
	coverageRepo repositories.EmbeddingBackfillRepository,
	logger *slog.Logger,
) *EmbeddingStatusService {
	return &EmbeddingStatusService{
		providerRepo: providerRepo,
		coverageRepo: coverageRepo,
		logger:       logger,
	}
}

// GetCoverage resolves the team's active model, counts total vs. embedded per entity
// type under that model, and derives pending + percent. When the team has no active
// provider it counts against an empty model (nothing is embedded), yielding all-pending
// coverage with HasActiveProvider=false and a null ActiveModel.
func (s *EmbeddingStatusService) GetCoverage(
	ctx context.Context, teamID string,
) (*models.EmbeddingCoverageResponse, error) {
	modelID := ""
	hasProvider := false

	provider, err := s.providerRepo.GetActiveProvider(ctx, teamID)
	switch {
	case err == nil:
		modelID = provider.Model
		hasProvider = true
	case stderrors.Is(err, repositories.ErrNoActiveEmbeddingProvider):
		// No active provider is a normal state, not an error: report everything as
		// pending so the UI can tell "not configured" from "configured but stuck".
	default:
		return nil, fmt.Errorf("failed to resolve active embedding provider: %w", err)
	}

	counts, err := s.coverageRepo.CountCoverage(ctx, modelID, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to count embedding coverage: %w", err)
	}

	resp := &models.EmbeddingCoverageResponse{
		HasActiveProvider: hasProvider,
		Coverage:          make([]models.EmbeddingCoverageItem, 0, len(counts)),
	}
	if hasProvider {
		model := modelID
		resp.ActiveModel = &model
	}
	for _, c := range counts {
		resp.Coverage = append(resp.Coverage, models.EmbeddingCoverageItem{
			EntityType:      c.EntityType,
			Total:           c.Total,
			Embedded:        c.Embedded,
			Pending:         c.Total - c.Embedded,
			EmbeddedPercent: embeddedPercent(c.Embedded, c.Total),
		})
	}
	return resp, nil
}

// embeddedPercent is round(embedded / total * 100), guarding against a divide by zero
// when a team has no entities of a type (0% rather than an error).
func embeddedPercent(embedded, total int64) int {
	if total <= 0 {
		return 0
	}
	return int(math.Round(float64(embedded) / float64(total) * 100))
}
