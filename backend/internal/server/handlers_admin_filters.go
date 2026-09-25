package server

import (
	"fmt"
	"time"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Range validation shared by the admin list endpoints' advanced filters (#1133;
// reused by the team and project listings, #1138/#1143). It lives in the handler
// because the generated binding enforces neither `minimum` nor any cross-field
// rule, and there is no request-validator middleware — the same reason
// validateAdminSortEnum exists.

// validateAdminCountRange checks one <name>_min/<name>_max pair: each bound is
// non-negative and min <= max. A violation is a 400; a valid pair is returned as
// the repository's range (nil bounds stay open).
func validateAdminCountRange(name string, lower, upper *int64) (repositories.AdminCountRange, error) {
	if lower != nil && *lower < 0 {
		return repositories.AdminCountRange{}, apierrors.NewBadRequestError(
			fmt.Sprintf("%s_min must be non-negative, got %d", name, *lower))
	}
	if upper != nil && *upper < 0 {
		return repositories.AdminCountRange{}, apierrors.NewBadRequestError(
			fmt.Sprintf("%s_max must be non-negative, got %d", name, *upper))
	}
	if lower != nil && upper != nil && *lower > *upper {
		return repositories.AdminCountRange{}, apierrors.NewBadRequestError(
			fmt.Sprintf("%s_min (%d) must not exceed %s_max (%d)", name, *lower, name, *upper))
	}
	return repositories.AdminCountRange{Min: lower, Max: upper}, nil
}

// adminCountRangeParam binds one <name>_min/<name>_max query pair to the
// repository range it validates into.
type adminCountRangeParam struct {
	name         string
	lower, upper *int64
	dst          *repositories.AdminCountRange
}

// applyAdminCountRanges validates every pair with validateAdminCountRange and
// stores each valid range in its dst, stopping at the first 400.
func applyAdminCountRanges(params []adminCountRangeParam) error {
	for _, cr := range params {
		r, err := validateAdminCountRange(cr.name, cr.lower, cr.upper)
		if err != nil {
			return err
		}
		*cr.dst = r
	}
	return nil
}

// validateAdminTimeRange rejects an inverted <name>_from/<name>_to pair. The
// RFC 3339 format itself is already enforced by the generated binding.
func validateAdminTimeRange(name string, from, to *time.Time) error {
	if from != nil && to != nil && from.After(*to) {
		return apierrors.NewBadRequestError(
			fmt.Sprintf("%s_from must not be after %s_to", name, name))
	}
	return nil
}
