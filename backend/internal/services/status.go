package services

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/vibexp/vibexp/internal/models"
)

// ErrInvalidStatus is returned by a create, an update or a list when the caller
// supplied a status the resource's team does not have for that resource type:
// neither one of the type's system statuses nor one of the team's custom ones
// (#912, opened to custom statuses in #1266). The wrapped message lists the
// allowed values, and handlers map it to 400 with that message.
//
// Enforcement lives in the SERVICE, and only there. The generated request
// binder does not validate a status, in a body or in a query parameter, and
// the MCP tools go straight to the services -- so a check anywhere else is
// either a second copy that has to agree with this one or a path that skips
// it. The REST handlers used to carry such copies; they now only translate
// this error.
var ErrInvalidStatus = errors.New("invalid status")

// statusValidator checks a status against a team's catalog for one resource
// type. Its zero value, and any validator built without a catalog, knows the
// system statuses only and so rejects every custom one: it fails closed.
type statusValidator struct {
	catalog StatusCatalogServiceInterface
}

// Validate rejects a status that is not in the catalog of (teamID,
// resourceType). teamID must be the team the resource belongs to, never one
// the caller merely named: another team's custom status is not allowed here.
//
// An empty status is accepted, because on every one of these request models
// and list filters "" is the wire form of "not supplied" rather than a value:
// the create paths substitute the documented default, the update paths leave
// the stored status alone and the list paths apply no filter. What "" must
// never do is reach the column -- that is the callers' job, pinned by
// TestServicesTreatEmptyStatusAsUnchanged rather than asserted here.
//
// A system status is answered without touching the catalog, so the common
// write costs no query. Anything else costs one.
func (v statusValidator) Validate(ctx context.Context, teamID, resourceType, status string) error {
	if status == "" {
		return nil
	}
	allowed := models.SystemStatusesFor(resourceType)
	if slices.Contains(allowed, status) {
		return nil
	}
	if v.catalog != nil && teamID != "" {
		slugs, err := v.catalog.AllowedSlugs(ctx, teamID, resourceType)
		if err != nil {
			return fmt.Errorf("failed to load the status catalog: %w", err)
		}
		// AllowedSlugs is the system statuses followed by the team's own, which
		// is also the order the rejection lists them in.
		allowed = slugs
		if slices.Contains(allowed, status) {
			return nil
		}
	}
	return fmt.Errorf("%w: must be one of: %s",
		ErrInvalidStatus, strings.Join(allowed, ", "))
}

// ValidateOptional is Validate for a *string status, where nil means "not
// supplied": the update requests, and the memory create request and list
// filter.
func (v statusValidator) ValidateOptional(
	ctx context.Context, teamID, resourceType string, status *string,
) error {
	if status == nil {
		return nil
	}
	return v.Validate(ctx, teamID, resourceType, *status)
}
