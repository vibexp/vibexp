package services

import (
	"errors"
	"fmt"
	"strings"

	"github.com/vibexp/vibexp/internal/models"
)

// ErrInvalidStatus is returned by a create/update when the caller supplied a
// status outside the subset its resource type documents (#912). Handlers map it
// to 400.
//
// Enforcement lives in the SERVICE for the same reason ErrInvalidLabels does:
// the `validate:"oneof=..."` struct tags on these request models are inert --
// nothing calls validate.Struct on them -- and the generated request binder
// does not validate a body enum either. The REST handlers do hand-validate
// status field by field, but the six MCP write tools go straight to the
// services, so until now an MCP client could persist any string it liked into
// a status column and it would come back out on the wire as a value neither
// generated client has a union member for.
//
// The handler-side checks stay where they are: they answer with each domain's
// long-standing 400 body, which is published surface, and reaching the service
// is not required to reject a value the handler already knows is wrong.
var ErrInvalidStatus = errors.New("invalid status")

// validateStatus rejects a status the resource type's documented subset does
// not contain.
//
// An empty status is accepted, because on every one of these request models ""
// is the wire form of "not supplied" rather than a value: the create paths
// substitute the documented default and the update paths leave the stored
// status alone. What "" must never do is reach the column -- `status` is a
// REQUIRED response field constrained to an enum, so a blank one is a body no
// generated client can represent. That is the callers' job, and all four update
// paths now do it (`req.Status != nil && *req.Status != ""`).
func validateStatus(allowed []string, status string) error {
	if status == "" || models.IsAllowedStatus(allowed, status) {
		return nil
	}
	return fmt.Errorf("%w: must be one of: %s",
		ErrInvalidStatus, strings.Join(allowed, ", "))
}

// validateOptionalStatus is validateStatus for the update requests, whose Status
// is a *string meaning "supplied or not". A nil pointer is not a status change,
// so there is nothing to check.
func validateOptionalStatus(allowed []string, status *string) error {
	if status == nil {
		return nil
	}
	return validateStatus(allowed, *status)
}
