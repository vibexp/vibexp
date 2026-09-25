package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"time"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// Admin CSV exports (#1149). Each export binds its params onto the SAME
// generated list params, through the SAME toAdmin*Filters validator, onto the
// SAME repository WHERE/ORDER BY as its listing, so an export can never
// disagree with the list. The CSV is streamed through an io.Pipe whose reader
// is the generated text/csv response body: the set is never buffered.

// Default sort of every admin listing (the spec's defaults, and the repository
// fallback), recorded in the export activity when the query did not set one.
const (
	adminExportDefaultSortBy    = "created_at"
	adminExportDefaultSortOrder = "desc"
)

// adminExportStreamError marks a failure AFTER the 200 and its headers went out.
// adminResponseErrorHandler aborts the connection for it instead of appending a
// problem document to a half-written CSV.
type adminExportStreamError struct {
	err error
}

func (e *adminExportStreamError) Error() string {
	return "admin export stream failed: " + e.err.Error()
}
func (e *adminExportStreamError) Unwrap() error { return e.err }

// adminExportParamsToList copies every field of a generated export params
// struct onto the matching field of its list params struct. The export op's
// params are the list op's minus page/limit (pinned by the spec-parity test),
// and each enum field converts between two named string types. A field with no
// convertible counterpart is a programming error, reported rather than dropped.
func adminExportParamsToList[E, L any](export E) (L, error) {
	var list L
	src := reflect.ValueOf(export)
	dst := reflect.ValueOf(&list).Elem()
	for i := range src.NumField() {
		name := src.Type().Field(i).Name
		field := dst.FieldByName(name)
		if !field.IsValid() || !src.Field(i).Type().ConvertibleTo(field.Type()) {
			return list, fmt.Errorf("export param %s has no counterpart in %T", name, list)
		}
		field.Set(src.Field(i).Convert(field.Type()))
	}
	return list, nil
}

// adminExportFilterSummary renders the params the request actually set as
// {query-param name: value}, for the export activity. The generated params'
// json tags are the query-param names and omit unset (nil) fields.
func adminExportFilterSummary(params any) map[string]interface{} {
	summary := map[string]interface{}{}
	raw, err := json.Marshal(params)
	if err != nil {
		return summary
	}
	if err := json.Unmarshal(raw, &summary); err != nil {
		return map[string]interface{}{}
	}
	delete(summary, "sort_by")
	delete(summary, "sort_order")
	return summary
}

// adminExportFilename is the attachment name, dated in UTC.
func adminExportFilename(list string, now time.Time) string {
	return fmt.Sprintf(`attachment; filename="admin-%s-%s.csv"`, list, now.UTC().Format("20060102"))
}

// adminExportRequest is what an export handler hands to startAdminExport.
type adminExportRequest struct {
	list         string // users | teams | projects
	activityType string
	params       any
	sortBy       string
	sortOrder    string
}

// startAdminExport starts writing exp into a pipe and returns its reader. The
// writer goroutine closes the pipe with an adminExportStreamError on failure
// (the response visitor's io.Copy surfaces it to adminResponseErrorHandler),
// and records the export activity once the stream completed, before the EOF.
// When the client goes away, the visitor closes the reader, the next write
// fails, and the repository closes its rows.
func (a *adminStrictServer) startAdminExport(
	ctx context.Context, req adminExportRequest, exp services.AdminExport,
) *io.PipeReader {
	pr, pw := io.Pipe()
	go func() {
		rows, err := exp.WriteCSV(ctx, pw)
		var closeWith error
		if err != nil {
			closeWith = &adminExportStreamError{err: err}
		} else {
			// Record before closing: every byte has already been consumed (pipe
			// writes block until read), and the reader's EOF then also means the
			// activity is in place.
			a.recordExportActivity(context.WithoutCancel(ctx), req, exp, rows)
		}
		// CloseWithError(nil) is Close; either way it only ever returns nil.
		if closeErr := pw.CloseWithError(closeWith); closeErr != nil {
			a.s.logger.With("error", closeErr).Error("Failed to close admin export pipe")
		}
	}()
	return pr
}

// recordExportActivity records the admin_<list>_exported activity for the
// acting admin. A failure is logged, never surfaced: the export already went out.
func (a *adminStrictServer) recordExportActivity(
	ctx context.Context, req adminExportRequest, exp services.AdminExport, rows int,
) {
	activityService := a.s.container.ActivityService()
	if activityService == nil {
		return
	}

	sortBy, sortOrder := req.sortBy, req.sortOrder
	if sortBy == "" {
		sortBy = adminExportDefaultSortBy
	}
	if sortOrder == "" {
		sortOrder = adminExportDefaultSortOrder
	}

	err := activityService.RecordResourceActivity(
		ctx,
		a.actingAdminID(ctx),
		req.activityType,
		activities.EntityTypeSystem,
		nil,
		"Instance admin exported the "+req.list+" list",
		map[string]interface{}{
			"list":        req.list,
			"filters":     adminExportFilterSummary(req.params),
			"sort_by":     sortBy,
			"sort_order":  sortOrder,
			"row_count":   rows,
			"total_count": exp.TotalCount,
			"truncated":   exp.Truncated,
		},
	)
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "activity_type", req.activityType, "error", err,
		).Error("Failed to record admin export activity")
	}
}

// logExportFailure reports a failure to prepare an export as a 500.
func (a *adminStrictServer) logExportFailure(handler string, err error) error {
	a.s.logger.With(
		"service", serverLogServiceName, "handler", handler, "error", err,
	).Error("Failed to prepare admin export")
	return apierrors.NewInternalError(adminMsgInternalError)
}

// ExportAdminUsers streams the filtered admin user listing as CSV.
func (a *adminStrictServer) ExportAdminUsers(
	ctx context.Context, request admingen.ExportAdminUsersRequestObject,
) (admingen.ExportAdminUsersResponseObject, error) {
	params, err := adminExportParamsToList[admingen.ExportAdminUsersParams, admingen.ListAdminUsersParams](
		request.Params)
	if err != nil {
		return nil, a.logExportFailure("ExportAdminUsers", err)
	}
	filters, err := toAdminUserFilters(params)
	if err != nil {
		return nil, err
	}

	exp, err := a.s.container.AdminService().ExportUsers(ctx, filters)
	if err != nil {
		return nil, a.logExportFailure("ExportAdminUsers", err)
	}

	body := a.startAdminExport(ctx, adminExportRequest{
		list: "users", activityType: activities.ActivityTypeAdminUsersExported,
		params: request.Params, sortBy: filters.SortBy, sortOrder: filters.SortOrder,
	}, exp)
	return admingen.ExportAdminUsers200TextcsvResponse{
		Body: body,
		Headers: admingen.ExportAdminUsers200ResponseHeaders{
			ContentDisposition: adminExportFilename("users", time.Now()),
			XExportTotalCount:  int64(exp.TotalCount),
			XExportTruncated:   exp.Truncated,
		},
	}, nil
}

// ExportAdminTeams streams the filtered admin team listing as CSV.
func (a *adminStrictServer) ExportAdminTeams(
	ctx context.Context, request admingen.ExportAdminTeamsRequestObject,
) (admingen.ExportAdminTeamsResponseObject, error) {
	params, err := adminExportParamsToList[admingen.ExportAdminTeamsParams, admingen.ListAdminTeamsParams](
		request.Params)
	if err != nil {
		return nil, a.logExportFailure("ExportAdminTeams", err)
	}
	filters, err := toAdminTeamFilters(params)
	if err != nil {
		return nil, err
	}

	exp, err := a.s.container.AdminService().ExportTeams(ctx, filters)
	if err != nil {
		return nil, a.logExportFailure("ExportAdminTeams", err)
	}

	body := a.startAdminExport(ctx, adminExportRequest{
		list: "teams", activityType: activities.ActivityTypeAdminTeamsExported,
		params: request.Params, sortBy: filters.SortBy, sortOrder: filters.SortOrder,
	}, exp)
	return admingen.ExportAdminTeams200TextcsvResponse{
		Body: body,
		Headers: admingen.ExportAdminTeams200ResponseHeaders{
			ContentDisposition: adminExportFilename("teams", time.Now()),
			XExportTotalCount:  int64(exp.TotalCount),
			XExportTruncated:   exp.Truncated,
		},
	}, nil
}

// ExportAdminProjects streams the filtered admin project listing as CSV.
func (a *adminStrictServer) ExportAdminProjects(
	ctx context.Context, request admingen.ExportAdminProjectsRequestObject,
) (admingen.ExportAdminProjectsResponseObject, error) {
	params, err := adminExportParamsToList[admingen.ExportAdminProjectsParams, admingen.ListAdminProjectsParams](
		request.Params)
	if err != nil {
		return nil, a.logExportFailure("ExportAdminProjects", err)
	}
	filters, err := toAdminProjectFilters(params)
	if err != nil {
		return nil, err
	}

	exp, err := a.s.container.AdminService().ExportProjects(ctx, filters)
	if err != nil {
		return nil, a.logExportFailure("ExportAdminProjects", err)
	}

	body := a.startAdminExport(ctx, adminExportRequest{
		list: "projects", activityType: activities.ActivityTypeAdminProjectsExported,
		params: request.Params, sortBy: filters.SortBy, sortOrder: filters.SortOrder,
	}, exp)
	return admingen.ExportAdminProjects200TextcsvResponse{
		Body: body,
		Headers: admingen.ExportAdminProjects200ResponseHeaders{
			ContentDisposition: adminExportFilename("projects", time.Now()),
			XExportTotalCount:  int64(exp.TotalCount),
			XExportTruncated:   exp.Truncated,
		},
	}, nil
}
