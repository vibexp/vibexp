package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// TestAllowedJSONFields_DerivedFromGeneratedType is what keeps the guard honest.
// The allowed field set is reflected off the GENERATED request type, so adding a
// property to the schema widens the guard automatically and no hand-maintained
// list can drift from the spec.
func TestAllowedJSONFields_DerivedFromGeneratedType(t *testing.T) {
	allowed := allowedJSONFields(admingen.AdminUserUpdateRequest{})

	assert.Equal(t, map[string]struct{}{"name": {}}, allowed,
		"AdminUserUpdateRequest currently declares only `name`; if this fails the "+
			"schema changed and the guard has already followed it")
}

// TestUpdateAdminUser_RejectsNonEditableFields is #455's acceptance criterion:
// identity fields must be REJECTED, not ignored.
//
// This cannot be left to the generated code. oapi-codegen emits a plain
// json.Decode with no DisallowUnknownFields, so without the guard the request
// below would return 200 while silently discarding the email the caller believed
// they were changing. The service mock has NO expectations, so any leak through
// to the handler fails the test rather than passing quietly.
func TestUpdateAdminUser_RejectsNonEditableFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "email is IdP-owned",
			body: `{"name":"Renamed","email":"attacker@example.com"}`,
			want: "email",
		},
		{
			name: "idp_provider is IdP-owned",
			body: `{"name":"Renamed","idp_provider":"evil"}`,
			want: "idp_provider",
		},
		{
			name: "idp_subject is IdP-owned",
			body: `{"name":"Renamed","idp_subject":"sub-123"}`,
			want: "idp_subject",
		},
		{
			name: "id cannot be reassigned",
			body: `{"name":"Renamed","id":"11111111-1111-1111-1111-111111111111"}`,
			want: "id",
		},
		{
			name: "status is owned by the suspend/reactivate endpoints",
			body: `{"name":"Renamed","status":"active"}`,
			want: "status",
		},
		{
			name: "several at once are all named",
			body: `{"name":"Renamed","email":"a@b.c","idp_subject":"s"}`,
			want: "email, idp_subject",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id := uuid.NewString()
			// No expectations: reaching the service at all is a failure.
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

			req := httptest.NewRequest("PATCH", "/api/v1/admin/users/"+id, bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.want)
		})
	}
}

// TestUpdateAdminUser_AcceptsOnlyDeclaredField is the control: the guard must
// not reject the legitimate body.
func TestUpdateAdminUser_AcceptsOnlyDeclaredField(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("UpdateUserName", mock.Anything, id, "Renamed").
		Return(adminUserDetailNamed(id, "Renamed"), nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("PATCH", "/api/v1/admin/users/"+id,
		bytes.NewReader([]byte(`{"name":"Renamed"}`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
}

// TestAdminBodyGuard_IgnoresUnguardedRequests proves the guard is inert for
// operations it does not cover, so it cannot break the rest of the surface.
func TestAdminBodyGuard_IgnoresUnguardedRequests(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetUserDetail", mock.Anything, id).Return(adminUserDetailNamed(id, "Target"), nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	// A GET carries no body and is not in the guard's registry.
	req := httptest.NewRequest("GET", "/api/v1/admin/users/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
}

// TestUnknownFields returns offending keys in a stable, sorted order so error
// messages do not vary run to run (Go map iteration is randomized).
func TestUnknownFields(t *testing.T) {
	allowed := map[string]struct{}{"name": {}}
	body := map[string]json.RawMessage{
		"zeta":  json.RawMessage(`1`),
		"name":  json.RawMessage(`"ok"`),
		"alpha": json.RawMessage(`2`),
	}

	assert.Equal(t, []string{"alpha", "zeta"}, unknownFields(body, allowed))
	assert.Empty(t, unknownFields(map[string]json.RawMessage{"name": json.RawMessage(`"ok"`)}, allowed))
}
