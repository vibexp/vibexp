package server

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// Admin user creation handler (#462). Since #1436 the name is optional, the
// user can be added to teams, and the response says how its owner gets in.

// adminUserNameMaxLength is the spec's maxLength on the name, in characters.
const adminUserNameMaxLength = 255

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
	teams, err := toAdminTeamAssignments(request.Body.Teams)
	if err != nil {
		return nil, err
	}

	detail, err := a.s.container.AdminService().CreateUser(ctx, services.AdminUserCreateRequest{
		Email:       email,
		Name:        name,
		IDPProvider: request.Body.IdpProvider,
		Teams:       teams,
	})
	if err != nil {
		return nil, a.mapCreateUserError(err)
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

	// The account exists from here on; how its owner gets in never fails the
	// creation (an email that could not be sent is reported in setup).
	setup := a.s.container.AdminAccountService().SetUpNewUser(ctx, a.actingAdminID(ctx), detail.ID)
	if setup.Delivery != "none" {
		a.recordUserMutationActivity(ctx, activities.ActivityTypeAdminUserSetupLinkIssued, detail.ID, detail.Email,
			map[string]interface{}{"delivery": string(setup.Delivery)})
	}

	genDetail, convErr := toGenAdminUserDetail(detail)
	if convErr != nil {
		return nil, a.logConversionFailure("CreateAdminUser", convErr)
	}
	return admingen.CreateAdminUser201JSONResponse(toGenAdminUserCreated(genDetail, setup)), nil
}

// mapCreateUserError turns a CreateUser failure into its HTTP error: a bad
// team or an address off the allowlist is a 400, a taken address a 409, and
// anything else a logged 500.
func (a *adminStrictServer) mapCreateUserError(err error) error {
	var teamErr *services.ErrAdminUserTeamInvalid
	if errors.As(err, &teamErr) {
		return apierrors.NewValidationError("The user was not created: "+teamErr.Error(),
			[]apierrors.ValidationError{{
				Field:   fmt.Sprintf("teams[%d]", teamErr.Index),
				Message: teamErr.Reason,
				Code:    "team_invalid",
			}})
	}
	var takenErr *services.ErrAdminUserEmailTaken
	if errors.As(err, &takenErr) {
		return adminConflictError(takenErr.Error())
	}
	var notAllowedErr *services.ErrAdminUserEmailNotAllowed
	if errors.As(err, &notAllowedErr) {
		return apierrors.NewValidationError(
			"The user was not created: "+notAllowedErr.Error(),
			[]apierrors.ValidationError{{
				Field:   "email",
				Message: notAllowedErr.Error(),
				Code:    codeEmailNotAllowed,
			}})
	}
	a.s.logger.With(
		"service", serverLogServiceName, "handler", "CreateAdminUser", "error", err,
	).Error("Failed to create admin user")
	return apierrors.NewInternalError(adminMsgInternalError)
}

// maxAdminUserTeams mirrors the spec's maxItems on teams, which the generated
// binding does not enforce.
const maxAdminUserTeams = 50

// toAdminTeamAssignments maps the request's teams onto the service's. The
// generated binding does not enforce the role enum, so it is checked here.
func toAdminTeamAssignments(teams *[]admingen.AdminUserTeamAssignment) ([]services.AdminTeamAssignment, error) {
	if teams == nil {
		return nil, nil
	}
	if len(*teams) > maxAdminUserTeams {
		return nil, apierrors.NewValidationError("The user was not created: too many teams",
			[]apierrors.ValidationError{{
				Field: "teams", Message: fmt.Sprintf("at most %d teams", maxAdminUserTeams), Code: "too_many_teams",
			}})
	}
	out := make([]services.AdminTeamAssignment, 0, len(*teams))
	for i, t := range *teams {
		if !t.Role.Valid() {
			return nil, apierrors.NewValidationError("The user was not created: a team role is not valid",
				[]apierrors.ValidationError{{
					Field: fmt.Sprintf("teams[%d]", i), Message: "the role must be member or admin",
					Code: "team_invalid",
				}})
		}
		out = append(out, services.AdminTeamAssignment{
			TeamID: t.TeamId.String(), Role: models.TeamMemberRole(t.Role),
		})
	}
	return out, nil
}

// toGenAdminUserCreated adds the setup outcome to a created user's detail.
func toGenAdminUserCreated(d admingen.AdminUserDetail, setup services.AdminUserSetup) admingen.AdminUserCreated {
	out := admingen.AdminUserCreated{
		Id: d.Id, Email: d.Email, Name: d.Name, IdpProvider: d.IdpProvider, Status: admingen.AdminUserCreatedStatus(d.Status),
		CreatedAt: d.CreatedAt, Memberships: d.Memberships,
		SignInMethods: d.SignInMethods, HasPassword: d.HasPassword, PasswordDisabled: d.PasswordDisabled,
		PendingSetup: d.PendingSetup,
		Setup:        admingen.AdminUserSetup{Delivery: admingen.AdminUserSetupDelivery(setup.Delivery)},
	}
	if setup.LinkURL != "" {
		link := setup.LinkURL
		out.Setup.LinkUrl = &link
	}
	if !setup.ExpiresAt.IsZero() {
		expires := setup.ExpiresAt.UTC()
		out.Setup.ExpiresAt = &expires
	}
	if setup.Reason != "" {
		reason := admingen.AdminUserSetupReason(setup.Reason)
		out.Setup.Reason = &reason
	}
	return out
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
	email = models.NormalizeEmail(string(body.Email))
	if body.Name != nil {
		name = strings.TrimSpace(*body.Name)
	}
	if len([]rune(name)) > adminUserNameMaxLength {
		return "", "", apierrors.NewBadRequestError("name is too long")
	}

	if email == "" {
		return "", "", apierrors.NewBadRequestError("email is required")
	}
	if _, parseErr := mail.ParseAddress(email); parseErr != nil {
		return "", "", apierrors.NewBadRequestError("email is not a valid address")
	}
	return email, name, nil
}
