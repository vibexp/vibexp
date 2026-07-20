package services

import (
	"context"
	"log/slog"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

const (
	// relationSeedCrossTypeMaxDistance is the loose cosine-distance ceiling for a
	// cross-type suggestion (governed-by / built-from / explained-by). It also
	// bounds the candidate query, so it must be >= the same-type ceiling.
	relationSeedCrossTypeMaxDistance = 0.35
	// relationSeedSameTypeMaxDistance is the tighter ceiling a same-type pair must
	// clear to be treated as a near-duplicate supersedes suggestion.
	relationSeedSameTypeMaxDistance = 0.15
	// relationSeedCandidateLimit caps the candidate pairs one run considers.
	relationSeedCandidateLimit = 2000
)

// RelationSeedServiceInterface is the one-shot per-team embedding-similarity
// backfill that seeds origin=ai, status=suggested edges (issue #426, §8.1
// cold-start mitigation).
type RelationSeedServiceInterface interface {
	// Backfill types every similarity candidate for the team via the relation
	// matrix and creates the resulting edges (idempotently). It returns a run
	// summary and never partially fails on a single bad candidate.
	Backfill(ctx context.Context, userID, teamID string) (models.RelationSeedSummary, error)
}

// RelationSeedService implements RelationSeedServiceInterface.
type RelationSeedService struct {
	repo            repositories.RelationRepository
	relationService RelationServiceInterface
	logger          *slog.Logger
}

var _ RelationSeedServiceInterface = (*RelationSeedService)(nil)

// NewRelationSeedService creates a new RelationSeedService.
func NewRelationSeedService(
	repo repositories.RelationRepository,
	relationService RelationServiceInterface,
	logger *slog.Logger,
) *RelationSeedService {
	return &RelationSeedService{
		repo:            repo,
		relationService: relationService,
		logger:          logger,
	}
}

// Backfill seeds ai-suggested edges for the team from embedding similarity. Each
// candidate is typed deterministically via the matrix; cross-project / invalid
// candidates are rejected by RelationService.Create and counted, not fatal.
func (s *RelationSeedService) Backfill(
	ctx context.Context, userID, teamID string,
) (models.RelationSeedSummary, error) {
	var summary models.RelationSeedSummary

	candidates, err := s.repo.FindSeedCandidates(
		ctx, teamID, relationSeedCrossTypeMaxDistance, relationSeedCandidateLimit,
	)
	if err != nil {
		return summary, err
	}
	summary.Candidates = len(candidates)

	for i := range candidates {
		req, ok := seedCandidateToRequest(candidates[i])
		if !ok {
			continue // no matrix rule for this pair, or same-type over the tighter threshold
		}
		_, created, createErr := s.relationService.Create(ctx, userID, teamID, req)
		if createErr != nil {
			// Cross-project, matrix, or missing-endpoint rejections are expected on
			// noisy similarity output — skip, don't fail the whole run.
			summary.SkippedInvalid++
			continue
		}
		if created {
			summary.Seeded++
		} else {
			summary.SkippedExisting++
		}
	}

	s.logger.With(
		"service", "relation_seed",
		"team_id", teamID,
		"candidates", summary.Candidates,
		"seeded", summary.Seeded,
		"skipped_existing", summary.SkippedExisting,
		"skipped_invalid", summary.SkippedInvalid,
	).Info("Relation seed backfill complete")
	return summary, nil
}

// seedCandidateToRequest types a similarity candidate deterministically. A
// cross-type pair's OBJECT type fixes the relation (governed-by→blueprint,
// built-from→prompt, explained-by→memory); an object of type artifact has no
// matrix rule (the reverse-ordered candidate carries the valid direction). A
// same-type near-duplicate (under the tighter threshold, distinct timestamps)
// becomes a supersedes edge from the NEWER to the OLDER resource. Returns
// (nil, false) when the candidate maps to no edge. Every edge is origin=ai.
func seedCandidateToRequest(c models.RelationSeedCandidate) (*models.CreateRelationRequest, bool) {
	if c.FromType != c.ToType {
		var relationType string
		switch c.ToType {
		case models.RelationResourceTypeBlueprint:
			relationType = models.RelationTypeGovernedBy
		case models.RelationResourceTypePrompt:
			relationType = models.RelationTypeBuiltFrom
		case models.RelationResourceTypeMemory:
			relationType = models.RelationTypeExplainedBy
		default:
			return nil, false
		}
		return &models.CreateRelationRequest{
			FromType:     c.FromType,
			FromID:       c.FromID,
			ToType:       c.ToType,
			ToID:         c.ToID,
			RelationType: relationType,
			Origin:       models.RelationOriginAI,
		}, true
	}

	// Same type → supersedes candidate, but only for genuine near-duplicates.
	if c.Distance >= relationSeedSameTypeMaxDistance || c.FromUpdatedAt.Equal(c.ToUpdatedAt) {
		return nil, false
	}
	newerID, olderID := c.FromID, c.ToID
	if c.FromUpdatedAt.Before(c.ToUpdatedAt) {
		newerID, olderID = c.ToID, c.FromID // the newer resource supersedes the older
	}
	return &models.CreateRelationRequest{
		FromType:     c.FromType,
		FromID:       newerID,
		ToType:       c.ToType,
		ToID:         olderID,
		RelationType: models.RelationTypeSupersedes,
		Origin:       models.RelationOriginAI,
	}, true
}
