package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/logging"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// MaxLockCommentLength is the longest lock comment accepted, in characters
// (runes), counted after surrounding whitespace is trimmed. The spec's
// LockResourceRequest.comment publishes the same bound.
const MaxLockCommentLength = 500

// ResourceLockHistoryLimit is how many events History returns at most. The
// history endpoint is not paginated; it answers with the newest events only.
const ResourceLockHistoryLimit = 100

var (
	// ErrLockCommentRequired is returned by Lock when the comment is empty once
	// surrounding whitespace is trimmed.
	ErrLockCommentRequired = errors.New("a lock comment is required")

	// ErrLockCommentTooLong is returned by Lock when the trimmed comment is
	// longer than MaxLockCommentLength characters.
	ErrLockCommentTooLong = fmt.Errorf("a lock comment may be at most %d characters", MaxLockCommentLength)

	// ErrResourceTypeNotLockable is returned for a resource type that cannot be
	// locked (see models.IsLockableResourceType). No request can produce it:
	// every caller passes a constant.
	ErrResourceTypeNotLockable = errors.New("resource type cannot be locked")
)

// ResourceLockServiceInterface locks and unlocks prompts, blueprints and
// artifacts and reads their lock history (epic #1285). It is the one place the
// lock rules live; the REST handlers and the MCP lock tool both call it.
//
// resourceType is one of the models.RelationResourceType* values a lock applies
// to, and resourceID the resource's id (not its slug).
type ResourceLockServiceInterface interface {
	// Lock locks the resource for userID, who must be a member of the team.
	// The comment is trimmed; ErrLockCommentRequired or ErrLockCommentTooLong
	// reports one that is then empty or too long. A resource that is already
	// locked reports repositories.ErrResourceAlreadyLocked, and one that does
	// not exist in the team its own not-found sentinel.
	Lock(ctx context.Context, userID, teamID, resourceType, resourceID, comment string) error

	// Unlock releases the resource's lock. The member who placed the lock may
	// release it, and so may a member whose role holds
	// authz.ResourceUnlockAny; anyone else gets ErrPermissionDenied. A resource
	// that is not locked reports repositories.ErrResourceNotLocked.
	Unlock(ctx context.Context, userID, teamID, resourceType, resourceID string) error

	// History returns the resource's lock and unlock events, newest first and
	// at most ResourceLockHistoryLimit of them, to any member of the team. The
	// result is never nil.
	History(
		ctx context.Context, userID, teamID, resourceType, resourceID string,
	) ([]*models.ResourceLockEvent, error)
}

// lockStore is the lock surface of one resource repository: the two atomic
// lock writes and a read of the lock currently held.
type lockStore interface {
	SetLock(ctx context.Context, teamID, resourceID, lockerID, comment string) error
	ClearLock(ctx context.Context, teamID, resourceID string) error
	// currentLock returns the lock held on the resource, nil when it is
	// unlocked, and the resource's not-found sentinel when it does not exist in
	// the team.
	currentLock(ctx context.Context, userID, teamID, resourceID string) (*models.ResourceLock, error)
}

// The three adapters below differ only in which repository they read the
// resource from; SetLock and ClearLock are promoted from the embedded
// repository unchanged.

type promptLockStore struct{ repositories.PromptRepository }

func (s promptLockStore) currentLock(
	ctx context.Context, userID, teamID, resourceID string,
) (*models.ResourceLock, error) {
	prompt, err := s.GetByID(ctx, userID, teamID, resourceID)
	if err != nil {
		return nil, err
	}
	return prompt.Lock, nil
}

type blueprintLockStore struct {
	repositories.BlueprintRepository
}

func (s blueprintLockStore) currentLock(
	ctx context.Context, userID, teamID, resourceID string,
) (*models.ResourceLock, error) {
	blueprint, err := s.GetByID(ctx, userID, teamID, resourceID)
	if err != nil {
		return nil, err
	}
	return blueprint.Lock, nil
}

type artifactLockStore struct {
	repositories.ArtifactRepository
}

func (s artifactLockStore) currentLock(
	ctx context.Context, userID, teamID, resourceID string,
) (*models.ResourceLock, error) {
	artifact, err := s.GetByID(ctx, userID, teamID, resourceID)
	if err != nil {
		return nil, err
	}
	return artifact.Lock, nil
}

// ResourceLockServiceDeps groups the dependencies of ResourceLockService.
type ResourceLockServiceDeps struct {
	Prompts    repositories.PromptRepository
	Blueprints repositories.BlueprintRepository
	Artifacts  repositories.ArtifactRepository
	Events     repositories.ResourceLockEventRepository
	Authorizer AuthorizationServiceInterface
	Logger     *slog.Logger
}

// ResourceLockService is the concrete ResourceLockServiceInterface.
type ResourceLockService struct {
	stores     map[string]lockStore
	events     repositories.ResourceLockEventRepository
	authorizer AuthorizationServiceInterface
	logger     *slog.Logger
}

// NewResourceLockService creates a new ResourceLockService.
func NewResourceLockService(deps ResourceLockServiceDeps) *ResourceLockService {
	logger := deps.Logger
	if logger == nil {
		logger = logging.New(logging.Config{})
	}
	return &ResourceLockService{
		stores: map[string]lockStore{
			models.RelationResourceTypePrompt:    promptLockStore{deps.Prompts},
			models.RelationResourceTypeBlueprint: blueprintLockStore{deps.Blueprints},
			models.RelationResourceTypeArtifact:  artifactLockStore{deps.Artifacts},
		},
		events:     deps.Events,
		authorizer: deps.Authorizer,
		logger:     logger,
	}
}

// Lock implements ResourceLockServiceInterface.
//
// The write is the repository's single conditional UPDATE, so of two
// concurrent calls exactly one wins and the other gets
// ErrResourceAlreadyLocked; there is no read-then-write here.
func (s *ResourceLockService) Lock(
	ctx context.Context, userID, teamID, resourceType, resourceID, comment string,
) error {
	store, err := s.authorizedStore(ctx, userID, teamID, resourceType)
	if err != nil {
		return err
	}

	comment = strings.TrimSpace(comment)
	if comment == "" {
		return ErrLockCommentRequired
	}
	if utf8.RuneCountInString(comment) > MaxLockCommentLength {
		return ErrLockCommentTooLong
	}

	if err := store.SetLock(ctx, teamID, resourceID, userID, comment); err != nil {
		return err
	}
	s.appendEvent(ctx, &models.ResourceLockEvent{
		TeamID:       teamID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       models.LockActionLock,
		ActorUserID:  &userID,
		Comment:      &comment,
	})
	return nil
}

// Unlock implements ResourceLockServiceInterface.
//
// Who may unlock is decided against the lock as read here, and the lock is
// then cleared by a separate statement. Between the two it could be released
// and taken again by someone else, in which case this call clears that newer
// lock. That window is accepted: the caller was entitled to unlock the
// resource a moment earlier, and an owner or admin may clear any lock anyway.
//
// CanActOnResource is deliberately not used. It compares the caller to the
// resource's creator, while unlocking turns on who placed the lock, and the
// matrix has no "own" unlock permission to pair with ResourceUnlockAny.
func (s *ResourceLockService) Unlock(
	ctx context.Context, userID, teamID, resourceType, resourceID string,
) error {
	store, err := s.authorizedStore(ctx, userID, teamID, resourceType)
	if err != nil {
		return err
	}

	lock, err := store.currentLock(ctx, userID, teamID, resourceID)
	if err != nil {
		return err
	}
	if lock == nil {
		return repositories.ErrResourceNotLocked
	}
	if lock.LockedBy != userID {
		if err := s.authorizer.Can(ctx, userID, teamID, authz.ResourceUnlockAny); err != nil {
			return err
		}
	}

	if err := store.ClearLock(ctx, teamID, resourceID); err != nil {
		return err
	}
	s.appendEvent(ctx, &models.ResourceLockEvent{
		TeamID:       teamID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       models.LockActionUnlock,
		ActorUserID:  &userID,
	})
	return nil
}

// History implements ResourceLockServiceInterface.
//
// The resource itself is not looked up: the history outlives the resource it
// describes, and the events are read by team, so another team's resource id
// simply has no events here.
func (s *ResourceLockService) History(
	ctx context.Context, userID, teamID, resourceType, resourceID string,
) ([]*models.ResourceLockEvent, error) {
	if _, err := s.authorizedStore(ctx, userID, teamID, resourceType); err != nil {
		return nil, err
	}
	events, err := s.events.ListByResource(ctx, teamID, resourceType, resourceID, ResourceLockHistoryLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to list lock history: %w", err)
	}
	return events, nil
}

// authorizedStore resolves the lock store for resourceType and proves the
// caller is a member of the team. Membership is the whole requirement for
// locking and for reading the history; Unlock adds its own check on top.
func (s *ResourceLockService) authorizedStore(
	ctx context.Context, userID, teamID, resourceType string,
) (lockStore, error) {
	store, ok := s.stores[resourceType]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrResourceTypeNotLockable, resourceType)
	}
	if err := s.authorizer.IsMember(ctx, userID, teamID); err != nil {
		return nil, err
	}
	return store, nil
}

// appendEvent writes one history event after a lock change has been
// committed.
//
// A failure is logged and not returned. The lock write and the history insert
// are separate statements, so by the time the insert can fail the resource's
// lock state has already changed; reporting an error would tell the caller the
// opposite of what happened, and a retry would then meet a 409.
func (s *ResourceLockService) appendEvent(ctx context.Context, event *models.ResourceLockEvent) {
	if err := s.events.Append(ctx, event); err != nil {
		s.logger.With(
			"team_id", event.TeamID,
			"resource_type", event.ResourceType,
			"resource_id", event.ResourceID,
			"action", event.Action,
			"error", err,
		).ErrorContext(ctx, "Failed to append resource lock history event")
	}
}
