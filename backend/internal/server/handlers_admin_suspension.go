package server

import (
	"context"
	"errors"
	"net/http"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// Admin suspend/reactivate handlers (#454).

// SuspendAdminUser suspends a user, blocking every authentication entry point.
func (a *adminStrictServer) SuspendAdminUser(
	ctx context.Context, request admingen.SuspendAdminUserRequestObject,
) (admingen.SuspendAdminUserResponseObject, error) {
	targetID := request.Id.String()

	detail, err := a.s.container.AdminService().SuspendUser(
		ctx, a.actingAdminID(ctx), targetID, a.s.config.IsInstanceAdmin,
	)
	if err != nil {
		return nil, a.mapSuspensionError(err, "SuspendAdminUser")
	}
	if detail == nil {
		return nil, apierrors.NewResourceNotFoundError("user", "User not found")
	}

	a.recordSuspensionActivity(ctx, activities.ActivityTypeAdminUserSuspended, targetID, detail.Email)

	genDetail, convErr := toGenAdminUserDetail(detail)
	if convErr != nil {
		return nil, a.logConversionFailure("SuspendAdminUser", convErr)
	}
	return admingen.SuspendAdminUser200JSONResponse(genDetail), nil
}

// ReactivateAdminUser restores a suspended user's access.
func (a *adminStrictServer) ReactivateAdminUser(
	ctx context.Context, request admingen.ReactivateAdminUserRequestObject,
) (admingen.ReactivateAdminUserResponseObject, error) {
	targetID := request.Id.String()

	detail, err := a.s.container.AdminService().ReactivateUser(ctx, targetID)
	if err != nil {
		return nil, a.mapSuspensionError(err, "ReactivateAdminUser")
	}
	if detail == nil {
		return nil, apierrors.NewResourceNotFoundError("user", "User not found")
	}

	a.recordSuspensionActivity(ctx, activities.ActivityTypeAdminUserReactivated, targetID, detail.Email)

	genDetail, convErr := toGenAdminUserDetail(detail)
	if convErr != nil {
		return nil, a.logConversionFailure("ReactivateAdminUser", convErr)
	}
	return admingen.ReactivateAdminUser200JSONResponse(genDetail), nil
}

// actingAdminID returns the authenticated caller's user id. The admin surface
// sits behind instanceAdminMiddleware, so this is always populated here; the
// empty fallback simply means the self-suspension guard cannot match, which
// fails safe (the config-admin guard still applies).
func (a *adminStrictServer) actingAdminID(ctx context.Context) string {
	if userID, ok := ctx.Value(contextKeyUserID).(string); ok {
		return userID
	}
	return ""
}

// mapSuspensionError turns the service's guard errors into their HTTP shapes:
// both lockout guards are 409 Conflict, anything else is a logged 500.
func (a *adminStrictServer) mapSuspensionError(err error, handler string) error {
	var selfErr *services.ErrAdminSuspendSelf
	if errors.As(err, &selfErr) {
		return adminConflictError(selfErr.Error())
	}
	var adminErr *services.ErrAdminSuspendInstanceAdmin
	if errors.As(err, &adminErr) {
		return adminConflictError(adminErr.Error())
	}

	a.s.logger.With(
		"service", serverLogServiceName, "handler", handler, "error", err,
	).Error("Failed to change user suspension status")
	return apierrors.NewInternalError(adminMsgInternalError)
}

// logConversionFailure reports a domain→generated conversion failure as a 500.
func (a *adminStrictServer) logConversionFailure(handler string, err error) error {
	a.s.logger.With(
		"service", serverLogServiceName, "handler", handler, "error", err,
	).Error("Failed to convert admin user detail")
	return apierrors.NewInternalError(adminMsgInternalError)
}

// adminConflictError builds the 409 used by both lockout guards. There is no
// NewConflictError builder; CodeResourceConflict is the existing conflict code
// and NewResourceExistsError would misreport this as a duplicate.
func adminConflictError(detail string) *apierrors.APIError {
	return apierrors.NewAPIError(
		apierrors.CodeResourceConflict,
		apierrors.GetErrorTitle(apierrors.CodeResourceConflict),
		detail,
		http.StatusConflict,
	)
}

// recordSuspensionActivity writes the audit row for a lifecycle transition. The
// ACTING ADMIN is the activity's user_id and the affected account is the
// entity_id, so "who did this to whom" is answerable from the activities table.
//
// It calls the activity service directly rather than through ActivityRecorder,
// which requires an *http.Request the strict-server handler does not have. A
// recording failure must never fail the transition itself — the status change is
// already committed — so the error is logged and dropped.
func (a *adminStrictServer) recordSuspensionActivity(
	ctx context.Context, activityType, targetUserID, targetEmail string,
) {
	activityService := a.s.container.ActivityService()
	if activityService == nil {
		return
	}

	err := activityService.RecordResourceActivity(
		ctx,
		a.actingAdminID(ctx),
		activityType,
		activities.EntityTypeUser,
		&targetUserID,
		"Instance admin changed a user's account status",
		map[string]interface{}{
			"target_user_id": targetUserID,
			"target_email":   targetEmail,
			"new_status":     statusForActivity(activityType),
		},
	)
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName,
			"activity_type", activityType,
			"target_user_id", targetUserID,
			"error", err,
		).Error("Failed to record admin suspension activity")
	}
}

// statusForActivity maps the activity type back to the status it represents, so
// the recorded metadata states the resulting state explicitly.
func statusForActivity(activityType string) string {
	if activityType == activities.ActivityTypeAdminUserSuspended {
		return models.UserStatusSuspended
	}
	return models.UserStatusActive
}
