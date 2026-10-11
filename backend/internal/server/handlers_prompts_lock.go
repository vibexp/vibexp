package server

import (
	"context"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	promptsgen "github.com/vibexp/vibexp/internal/server/gen/prompts"
)

// The prompt lock operations (issue #1288, epic #1285). The rules, the
// activity record and the error mapping are shared with blueprints and
// artifacts in handlers_resource_lock.go; this file resolves the prompt and
// converts the result.

// LockPrompt locks a prompt for the caller and returns the updated prompt.
func (ps *promptsStrictServer) LockPrompt(
	ctx context.Context, request promptsgen.LockPromptRequestObject,
) (promptsgen.LockPromptResponseObject, error) {
	// The generated decoder enforces neither `required` nor `minLength`, so a
	// missing comment arrives as "" and the service rejects it.
	comment := ""
	if request.Body != nil {
		comment = request.Body.Comment
	}

	body, err := ps.changePromptLock(ctx, "LockPrompt", request.TeamId.String(), request.Slug,
		func(userID string, target lockTarget) error {
			return ps.s.lockResource(ctx, userID, target, comment)
		})
	if err != nil {
		return nil, err
	}
	return promptsgen.LockPrompt200JSONResponse(body), nil
}

// UnlockPrompt releases a prompt's lock and returns the updated prompt.
func (ps *promptsStrictServer) UnlockPrompt(
	ctx context.Context, request promptsgen.UnlockPromptRequestObject,
) (promptsgen.UnlockPromptResponseObject, error) {
	body, err := ps.changePromptLock(ctx, "UnlockPrompt", request.TeamId.String(), request.Slug,
		func(userID string, target lockTarget) error {
			return ps.s.unlockResource(ctx, userID, target)
		})
	if err != nil {
		return nil, err
	}
	return promptsgen.UnlockPrompt200JSONResponse(body), nil
}

// GetPromptLockHistory returns a prompt's lock and unlock events, newest first.
func (ps *promptsStrictServer) GetPromptLockHistory(
	ctx context.Context, request promptsgen.GetPromptLockHistoryRequestObject,
) (promptsgen.GetPromptLockHistoryResponseObject, error) {
	userID, target, err := ps.promptLockTarget(ctx, request.TeamId.String(), request.Slug)
	if err != nil {
		return nil, err
	}

	events, err := ps.s.resourceLockHistory(ctx, userID, target)
	if err != nil {
		return nil, err
	}

	body, err := toGenPromptLockHistory(events)
	if err != nil {
		ps.s.logger.With("error", err, "operation", "GetPromptLockHistory").
			Error("Prompts response conversion failed")
		return nil, apierrors.NewInternalError(promptsMsgInternalError)
	}
	return promptsgen.GetPromptLockHistory200JSONResponse(body), nil
}

// promptLockTarget resolves the prompt a lock operation addresses by slug.
func (ps *promptsStrictServer) promptLockTarget(
	ctx context.Context, teamID, slug string,
) (string, lockTarget, error) {
	userID, err := promptsUserID(ctx)
	if err != nil {
		return "", lockTarget{}, err
	}
	prompt, err := ps.loadPrompt(userID, teamID, slug)
	if err != nil {
		return "", lockTarget{}, err
	}
	return userID, lockTarget{
		resourceType: models.RelationResourceTypePrompt,
		teamID:       teamID,
		resourceID:   prompt.ID,
		slug:         prompt.Slug,
	}, nil
}

// changePromptLock is the shared body of LockPrompt and UnlockPrompt: resolve
// the prompt, apply the change, then read the prompt again so the response
// carries the lock state the change left behind.
func (ps *promptsStrictServer) changePromptLock(
	ctx context.Context, op, teamID, slug string, change func(userID string, target lockTarget) error,
) (promptsgen.Prompt, error) {
	userID, target, err := ps.promptLockTarget(ctx, teamID, slug)
	if err != nil {
		return promptsgen.Prompt{}, err
	}
	if err = change(userID, target); err != nil {
		return promptsgen.Prompt{}, err
	}

	prompt, err := ps.loadPrompt(userID, teamID, slug)
	if err != nil {
		return promptsgen.Prompt{}, err
	}
	return ps.promptDetailBody(ctx, op, userID, teamID, prompt)
}

// toGenPromptLock converts the lock a prompt carries. Nil stays nil, which the
// generated field -- required+nullable, hence no omitempty -- renders as
// `"lock": null`.
func toGenPromptLock(src *models.ResourceLock) *promptsgen.ResourceLock {
	if src == nil {
		return nil
	}
	return &promptsgen.ResourceLock{
		LockedBy: promptsgen.ResourceLockActor{UserId: src.LockedBy, DisplayName: src.LockedByName},
		Comment:  src.Comment,
		LockedAt: src.LockedAt,
	}
}

// toGenPromptLockHistory converts a prompt's lock history.
//
// `make(..., 0, ...)` is what guarantees the required `events` array
// serializes as `[]` and never `null`: generated strict-server types cannot use
// the models.JSONArray shim (ResourceLockHistoryResponse is registered in
// adHocRequiredArrayAllowlist for exactly that reason).
func toGenPromptLockHistory(
	events []*models.ResourceLockEvent,
) (promptsgen.ResourceLockHistoryResponse, error) {
	out := make([]promptsgen.ResourceLockEvent, 0, len(events))
	for _, event := range events {
		id, err := promptUUID("lock event id", event.ID)
		if err != nil {
			return promptsgen.ResourceLockHistoryResponse{}, err
		}
		converted := promptsgen.ResourceLockEvent{
			Id:        id,
			Action:    promptsgen.ResourceLockEventAction(event.Action),
			Comment:   event.Comment,
			CreatedAt: event.CreatedAt,
		}
		// The actor is null once the account is deleted (actor_user_id is
		// ON DELETE SET NULL).
		if event.ActorUserID != nil {
			converted.Actor = &promptsgen.ResourceLockActor{
				UserId: *event.ActorUserID, DisplayName: event.ActorName,
			}
		}
		out = append(out, converted)
	}
	return promptsgen.ResourceLockHistoryResponse{Events: out}, nil
}
