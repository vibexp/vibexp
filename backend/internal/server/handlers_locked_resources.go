package server

import (
	"errors"
	"net/http"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/repositories"
)

// The detail sentences of the 423 an indirect delete or move answers with when
// locked resources stand in its way (issue #1290). Each says what was refused
// and what to do; the resources themselves are in metadata.locked_resources.
const (
	lockedMsgProject = "This project contains locked resources, so it cannot be deleted or migrated. " +
		"Unlock them first."
	lockedMsgType = "A locked artifact uses this type, so the type cannot be deleted. " +
		"Unlock the artifact first."
	lockedMsgStatus = "A locked resource carries this status, so the status cannot be deleted. " +
		"Unlock the resource first."
	lockedMsgDefault = "Locked resources prevent this change. Unlock them first."
)

// lockedResourcesProblem returns the documented 423 for a
// repositories.LockedResourcesError anywhere in err's chain, or nil when err is
// not one. The four guarded operations -- delete project, migrate project,
// delete type, delete status -- all answer through it, so they agree on the
// code and on the shape of the list.
func lockedResourcesProblem(err error) *apierrors.APIError {
	var locked *repositories.LockedResourcesError
	if !errors.As(err, &locked) {
		return nil
	}

	resources := make([]apierrors.LockedResource, 0, len(locked.Resources))
	for _, ref := range locked.Resources {
		resources = append(resources, apierrors.LockedResource{
			ResourceType: ref.ResourceType,
			ID:           ref.ID,
			Slug:         ref.Slug,
			Title:        ref.Title,
		})
	}
	return apierrors.NewHasLockedResourcesError(lockedResourcesDetail(locked.Reason), resources)
}

// lockedResourcesDetail picks the sentence for the reason a guard refused.
func lockedResourcesDetail(reason error) string {
	switch {
	case errors.Is(reason, repositories.ErrProjectHasLockedResources):
		return lockedMsgProject
	case errors.Is(reason, repositories.ErrTypeUsedByLockedResource):
		return lockedMsgType
	case errors.Is(reason, repositories.ErrTeamStatusUsedByLockedResource):
		return lockedMsgStatus
	}
	return lockedMsgDefault
}

// writeLockedResources answers a refusal by a locked-resource guard with its
// 423 and reports whether it handled err. The legacy project handlers call it
// before their own error mapping, which would turn the refusal into a 500.
func writeLockedResources(w http.ResponseWriter, r *http.Request, err error) bool {
	apiErr := lockedResourcesProblem(err)
	if apiErr == nil {
		return false
	}
	apierrors.WriteJSONError(w, r, apiErr)
	return true
}
