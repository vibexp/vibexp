package server

import (
	"context"
	"errors"

	"github.com/google/uuid"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// Admin user edit + guarded hard delete handlers (#455).

// UpdateAdminUser changes a user's display name. Bodies carrying any other field
// are rejected before this runs, by rejectUnknownAdminBodyFields.
func (a *adminStrictServer) UpdateAdminUser(
	ctx context.Context, request admingen.UpdateAdminUserRequestObject,
) (admingen.UpdateAdminUserResponseObject, error) {
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("Request body is required")
	}
	targetID := request.Id.String()

	detail, err := a.s.container.AdminService().UpdateUserName(ctx, targetID, request.Body.Name)
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "UpdateAdminUser", "error", err,
		).Error("Failed to update admin user")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	if detail == nil {
		return nil, apierrors.NewResourceNotFoundError("user", "User not found")
	}

	a.recordUserMutationActivity(ctx, activities.ActivityTypeAdminUserUpdated, targetID, detail.Email,
		map[string]interface{}{"new_name": detail.Name})

	genDetail, convErr := toGenAdminUserDetail(detail)
	if convErr != nil {
		return nil, a.logConversionFailure("UpdateAdminUser", convErr)
	}
	return admingen.UpdateAdminUser200JSONResponse(genDetail), nil
}

// DeleteAdminUser permanently removes a user, unless one of the guards refuses.
func (a *adminStrictServer) DeleteAdminUser(
	ctx context.Context, request admingen.DeleteAdminUserRequestObject,
) (admingen.DeleteAdminUserResponseObject, error) {
	targetID := request.Id.String()

	// Capture the email BEFORE the delete: afterwards there is no row to read it
	// from, and the audit row would name nobody.
	targetEmail := a.lookupEmailForAudit(ctx, targetID)

	deleted, err := a.s.container.AdminService().DeleteUser(
		ctx, a.actingAdminID(ctx), targetID, a.s.config.IsInstanceAdmin,
	)
	if err != nil {
		return nil, a.mapDeleteError(err)
	}
	if !deleted {
		return nil, apierrors.NewResourceNotFoundError("user", "User not found")
	}

	a.recordUserMutationActivity(ctx, activities.ActivityTypeAdminUserDeleted, targetID, targetEmail, nil)

	return admingen.DeleteAdminUser204Response{}, nil
}

// mapDeleteError turns the three refusal kinds into their HTTP shapes. All are
// 409, but a blocked delete carries the structured blocker list the SPA renders,
// so it uses the documented response object rather than a problem document.
func (a *adminStrictServer) mapDeleteError(err error) error {
	var blockedErr *services.ErrAdminDeleteBlocked
	if errors.As(err, &blockedErr) {
		return &adminDeleteBlockedError{blockers: blockedErr.Blockers, detail: blockedErr.Error()}
	}

	var selfErr *services.ErrAdminDeleteSelf
	if errors.As(err, &selfErr) {
		return adminConflictError(selfErr.Error())
	}
	var adminErr *services.ErrAdminDeleteInstanceAdmin
	if errors.As(err, &adminErr) {
		return adminConflictError(adminErr.Error())
	}

	a.s.logger.With(
		"service", serverLogServiceName, "handler", "DeleteAdminUser", "error", err,
	).Error("Failed to delete admin user")
	return apierrors.NewInternalError(adminMsgInternalError)
}

// lookupEmailForAudit best-effort reads the target's email so the audit row can
// name them after the row is gone. A failure must not block the delete, so it
// degrades to an empty string.
func (a *adminStrictServer) lookupEmailForAudit(ctx context.Context, targetID string) string {
	detail, err := a.s.container.AdminService().GetUserDetail(ctx, targetID)
	if err != nil || detail == nil {
		return ""
	}
	return detail.Email
}

// recordUserMutationActivity writes the audit row for an edit or a delete.
//
// The ACTING ADMIN is the row's user_id. This is not cosmetic: activities.user_id
// is ON DELETE CASCADE, so a row attributed to the deleted user would be removed
// by the very delete it records.
func (a *adminStrictServer) recordUserMutationActivity(
	ctx context.Context, activityType, targetUserID, targetEmail string, extra map[string]interface{},
) {
	activityService := a.s.container.ActivityService()
	if activityService == nil {
		return
	}

	metadata := map[string]interface{}{
		"target_user_id": targetUserID,
		"target_email":   targetEmail,
	}
	for k, v := range extra {
		metadata[k] = v
	}

	err := activityService.RecordResourceActivity(
		ctx,
		a.actingAdminID(ctx),
		activityType,
		activities.EntityTypeUser,
		&targetUserID,
		"Instance admin modified a user account",
		metadata,
	)
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName,
			"activity_type", activityType,
			"target_user_id", targetUserID,
			"error", err,
		).Error("Failed to record admin user mutation activity")
	}
}

// adminDeleteBlockedError carries a blocked delete through the strict server's
// error path while still rendering the documented 409 body. The generated
// response objects cannot be returned as errors, and returning it as a normal
// response would bypass adminResponseErrorHandler, so this implements error and
// is unwrapped by that handler.
type adminDeleteBlockedError struct {
	blockers []models.AdminDeleteBlocker
	detail   string
}

func (e *adminDeleteBlockedError) Error() string { return e.detail }

// toGenDeleteBlockedResponse renders the documented 409 body. The blockers array
// is built with make(..., 0) so it serializes as [] rather than null (#125).
func toGenDeleteBlockedResponse(e *adminDeleteBlockedError) admingen.AdminUserDeleteBlockedResponse {
	blockers := make([]admingen.AdminDeleteBlocker, 0, len(e.blockers))
	for _, b := range e.blockers {
		teamID, err := uuid.Parse(b.TeamID)
		if err != nil {
			// A non-UUID team id cannot be rendered, but dropping the whole
			// response would turn a safe refusal into a 500 that looks like the
			// delete might have happened. Skip the row; the refusal still stands.
			continue
		}
		blockers = append(blockers, admingen.AdminDeleteBlocker{
			TeamId:      teamID,
			TeamName:    b.TeamName,
			MemberCount: b.MemberCount,
		})
	}
	return admingen.AdminUserDeleteBlockedResponse{
		Message:  e.detail,
		Blockers: blockers,
	}
}
