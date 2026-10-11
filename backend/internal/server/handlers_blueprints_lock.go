package server

import (
	"context"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	blueprintsgen "github.com/vibexp/vibexp/internal/server/gen/blueprints"
)

// The blueprint lock operations (issue #1288, epic #1285). The rules, the
// activity record and the error mapping are shared with blueprints and
// artifacts in handlers_resource_lock.go; this file resolves the blueprint and
// converts the result.

// LockBlueprint locks a blueprint for the caller and returns the updated blueprint.
func (bs *blueprintsStrictServer) LockBlueprint(
	ctx context.Context, request blueprintsgen.LockBlueprintRequestObject,
) (blueprintsgen.LockBlueprintResponseObject, error) {
	// The generated decoder enforces neither `required` nor `minLength`, so a
	// missing comment arrives as "" and the service rejects it.
	comment := ""
	if request.Body != nil {
		comment = request.Body.Comment
	}

	body, err := bs.changeBlueprintLock(ctx, request.TeamId.String(), request.ProjectId, request.Slug,
		func(userID string, target lockTarget) error {
			return bs.s.lockResource(ctx, userID, target, comment)
		})
	if err != nil {
		return nil, err
	}
	return blueprintsgen.LockBlueprint200JSONResponse(body), nil
}

// UnlockBlueprint releases a blueprint's lock and returns the updated blueprint.
func (bs *blueprintsStrictServer) UnlockBlueprint(
	ctx context.Context, request blueprintsgen.UnlockBlueprintRequestObject,
) (blueprintsgen.UnlockBlueprintResponseObject, error) {
	body, err := bs.changeBlueprintLock(ctx, request.TeamId.String(), request.ProjectId, request.Slug,
		func(userID string, target lockTarget) error {
			return bs.s.unlockResource(ctx, userID, target)
		})
	if err != nil {
		return nil, err
	}
	return blueprintsgen.UnlockBlueprint200JSONResponse(body), nil
}

// GetBlueprintLockHistory returns a blueprint's lock and unlock events, newest first.
func (bs *blueprintsStrictServer) GetBlueprintLockHistory(
	ctx context.Context, request blueprintsgen.GetBlueprintLockHistoryRequestObject,
) (blueprintsgen.GetBlueprintLockHistoryResponseObject, error) {
	userID, target, err := bs.blueprintLockTarget(ctx, request.TeamId.String(), request.ProjectId, request.Slug)
	if err != nil {
		return nil, err
	}

	events, err := bs.s.resourceLockHistory(ctx, userID, target)
	if err != nil {
		return nil, err
	}

	body, err := toGenBlueprintLockHistory(events)
	if err != nil {
		bs.s.logger.With("error", err, "operation", "GetBlueprintLockHistory").
			Error("Blueprints response conversion failed")
		return nil, apierrors.NewInternalError(blueprintsMsgInternalError)
	}
	return blueprintsgen.GetBlueprintLockHistory200JSONResponse(body), nil
}

// blueprintLockTarget resolves the blueprint a lock operation addresses by (project_id, slug). The
// project id is a plain string in the path, so it is validated here as the
// detail read does.
func (bs *blueprintsStrictServer) blueprintLockTarget(
	ctx context.Context, teamID, pathProjectID, slug string,
) (string, lockTarget, error) {
	userID, err := blueprintsUserID(ctx)
	if err != nil {
		return "", lockTarget{}, err
	}
	projectID, err := blueprintPathProjectID(pathProjectID)
	if err != nil {
		return "", lockTarget{}, err
	}
	blueprint, err := bs.loadBlueprint(userID, teamID, projectID, slug)
	if err != nil {
		return "", lockTarget{}, err
	}
	return userID, lockTarget{
		resourceType: models.RelationResourceTypeBlueprint,
		teamID:       teamID,
		resourceID:   blueprint.ID,
		slug:         blueprint.Slug,
	}, nil
}

// changeBlueprintLock is the shared body of LockBlueprint and UnlockBlueprint: resolve
// the blueprint, apply the change, then read the blueprint again so the response
// carries the lock state the change left behind.
func (bs *blueprintsStrictServer) changeBlueprintLock(
	ctx context.Context, teamID, pathProjectID, slug string, change func(userID string, target lockTarget) error,
) (blueprintsgen.BlueprintDetail, error) {
	userID, target, err := bs.blueprintLockTarget(ctx, teamID, pathProjectID, slug)
	if err != nil {
		return blueprintsgen.BlueprintDetail{}, err
	}
	if err = change(userID, target); err != nil {
		return blueprintsgen.BlueprintDetail{}, err
	}

	// pathProjectID was validated by blueprintLockTarget above.
	blueprint, err := bs.loadBlueprint(userID, teamID, pathProjectID, slug)
	if err != nil {
		return blueprintsgen.BlueprintDetail{}, err
	}
	return bs.blueprintDetailBody(ctx, userID, teamID, blueprint)
}

// toGenBlueprintLock converts the lock a blueprint carries. Nil stays nil, which the
// generated field -- required+nullable, hence no omitempty -- renders as
// `"lock": null`.
func toGenBlueprintLock(src *models.ResourceLock) *blueprintsgen.ResourceLock {
	if src == nil {
		return nil
	}
	return &blueprintsgen.ResourceLock{
		LockedBy: blueprintsgen.ResourceLockActor{UserId: src.LockedBy, DisplayName: src.LockedByName},
		Comment:  src.Comment,
		LockedAt: src.LockedAt,
	}
}

// toGenBlueprintLockHistory converts a blueprint's lock history.
//
// `make(..., 0, ...)` is what guarantees the required `events` array
// serializes as `[]` and never `null`: generated strict-server types cannot use
// the models.JSONArray shim (ResourceLockHistoryResponse is registered in
// adHocRequiredArrayAllowlist for exactly that reason).
func toGenBlueprintLockHistory(
	events []*models.ResourceLockEvent,
) (blueprintsgen.ResourceLockHistoryResponse, error) {
	out := make([]blueprintsgen.ResourceLockEvent, 0, len(events))
	for _, event := range events {
		id, err := blueprintUUID("lock event id", event.ID)
		if err != nil {
			return blueprintsgen.ResourceLockHistoryResponse{}, err
		}
		converted := blueprintsgen.ResourceLockEvent{
			Id:        id,
			Action:    blueprintsgen.ResourceLockEventAction(event.Action),
			Comment:   event.Comment,
			CreatedAt: event.CreatedAt,
		}
		// The actor is null once the account is deleted (actor_user_id is
		// ON DELETE SET NULL).
		if event.ActorUserID != nil {
			converted.Actor = &blueprintsgen.ResourceLockActor{
				UserId: *event.ActorUserID, DisplayName: event.ActorName,
			}
		}
		out = append(out, converted)
	}
	return blueprintsgen.ResourceLockHistoryResponse{Events: out}, nil
}
