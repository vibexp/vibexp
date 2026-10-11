package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// Resource locks (issue #1288, epic #1285). Locking, unlocking and reading the
// lock history work the same way for prompts, blueprints and artifacts, so the
// rules live in services.ResourceLockService and this file holds what the
// three strict servers share on top of it: the service call, the activity
// record and the error mapping. Each domain's own handler file only resolves
// its resource and converts the result to its generated types.

// lockTarget identifies the resource a lock operation acts on. The handler
// has already resolved it by slug, so the id is known to exist in the team.
type lockTarget struct {
	// resourceType is one of the models.RelationResourceType* values a lock
	// applies to; it doubles as the noun in error messages.
	resourceType string
	teamID       string
	resourceID   string
	slug         string
}

// lockEntityTypes maps a lockable resource type to the activity entity type its
// lock activities are recorded against.
var lockEntityTypes = map[string]string{
	models.RelationResourceTypePrompt:    activities.EntityTypePrompt,
	models.RelationResourceTypeBlueprint: activities.EntityTypeBlueprint,
	models.RelationResourceTypeArtifact:  activities.EntityTypeArtifact,
}

// lockResource locks the target for the caller and records the activity.
func (s *Server) lockResource(ctx context.Context, userID string, target lockTarget, comment string) error {
	err := s.container.ResourceLockService().Lock(
		ctx, userID, target.teamID, target.resourceType, target.resourceID, comment,
	)
	if err != nil {
		return s.resourceLockError("lock", target, err)
	}
	s.recordLockActivity(ctx, userID, activities.ActivityTypeResourceLocked, target)
	return nil
}

// unlockResource releases the target's lock and records the activity.
func (s *Server) unlockResource(ctx context.Context, userID string, target lockTarget) error {
	err := s.container.ResourceLockService().Unlock(
		ctx, userID, target.teamID, target.resourceType, target.resourceID,
	)
	if err != nil {
		return s.resourceLockError("unlock", target, err)
	}
	s.recordLockActivity(ctx, userID, activities.ActivityTypeResourceUnlocked, target)
	return nil
}

// resourceLockHistory returns the target's lock and unlock events, newest
// first.
func (s *Server) resourceLockHistory(
	ctx context.Context, userID string, target lockTarget,
) ([]*models.ResourceLockEvent, error) {
	events, err := s.container.ResourceLockService().History(
		ctx, userID, target.teamID, target.resourceType, target.resourceID,
	)
	if err != nil {
		return nil, s.resourceLockError("read the lock history of", target, err)
	}
	return events, nil
}

// resourceLockError maps a ResourceLockService error onto the API error the
// lock operations document. Anything unrecognized is logged and reported as an
// opaque 500.
func (s *Server) resourceLockError(action string, target lockTarget, err error) error {
	noun := target.resourceType
	switch {
	case errors.Is(err, services.ErrLockCommentRequired), errors.Is(err, services.ErrLockCommentTooLong):
		return apierrors.NewBadRequestError(err.Error())
	case errors.Is(err, services.ErrPermissionDenied):
		return apierrors.NewForbiddenError("You do not have permission to " + action + " this " + noun)
	case isLockableResourceNotFound(err):
		// The handler resolved the resource a moment ago, so this is a delete
		// racing the lock operation.
		return apierrors.NewResourceNotFoundError(noun, "The "+noun+" was not found")
	case errors.Is(err, repositories.ErrResourceAlreadyLocked):
		return apierrors.NewResourceLockedError(noun)
	case errors.Is(err, repositories.ErrResourceNotLocked):
		return apierrors.NewResourceNotLockedError(noun)
	default:
		s.logger.With(
			"service", serverLogServiceName,
			"resource_type", target.resourceType,
			"resource_id", target.resourceID,
			"error", err,
		).Error("Failed to " + action + " resource")
		return apierrors.NewInternalError("Failed to " + action + " the " + noun)
	}
}

// writeResourceWriteLocked answers a write refused by a lock (#1289) with the
// documented 423 and reports whether it handled err. The prompt, blueprint and
// artifact update and delete handlers call it before their own error mapping,
// which would otherwise turn the refusal into a 500. The lock goes out in the
// problem's metadata; detail is the service's message, the same sentence the
// MCP tools print. r may be nil where the caller has no request in scope.
func writeResourceWriteLocked(w http.ResponseWriter, r *http.Request, err error) bool {
	var locked *services.ResourceLockedError
	if !errors.As(err, &locked) {
		return false
	}
	if r == nil {
		r = &http.Request{URL: &url.URL{Path: "/unknown"}}
	}
	apierrors.WriteJSONError(w, r, apierrors.NewResourceWriteLockedError(locked.Error(), apierrors.ResourceWriteLock{
		ResourceType: locked.ResourceType,
		Field:        locked.Field,
		LockedByID:   locked.LockedBy,
		LockedByName: locked.LockedByName,
		Comment:      locked.Comment,
	}))
	return true
}

// isLockableResourceNotFound reports whether err is the not-found sentinel of
// one of the three lockable resource repositories.
func isLockableResourceNotFound(err error) bool {
	return errors.Is(err, repositories.ErrPromptNotFound) ||
		errors.Is(err, repositories.ErrBlueprintNotFound) ||
		errors.Is(err, repositories.ErrArtifactNotFound)
}

// recordLockActivity records a lock or unlock in the acting user's activity
// log. Recording is best effort, like every other resource activity: a
// failure is logged and never fails the request.
//
// It calls the activity service directly rather than going through
// ActivityRecorder, which needs the *http.Request a strict-server handler does
// not have.
func (s *Server) recordLockActivity(ctx context.Context, userID, activityType string, target lockTarget) {
	if s.activityService == nil {
		return
	}
	resourceID := target.resourceID
	err := s.activityService.RecordResourceActivity(
		ctx, userID, activityType, lockEntityTypes[target.resourceType], &resourceID,
		lockActivityDescriptions[activityType]+": "+target.slug,
		map[string]interface{}{
			"resource_type": target.resourceType,
			"slug":          target.slug,
		},
	)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"activity_type", activityType,
			"resource_id", target.resourceID,
			"error", err,
		).Error("Failed to record resource lock activity")
	}
}

// lockActivityDescriptions is the human-readable lead of a lock activity's
// description; the resource's slug is appended.
var lockActivityDescriptions = map[string]string{
	activities.ActivityTypeResourceLocked:   "Locked resource",
	activities.ActivityTypeResourceUnlocked: "Unlocked resource",
}
