package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

const (
	relationDefaultListLimit = 20
	relationMaxListLimit     = 100
)

// RelationService implements RelationServiceInterface.
//
// Like CommentService it deliberately takes NO event publisher: relations are
// not embedded/searchable and not a domain event source — that is guaranteed by
// construction, by this service emitting no domain event.
type RelationService struct {
	repo        repositories.RelationRepository
	teamService TeamServiceInterface
	authz       AuthorizationServiceInterface
	logger      *slog.Logger
}

// Ensure RelationService implements RelationServiceInterface.
var _ RelationServiceInterface = (*RelationService)(nil)

// NewRelationService creates a new RelationService.
func NewRelationService(
	repo repositories.RelationRepository,
	teamService TeamServiceInterface,
	authzService AuthorizationServiceInterface,
	logger *slog.Logger,
) *RelationService {
	return &RelationService{
		repo:        repo,
		teamService: teamService,
		authz:       authzService,
		logger:      logger,
	}
}

// checkMembership verifies the user is a member of the team. Reads (list) have
// no matrix permission of their own, so membership is the gate.
func (s *RelationService) checkMembership(ctx context.Context, userID, teamID string) error {
	if s.teamService == nil {
		return nil
	}
	isMember, err := s.teamService.IsUserMemberOfTeam(ctx, userID, teamID)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to validate team membership for relation operation")
		return fmt.Errorf("failed to validate team membership")
	}
	if !isMember {
		return fmt.Errorf("user is not a member of the specified team")
	}
	return nil
}

// Create adds a typed edge between two resources. It validates the endpoint
// types against the relation-type matrix, rejects self-links and cross-project
// links, checks the caller may create resources, confirms both endpoints exist,
// and applies the tiered-trust initial status. Creation is idempotent.
func (s *RelationService) Create(
	ctx context.Context, userID, teamID string, req *models.CreateRelationRequest,
) (*models.Relation, bool, error) {
	if err := validateCreateRelationRequest(req); err != nil {
		return nil, false, err
	}

	// Creating a relation is open to any team member (epic #220), but the caller
	// must still BE one — Can proves the role grants the action.
	if authzErr := s.authz.Can(ctx, userID, teamID, authz.ResourceCreate); authzErr != nil {
		return nil, false, authzErr
	}

	// Both endpoints must exist in the team and share a project.
	projectID, err := s.resolveRelationProject(ctx, userID, teamID, req)
	if err != nil {
		return nil, false, err
	}

	created := userID
	relation := &models.Relation{
		TeamID:       teamID,
		ProjectID:    projectID,
		FromType:     req.FromType,
		FromID:       req.FromID,
		ToType:       req.ToType,
		ToID:         req.ToID,
		RelationType: req.RelationType,
		Origin:       req.Origin,
		Status:       models.InitialRelationStatus(req.Origin, req.RelationType),
		CreatedBy:    &created,
	}
	out, wasCreated, err := s.repo.Create(ctx, relation)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"relation_type", req.RelationType,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create relation")
		return nil, false, err
	}
	return out, wasCreated, nil
}

// validateCreateRelationRequest checks the enums, rejects self-links, and
// enforces the relation-type -> object-type matrix (governed-by -> blueprint,
// built-from -> prompt, explained-by -> memory, supersedes -> same type as the
// subject). Each failure gets a distinct error.
func validateCreateRelationRequest(req *models.CreateRelationRequest) error {
	if !models.IsValidRelationResourceType(req.FromType) || !models.IsValidRelationResourceType(req.ToType) {
		return fmt.Errorf("invalid resource type")
	}
	if !models.IsValidRelationType(req.RelationType) {
		return fmt.Errorf("invalid relation type: %q", req.RelationType)
	}
	if !models.IsValidRelationOrigin(req.Origin) {
		return fmt.Errorf("invalid origin: %q", req.Origin)
	}
	if req.FromType == req.ToType && req.FromID == req.ToID {
		return ErrRelationSelfLink
	}
	requiredObj, _ := models.RequiredObjectType(req.RelationType, req.FromType)
	if req.ToType != requiredObj {
		return ErrRelationInvalidType
	}
	return nil
}

// resolveRelationProject verifies both endpoints exist in the team and share a
// project, returning that project id (which becomes the edge's project). Missing
// endpoints and cross-project pairs get distinct errors.
func (s *RelationService) resolveRelationProject(
	ctx context.Context, userID, teamID string, req *models.CreateRelationRequest,
) (string, error) {
	fromProject, fromExists, err := s.repo.ResourceProjectID(ctx, teamID, req.FromType, req.FromID)
	if err != nil {
		return "", s.logResourceLookupError(userID, teamID, req.FromType, req.FromID, err)
	}
	if !fromExists {
		return "", ErrRelationResourceNotFound
	}
	toProject, toExists, err := s.repo.ResourceProjectID(ctx, teamID, req.ToType, req.ToID)
	if err != nil {
		return "", s.logResourceLookupError(userID, teamID, req.ToType, req.ToID, err)
	}
	if !toExists {
		return "", ErrRelationResourceNotFound
	}
	if fromProject != toProject {
		return "", ErrRelationCrossProject
	}
	return fromProject, nil
}

func (s *RelationService) logResourceLookupError(
	userID, teamID, resourceType, resourceID string, err error,
) error {
	s.logger.With(
		"user_id", userID,
		"team_id", teamID,
		"resource_type", resourceType,
		"resource_id", resourceID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to resolve relation endpoint")
	return err
}

// Confirm flips a suggested edge to confirmed, recording the caller. It requires
// resource.update.any and returns ErrRelationAlreadyConfirmed when the edge is
// already confirmed.
func (s *RelationService) Confirm(
	ctx context.Context, userID, teamID, relationID string,
) (*models.Relation, error) {
	relation, err := s.repo.GetByID(ctx, teamID, relationID)
	if err != nil {
		return nil, err
	}

	if authzErr := s.authz.Can(ctx, userID, teamID, authz.ResourceUpdateAny); authzErr != nil {
		return nil, authzErr
	}

	if relation.Status == models.RelationStatusConfirmed {
		return nil, ErrRelationAlreadyConfirmed
	}

	confirmed, err := s.repo.Confirm(ctx, teamID, relationID, userID)
	if err != nil {
		// The status='suggested' guard matched no row: a concurrent confirm won
		// the race (we already saw the row above), so report it as such.
		if errors.Is(err, repositories.ErrRelationNotFound) {
			return nil, ErrRelationAlreadyConfirmed
		}
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"relation_id", relationID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to confirm relation")
		return nil, err
	}
	return confirmed, nil
}

// Delete removes an edge. Delete is own-vs-any: the creator deletes their own,
// Admin/Owner delete any edge in the team. An edge whose creator has been
// deleted (created_by NULL) is deletable only by Admin/Owner.
func (s *RelationService) Delete(ctx context.Context, userID, teamID, relationID string) error {
	relation, err := s.repo.GetByID(ctx, teamID, relationID)
	if err != nil {
		return err
	}

	ownerID := ""
	if relation.CreatedBy != nil {
		ownerID = *relation.CreatedBy
	}
	if authzErr := s.authz.CanActOnResource(
		ctx, userID, teamID, ownerID,
		authz.ResourceDeleteOwn, authz.ResourceDeleteAny,
	); authzErr != nil {
		return authzErr
	}

	if delErr := s.repo.Delete(ctx, teamID, relationID); delErr != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"relation_id", relationID,
			"error", fmt.Sprintf("%+v", delErr),
		).Error("Failed to delete relation")
		return delErr
	}
	return nil
}

// ListByResource returns a page of the relations touching a resource (both
// directions), newest-first, for a team member.
func (s *RelationService) ListByResource(
	ctx context.Context, userID, teamID, resourceType, resourceID string, page, limit int,
) (*models.RelationListResponse, error) {
	if !models.IsValidRelationResourceType(resourceType) {
		return nil, fmt.Errorf("invalid resource type: %q", resourceType)
	}

	page, limit = clampRelationPage(page, limit)

	if err := s.checkMembership(ctx, userID, teamID); err != nil {
		return nil, err
	}

	related, totalCount, err := s.repo.ListByResource(ctx, teamID, resourceType, resourceID, page, limit)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"resource_type", resourceType,
			"resource_id", resourceID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list relations")
		return nil, err
	}

	totalPages := int(math.Ceil(float64(totalCount) / float64(limit)))
	return &models.RelationListResponse{
		Related:    related,
		TotalCount: totalCount,
		Page:       page,
		PerPage:    limit,
		TotalPages: totalPages,
	}, nil
}

// clampRelationPage applies the standard pagination defaults/ceilings.
func clampRelationPage(page, limit int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = relationDefaultListLimit
	} else if limit > relationMaxListLimit {
		limit = relationMaxListLimit
	}
	return page, limit
}
