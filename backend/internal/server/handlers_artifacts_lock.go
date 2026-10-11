package server

import (
	"context"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	artifactsgen "github.com/vibexp/vibexp/internal/server/gen/artifacts"
)

// The artifact lock operations (issue #1288, epic #1285). The rules, the
// activity record and the error mapping are shared with prompts and
// blueprints in handlers_resource_lock.go; this file resolves the artifact and
// converts the result.

// LockArtifact locks an artifact for the caller and returns the updated artifact.
func (as *artifactsStrictServer) LockArtifact(
	ctx context.Context, request artifactsgen.LockArtifactRequestObject,
) (artifactsgen.LockArtifactResponseObject, error) {
	// The generated decoder enforces neither `required` nor `minLength`, so a
	// missing comment arrives as "" and the service rejects it.
	comment := ""
	if request.Body != nil {
		comment = request.Body.Comment
	}

	body, err := as.changeArtifactLock(
		ctx, "LockArtifact", request.TeamId.String(), request.ProjectId.String(), request.Slug,
		func(userID string, target lockTarget) error {
			return as.s.lockResource(ctx, userID, target, comment)
		})
	if err != nil {
		return nil, err
	}
	return artifactsgen.LockArtifact200JSONResponse(body), nil
}

// UnlockArtifact releases an artifact's lock and returns the updated artifact.
func (as *artifactsStrictServer) UnlockArtifact(
	ctx context.Context, request artifactsgen.UnlockArtifactRequestObject,
) (artifactsgen.UnlockArtifactResponseObject, error) {
	body, err := as.changeArtifactLock(
		ctx, "UnlockArtifact", request.TeamId.String(), request.ProjectId.String(), request.Slug,
		func(userID string, target lockTarget) error {
			return as.s.unlockResource(ctx, userID, target)
		})
	if err != nil {
		return nil, err
	}
	return artifactsgen.UnlockArtifact200JSONResponse(body), nil
}

// GetArtifactLockHistory returns an artifact's lock and unlock events, newest first.
func (as *artifactsStrictServer) GetArtifactLockHistory(
	ctx context.Context, request artifactsgen.GetArtifactLockHistoryRequestObject,
) (artifactsgen.GetArtifactLockHistoryResponseObject, error) {
	userID, target, err := as.artifactLockTarget(ctx, request.TeamId.String(), request.ProjectId.String(), request.Slug)
	if err != nil {
		return nil, err
	}

	events, err := as.s.resourceLockHistory(ctx, userID, target)
	if err != nil {
		return nil, err
	}

	body, err := toGenArtifactLockHistory(events)
	if err != nil {
		as.s.logger.With("error", err, "operation", "GetArtifactLockHistory").
			Error("Artifacts response conversion failed")
		return nil, apierrors.NewInternalError(artifactsMsgInternalError)
	}
	return artifactsgen.GetArtifactLockHistory200JSONResponse(body), nil
}

// artifactLockTarget resolves the artifact a lock operation addresses by (project_id, slug).
func (as *artifactsStrictServer) artifactLockTarget(
	ctx context.Context, teamID, projectID, slug string,
) (string, lockTarget, error) {
	userID, err := artifactsUserID(ctx)
	if err != nil {
		return "", lockTarget{}, err
	}
	artifact, err := as.loadArtifact(userID, teamID, projectID, slug)
	if err != nil {
		return "", lockTarget{}, err
	}
	return userID, lockTarget{
		resourceType: models.RelationResourceTypeArtifact,
		teamID:       teamID,
		resourceID:   artifact.ID,
		slug:         artifact.Slug,
	}, nil
}

// changeArtifactLock is the shared body of LockArtifact and UnlockArtifact: resolve
// the artifact, apply the change, then read the artifact again so the response
// carries the lock state the change left behind.
func (as *artifactsStrictServer) changeArtifactLock(
	ctx context.Context, op, teamID, projectID, slug string, change func(userID string, target lockTarget) error,
) (artifactsgen.Artifact, error) {
	userID, target, err := as.artifactLockTarget(ctx, teamID, projectID, slug)
	if err != nil {
		return artifactsgen.Artifact{}, err
	}
	if err = change(userID, target); err != nil {
		return artifactsgen.Artifact{}, err
	}

	artifact, err := as.loadArtifact(userID, teamID, projectID, slug)
	if err != nil {
		return artifactsgen.Artifact{}, err
	}
	return as.artifactDetailBody(ctx, op, userID, teamID, artifact)
}

// toGenArtifactLock converts the lock an artifact carries. Nil stays nil, which the
// generated field -- required+nullable, hence no omitempty -- renders as
// `"lock": null`.
func toGenArtifactLock(src *models.ResourceLock) *artifactsgen.ResourceLock {
	if src == nil {
		return nil
	}
	return &artifactsgen.ResourceLock{
		LockedBy: artifactsgen.ResourceLockActor{UserId: src.LockedBy, DisplayName: src.LockedByName},
		Comment:  src.Comment,
		LockedAt: src.LockedAt,
	}
}

// toGenArtifactLockHistory converts an artifact's lock history.
//
// `make(..., 0, ...)` is what guarantees the required `events` array
// serializes as `[]` and never `null`: generated strict-server types cannot use
// the models.JSONArray shim (ResourceLockHistoryResponse is registered in
// adHocRequiredArrayAllowlist for exactly that reason).
func toGenArtifactLockHistory(
	events []*models.ResourceLockEvent,
) (artifactsgen.ResourceLockHistoryResponse, error) {
	out := make([]artifactsgen.ResourceLockEvent, 0, len(events))
	for _, event := range events {
		id, err := artifactUUID("lock event id", event.ID)
		if err != nil {
			return artifactsgen.ResourceLockHistoryResponse{}, err
		}
		converted := artifactsgen.ResourceLockEvent{
			Id:        id,
			Action:    artifactsgen.ResourceLockEventAction(event.Action),
			Comment:   event.Comment,
			CreatedAt: event.CreatedAt,
		}
		// The actor is null once the account is deleted (actor_user_id is
		// ON DELETE SET NULL).
		if event.ActorUserID != nil {
			converted.Actor = &artifactsgen.ResourceLockActor{
				UserId: *event.ActorUserID, DisplayName: event.ActorName,
			}
		}
		out = append(out, converted)
	}
	return artifactsgen.ResourceLockHistoryResponse{Events: out}, nil
}
