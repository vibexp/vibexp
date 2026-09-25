package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/repositories"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/server/openapispec"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// adminExportCase describes one of the three export ops, so every behaviour is
// pinned for all of them.
type adminExportCase struct {
	list         string
	method       string
	activityType string
	filters      any // the repository filters the query below must produce
	query        string
}

func adminExportCases() []adminExportCase {
	return []adminExportCase{
		{
			list: "users", method: "ExportUsers", activityType: activities.ActivityTypeAdminUsersExported,
			query:   "search=ada&status=active&sort_by=email&sort_order=asc",
			filters: repositories.AdminUserFilters{Search: strPtr("ada"), Status: strPtr("active"), SortBy: "email", SortOrder: "asc"},
		},
		{
			list: "teams", method: "ExportTeams", activityType: activities.ActivityTypeAdminTeamsExported,
			query:   "search=acme&owner_email=owner%40example.com&sort_by=name&sort_order=asc",
			filters: repositories.AdminTeamFilters{Search: strPtr("acme"), OwnerEmail: strPtr("owner@example.com"), SortBy: "name", SortOrder: "asc"},
		},
		{
			list: "projects", method: "ExportProjects", activityType: activities.ActivityTypeAdminProjectsExported,
			query:   "search=plat&sort_by=name&sort_order=asc",
			filters: repositories.AdminProjectFilters{Search: strPtr("plat"), SortBy: "name", SortOrder: "asc"},
		},
	}
}

// fakeAdminExport returns an export whose WriteCSV writes body and reports
// rows, or fails with streamErr after writing body.
func fakeAdminExport(total int, truncated bool, body string, rows int, streamErr error) services.AdminExport {
	return services.AdminExport{
		TotalCount: total,
		Truncated:  truncated,
		WriteCSV: func(_ context.Context, w io.Writer) (int, error) {
			if _, err := io.WriteString(w, body); err != nil {
				return 0, err
			}
			return rows, streamErr
		},
	}
}

func TestExportAdmin_StreamsCSVWithHeadersAndRecordsActivity(t *testing.T) {
	for _, tc := range adminExportCases() {
		t.Run(tc.list, func(t *testing.T) {
			body := "id,name\r\n1,A\r\n2,B\r\n"
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			mockAdmin.On(tc.method, mock.Anything, tc.filters).
				Return(fakeAdminExport(7, true, body, 2, nil), nil).Once()

			actingID := uuid.NewString()
			activitySvc := &MockActivityService{}
			activitySvc.On("RecordResourceActivity",
				mock.Anything, actingID, tc.activityType, activities.EntityTypeSystem, (*string)(nil),
				mock.Anything,
				mock.MatchedBy(func(md map[string]interface{}) bool {
					filters, ok := md["filters"].(map[string]interface{})
					return ok && md["list"] == tc.list && md["row_count"] == 2 && md["total_count"] == 7 &&
						md["truncated"] == true && md["sort_by"] != "" && md["sort_order"] == "asc" &&
						filters["search"] != nil && filters["sort_by"] == nil
				}),
			).Return(nil).Once()

			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
				adminService: mockAdmin, activityService: activitySvc,
			})
			req := httptest.NewRequest("GET", "/api/v1/admin/"+tc.list+"/export?"+tc.query, nil)
			req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, actingID))
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			assert.Equal(t, "text/csv", rr.Header().Get("Content-Type"))
			assert.Equal(t,
				fmt.Sprintf(`attachment; filename="admin-%s-%s.csv"`, tc.list, time.Now().UTC().Format("20060102")),
				rr.Header().Get("Content-Disposition"))
			assert.Equal(t, "7", rr.Header().Get("X-Export-Total-Count"))
			assert.Equal(t, "true", rr.Header().Get("X-Export-Truncated"))
			assert.Equal(t, body, rr.Body.String())
			activitySvc.AssertExpectations(t)

			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

func TestExportAdmin_InvalidParamsReturn400WithoutStreaming(t *testing.T) {
	for _, tc := range adminExportCases() {
		for _, query := range []string{"sort_by=bogus", "sort_order=sideways", "prompt_count_min=5&prompt_count_max=1"} {
			t.Run(tc.list+"/"+query, func(t *testing.T) {
				// No service or activity expectation: neither may be reached.
				mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
				activitySvc := &MockActivityService{}
				srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
					adminService: mockAdmin, activityService: activitySvc,
				})

				req := httptest.NewRequest("GET", "/api/v1/admin/"+tc.list+"/export?"+query, nil)
				rr := httptest.NewRecorder()
				mountAdminStrictRouter(srv).ServeHTTP(rr, req)

				require.Equal(t, http.StatusBadRequest, rr.Code)
				assert.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))
				activitySvc.AssertNotCalled(t, "RecordResourceActivity")
				specconformance.AssertConformsToSpec(t, req, rr)
			})
		}
	}
}

func TestExportAdmin_PrepareFailureReturns500(t *testing.T) {
	for _, tc := range adminExportCases() {
		t.Run(tc.list, func(t *testing.T) {
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			mockAdmin.On(tc.method, mock.Anything, mock.Anything).
				Return(services.AdminExport{}, errors.New("count failed")).Once()
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

			req := httptest.NewRequest("GET", "/api/v1/admin/"+tc.list+"/export", nil)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusInternalServerError, rr.Code)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestExportAdmin_NonAdminGets404 pins the non-advertisement gate through the
// real router, and that /export is not shadowed by /users/{id}.
func TestExportAdmin_NonAdminGets404(t *testing.T) {
	for _, tc := range adminExportCases() {
		t.Run(tc.list, func(t *testing.T) {
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
				adminService: servicesmocks.NewMockAdminServiceInterface(t),
				authService:  servicesmocks.NewMockAuthServiceInterface(t),
			})

			req := httptest.NewRequest("GET", "/api/v1/admin/"+tc.list+"/export", nil)
			rr := httptest.NewRecorder()
			srv.router.ServeHTTP(rr, req)

			require.Equal(t, http.StatusNotFound, rr.Code)
		})
	}
}

// TestExportAdmin_MidStreamFailureAbortsConnection proves a failure after the
// 200 went out never reaches the client as a clean (silently short) body, and
// that no activity is recorded for it.
func TestExportAdmin_MidStreamFailureAbortsConnection(t *testing.T) {
	for _, tc := range adminExportCases() {
		t.Run(tc.list, func(t *testing.T) {
			// Larger than net/http's write buffer, so the 200 and part of the body
			// are on the wire before the failure.
			partial := strings.Repeat("row,data\r\n", 2000)
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			mockAdmin.On(tc.method, mock.Anything, mock.Anything).
				Return(fakeAdminExport(10, false, partial, 5, errors.New("db connection lost")), nil).Once()
			activitySvc := &MockActivityService{}
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
				adminService: mockAdmin, activityService: activitySvc,
			})

			ts := httptest.NewServer(panicLoggerMiddleware(srv.logger)(mountAdminStrictRouter(srv)))
			defer ts.Close()

			resp, err := http.Get(ts.URL + "/api/v1/admin/" + tc.list + "/export")
			if err == nil {
				require.Equal(t, http.StatusOK, resp.StatusCode)
				_, err = io.ReadAll(resp.Body)
				assert.NoError(t, resp.Body.Close())
			}
			require.Error(t, err, "the client must see a broken transfer, not a clean EOF")
			activitySvc.AssertNotCalled(t, "RecordResourceActivity")
		})
	}
}

// TestStartAdminExport_ReaderClosedStopsWriter covers a client going away: the
// response visitor closes the pipe reader, the writer's next write fails, and
// the stream ends without recording an activity.
func TestStartAdminExport_ReaderClosedStopsWriter(t *testing.T) {
	activitySvc := &MockActivityService{}
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{activityService: activitySvc})
	a := &adminStrictServer{s: srv}

	writeErr := make(chan error, 1)
	exp := services.AdminExport{WriteCSV: func(_ context.Context, w io.Writer) (int, error) {
		_, err := io.WriteString(w, "id\r\n")
		writeErr <- err
		return 0, err
	}}

	body := a.startAdminExport(context.Background(), adminExportRequest{list: "users"}, exp)
	require.NoError(t, body.Close())

	select {
	case err := <-writeErr:
		require.ErrorIs(t, err, io.ErrClosedPipe)
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not stop after the reader closed")
	}
	activitySvc.AssertNotCalled(t, "RecordResourceActivity")
}

func TestAdminResponseErrorHandler_StreamErrorAborts(t *testing.T) {
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{})
	req := httptest.NewRequest("GET", "/api/v1/admin/users/export", nil)
	err := fmt.Errorf("copy: %w", &adminExportStreamError{err: errors.New("boom")})

	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		srv.adminResponseErrorHandler(httptest.NewRecorder(), req, err)
	})
	assert.Equal(t, "admin export stream failed: boom", (&adminExportStreamError{err: errors.New("boom")}).Error())
}

// setEveryField fills every field of a generated params struct with a non-zero
// value, so a converter that drops a field is caught.
func setEveryField(t *testing.T, v reflect.Value) {
	t.Helper()
	for i := range v.NumField() {
		field := v.Field(i)
		require.Equal(t, reflect.Pointer, field.Kind(), "%s is optional, so a pointer", v.Type().Field(i).Name)
		elem := reflect.New(field.Type().Elem())
		switch e := elem.Elem(); e.Kind() {
		case reflect.String:
			e.SetString("x")
		case reflect.Int64, reflect.Int:
			e.SetInt(1)
		case reflect.Bool:
			e.SetBool(true)
		case reflect.Struct: // time.Time
			e.Set(reflect.ValueOf(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
		case reflect.Array: // uuid.UUID
			e.Set(reflect.ValueOf(uuid.New()))
		default:
			t.Fatalf("unhandled field kind %s on %s", e.Kind(), v.Type().Field(i).Name)
		}
		field.Set(elem)
	}
}

// assertEveryListFieldSet asserts every list param except page/limit came
// through the converter.
func assertEveryListFieldSet(t *testing.T, v reflect.Value) {
	t.Helper()
	for i := range v.NumField() {
		name := v.Type().Field(i).Name
		if name == "Page" || name == "Limit" {
			assert.True(t, v.Field(i).IsNil(), "%s must not be set by an export", name)
			continue
		}
		assert.False(t, v.Field(i).IsNil(), "list param %s was not set from the export params", name)
	}
}

func TestAdminExportParamsToList_CopiesEveryField(t *testing.T) {
	t.Run("users", func(t *testing.T) {
		var e admingen.ExportAdminUsersParams
		setEveryField(t, reflect.ValueOf(&e).Elem())
		l, err := adminExportParamsToList[admingen.ExportAdminUsersParams, admingen.ListAdminUsersParams](e)
		require.NoError(t, err)
		assertEveryListFieldSet(t, reflect.ValueOf(l))
		assert.Equal(t, "x", string(*l.SortBy))
	})
	t.Run("teams", func(t *testing.T) {
		var e admingen.ExportAdminTeamsParams
		setEveryField(t, reflect.ValueOf(&e).Elem())
		l, err := adminExportParamsToList[admingen.ExportAdminTeamsParams, admingen.ListAdminTeamsParams](e)
		require.NoError(t, err)
		assertEveryListFieldSet(t, reflect.ValueOf(l))
	})
	t.Run("projects", func(t *testing.T) {
		var e admingen.ExportAdminProjectsParams
		setEveryField(t, reflect.ValueOf(&e).Elem())
		l, err := adminExportParamsToList[admingen.ExportAdminProjectsParams, admingen.ListAdminProjectsParams](e)
		require.NoError(t, err)
		assertEveryListFieldSet(t, reflect.ValueOf(l))
	})
	t.Run("a field without a counterpart is an error", func(t *testing.T) {
		type exportOnly struct{ Unknown *string }
		_, err := adminExportParamsToList[exportOnly, admingen.ListAdminUsersParams](exportOnly{})
		require.Error(t, err)
	})
}

func TestAdminExportFilterSummary_OnlySetParams(t *testing.T) {
	sortBy := admingen.ExportAdminUsersParamsSortBy("email")
	from := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	summary := adminExportFilterSummary(admingen.ExportAdminUsersParams{
		Search: strPtr("ada"), CreatedFrom: &from, SortBy: &sortBy,
	})
	assert.Equal(t, map[string]interface{}{"search": "ada", "created_from": "2026-01-02T03:04:05Z"}, summary)
	assert.Empty(t, adminExportFilterSummary(func() {}), "an unmarshalable value yields no filters")
}

// TestAdminExportSpecParity pins each export op's parameters to its list op's,
// minus page/limit: same names, locations, requiredness and schemas.
func TestAdminExportSpecParity(t *testing.T) {
	var spec struct {
		Paths map[string]struct {
			Get *struct {
				Parameters []json.RawMessage `json:"parameters"`
			} `json:"get"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(openapispec.JSON, &spec))

	params := func(path string) map[string]string {
		op := spec.Paths[path].Get
		require.NotNil(t, op, "GET %s is documented", path)
		out := map[string]string{}
		for _, raw := range op.Parameters {
			var p struct {
				Name     string          `json:"name"`
				In       string          `json:"in"`
				Required bool            `json:"required"`
				Schema   json.RawMessage `json:"schema"`
			}
			require.NoError(t, json.Unmarshal(raw, &p))
			if p.Name == "page" || p.Name == "limit" {
				continue
			}
			out[p.Name] = fmt.Sprintf("%s|%t|%s", p.In, p.Required, p.Schema)
		}
		return out
	}

	for _, list := range []string{"users", "teams", "projects"} {
		t.Run(list, func(t *testing.T) {
			listParams := params("/api/v1/admin/" + list)
			exportParams := params("/api/v1/admin/" + list + "/export")
			assert.NotEmpty(t, exportParams)
			assert.Equal(t, listParams, exportParams)
			for _, raw := range spec.Paths["/api/v1/admin/"+list+"/export"].Get.Parameters {
				assert.NotContains(t, string(raw), `"name":"page"`)
				assert.NotContains(t, string(raw), `"name":"limit"`)
			}
		})
	}
}
