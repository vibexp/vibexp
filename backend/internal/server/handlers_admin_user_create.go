package server

import (
	"context"
	"errors"
	"net/mail"
	"strings"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// Admin user creation handler (#462).

// CreateAdminUser creates a user and returns 201 with the created account.
//
// The personal workspace and default project are provisioned asynchronously by
// the `user.created` listener, so they may not exist yet when this returns —
// identical to self-signup, and stated in the operation description.
func (a *adminStrictServer) CreateAdminUser(
	ctx context.Context, request admingen.CreateAdminUserRequestObject,
) (admingen.CreateAdminUserResponseObject, error) {
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("Request body is required")
	}

	email, name, err := validateAdminUserCreate(*request.Body)
	if err != nil {
		return nil, err
	}

	detail, err := a.s.container.AdminService().CreateUser(ctx, services.AdminUserCreateRequest{
		Email:       email,
		Name:        name,
		IDPProvider: request.Body.IdpProvider,
	})
	if err != nil {
		var takenErr *services.ErrAdminUserEmailTaken
		if errors.As(err, &takenErr) {
			return nil, adminConflictError(takenErr.Error())
		}
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "CreateAdminUser", "error", err,
		).Error("Failed to create admin user")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	if detail == nil {
		// The row was written and the event published, but the follow-up read
		// found nothing. That is a server fault, not a 404: reporting "not found"
		// for something we just created would be actively misleading.
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "CreateAdminUser",
		).Error("Created user could not be read back")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}

	a.recordUserMutationActivity(ctx, activities.ActivityTypeAdminUserCreated, detail.ID, detail.Email, nil)

	genDetail, convErr := toGenAdminUserDetail(detail)
	if convErr != nil {
		return nil, a.logConversionFailure("CreateAdminUser", convErr)
	}
	return admingen.CreateAdminUser201JSONResponse(genDetail), nil
}

// validateAdminUserCreate trims the body and checks what the generated binding
// does not.
//
// Division of labour, verified rather than assumed: `format: email` IS enforced —
// oapi-codegen types the field as openapi_types.Email, whose UnmarshalJSON
// applies a regex, so a malformed or empty address is already a 400 before this
// runs. `minLength: 1` on `name` is NOT enforced, so a blank or whitespace-only
// name would otherwise create an account with no display name. The email checks
// below are kept as defence in depth: they cost nothing and still hold if the
// spec ever drops the format.
func validateAdminUserCreate(body admingen.AdminUserCreateRequest) (email, name string, err error) {
	email = strings.ToLower(strings.TrimSpace(string(body.Email)))
	name = strings.TrimSpace(body.Name)

	if email == "" {
		return "", "", apierrors.NewBadRequestError("email is required")
	}
	if _, parseErr := mail.ParseAddress(email); parseErr != nil {
		return "", "", apierrors.NewBadRequestError("email is not a valid address")
	}
	if name == "" {
		return "", "", apierrors.NewBadRequestError("name is required")
	}
	return email, name, nil
}
