package server

import (
	stderrors "errors"
	"net/http"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/services"
)

// providerPermissionMessage is what a caller without the right role is told when
// they try to change (or probe with) provider settings. Provider rows hold
// encrypted API keys and decide where a team's embedding/model traffic goes, so
// they are owner/admin surface (#464).
const providerPermissionMessage = "You do not have permission to manage this team's provider settings."

// writeIfPermissionDenied maps a services.ErrPermissionDenied to 403 and reports
// whether it handled the error. Provider handlers call it before their own
// error mapping so an authorization failure is never reported as a generic
// "create failed" 500 — the caller needs to know it is a role problem, and the
// operator needs the distinction in logs.
func writeIfPermissionDenied(w http.ResponseWriter, r *http.Request, err error) bool {
	if !stderrors.Is(err, services.ErrPermissionDenied) {
		return false
	}
	errors.WriteJSONError(w, r, errors.NewForbiddenError(providerPermissionMessage))
	return true
}

// requireProviderManagePermission gates the embedding maintenance actions that
// do NOT pass through a provider service and so cannot be authorized there:
// reprocess (enqueues a team-wide re-embed, spending the team's provider
// budget) and clear-embeddings (deletes every embedding the team has). Both are
// destructive and were reachable by any team member (#464).
//
// Handler-level authorization is the established pattern for this case — see
// handlers_relations.go — since there is no team-scoped service method to hang
// the check on.
func (s *Server) requireProviderManagePermission(
	w http.ResponseWriter, r *http.Request, userID, teamID string,
) bool {
	if err := s.container.AuthorizationService().Can(
		r.Context(), userID, teamID, authz.TeamUpdate,
	); err != nil {
		errors.WriteJSONError(w, r, errors.NewForbiddenError(providerPermissionMessage))
		return false
	}
	return true
}
