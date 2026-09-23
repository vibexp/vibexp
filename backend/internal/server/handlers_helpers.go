package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-playground/validator/v10"

	apierrors "github.com/vibexp/vibexp/internal/errors"
)

// handlerErrorParams bundles the log and response fields for logHandlerError,
// the shared error helper behind the artifact/blueprint/project handlers.
type handlerErrorParams struct {
	handler    string
	userID     string
	projectID  string // included in log fields only when non-empty
	slug       string // included in log fields only when non-empty
	err        error
	logMsg     string
	errCode    string
	errMsg     string
	statusCode int
}

// logHandlerError logs a handler error with the standard structured fields and
// writes the error response.
func (s *Server) logHandlerError(w http.ResponseWriter, p handlerErrorParams) {
	fields := []any{
		"service", serverLogServiceName, "handler", p.handler,
		"user_id", p.userID, "error", fmt.Sprintf("%+v", p.err),
	}
	if p.projectID != "" {
		fields = append(fields, "project_id", p.projectID)
	}
	if p.slug != "" {
		fields = append(fields, "slug", p.slug)
	}
	s.logger.With(fields...).Error(p.logMsg)
	writeErrorResponse(w, nil, p.errCode, p.errMsg, p.statusCode)
}

// validate is the shared struct validator used across request handlers
// (support, search, agent, MCP). It lives here, not in any one feature's
// handler file, so removing a single feature never strands the shared singleton.
var validate = validator.New()

// validateTeamAccess validates that a user has access to a specific team
// This function should be used for single-resource operations where team_id is provided
// Returns error with generic message to prevent team enumeration attacks
func (s *Server) validateTeamAccess(ctx context.Context, userID, teamID string) error {
	// Validate team membership
	isMember, err := s.container.TeamService().IsUserMemberOfTeam(ctx, userID, teamID)
	if err != nil {
		// Generic error message to prevent information leakage
		return fmt.Errorf("access denied")
	}

	if !isMember {
		// Generic error message - don't distinguish between "not a member" vs "team doesn't exist"
		return fmt.Errorf("access denied")
	}

	return nil
}

// PaginationParams holds validated pagination parameters
type PaginationParams struct {
	Page  int
	Limit int
}

// Pagination bounds shared by every list endpoint (and, via
// normalizeSearchPagination, the MCP search tool). They are the spec's
// minimum/maximum for `page` and `limit` (`per_page` on search).
const (
	paginationDefaultPage  = 1
	paginationMaxPage      = 10000
	paginationDefaultLimit = 10
	paginationMaxLimit     = 100
)

// Messages naming the allowed range, returned verbatim as the 400 detail.
var (
	paginationMsgPageRange  = fmt.Sprintf("page must be between 1 and %d", paginationMaxPage)
	paginationMsgLimitRange = fmt.Sprintf("limit must be between 1 and %d", paginationMaxLimit)
)

// validatePaginationParams parses and validates pagination query parameters.
//
// An empty string means "not provided" and yields the default (page 1,
// limit 10). A provided value that is non-numeric or outside its bounds
// (page 1..10000, limit 1..100) is REJECTED with a 400 bad-request error
// naming the allowed range -- never silently replaced by the default, which
// used to turn `limit=200` into a clean-looking 10-item page (#1107).
func validatePaginationParams(pageStr, limitStr string) (PaginationParams, error) {
	page, err := parseBoundedInt(pageStr, paginationDefaultPage, paginationMaxPage, paginationMsgPageRange)
	if err != nil {
		return PaginationParams{}, err
	}

	limit, err := parseBoundedInt(limitStr, paginationDefaultLimit, paginationMaxLimit, paginationMsgLimitRange)
	if err != nil {
		return PaginationParams{}, err
	}

	return PaginationParams{
		Page:  page,
		Limit: limit,
	}, nil
}

// errorMessage returns the client-facing message for err: an APIError's
// Detail (its Error() prefixes the code), anything else verbatim. It is how
// the legacy chi handlers pass a validatePaginationParams error to
// writeErrorResponse, whose body carries it as `detail`.
func errorMessage(err error) string {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Detail
	}
	return err.Error()
}

// parseBoundedInt parses raw as an integer in [1, maxValue]; empty yields
// defaultValue, anything else is a bad-request error carrying rangeMsg.
func parseBoundedInt(raw string, defaultValue, maxValue int, rangeMsg string) (int, error) {
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, apierrors.NewBadRequestError(rangeMsg)
	}
	return checkBounded(value, maxValue, rangeMsg)
}

// checkBounded returns value when it lies in [1, maxValue], else a
// bad-request error carrying rangeMsg. It is the one range check behind both
// the query-string and the integer (search body / MCP) pagination paths.
func checkBounded(value, maxValue int, rangeMsg string) (int, error) {
	if value < 1 || value > maxValue {
		return 0, apierrors.NewBadRequestError(rangeMsg)
	}
	return value, nil
}
