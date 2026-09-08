package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// The optional memory title (issue #911, epic #899).
//
// `title` is REQUIRED-but-nullable in the response schema, which is what makes
// the key always present. AssertConformsToSpec alone would NOT catch a title
// silently serialized as "" instead of null (both satisfy `string | null`), so
// the wire form is asserted explicitly -- the same reasoning as the labels
// assertions in resource_labels_test.go.

func TestGetMemory_TitleAlwaysPresentOnTheWire(t *testing.T) {
	t.Run("untitled memory emits null, never an absent key or an empty string", func(t *testing.T) {
		container := newMockMemoryContainer(t)
		memory := sampleStrictMemory()
		memory.Title = nil
		container.memoryService.On("GetMemory", memoriesTestUserID, memoriesTestTeamID, memoriesTestMemoryID).
			Return(memory, nil)

		srv := createMemoryTestServer(container)
		req, w := getMemoryRequest(t, srv, memoriesTestMemoryID)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		specconformance.AssertConformsToSpec(t, req, w)
		assert.Contains(t, w.Body.String(), `"title":null`)
		assert.NotContains(t, w.Body.String(), `"title":""`)
	})

	t.Run("a title round-trips to the wire", func(t *testing.T) {
		container := newMockMemoryContainer(t)
		memory := sampleStrictMemory()
		title := "Deploy checklist"
		memory.Title = &title
		container.memoryService.On("GetMemory", memoriesTestUserID, memoriesTestTeamID, memoriesTestMemoryID).
			Return(memory, nil)

		srv := createMemoryTestServer(container)
		req, w := getMemoryRequest(t, srv, memoriesTestMemoryID)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		specconformance.AssertConformsToSpec(t, req, w)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "Deploy checklist", body["title"])
	})
}

// The list page is where the SPA needs the title most -- it is the only place
// that would otherwise have to derive one per row.
func TestListMemories_CarriesTitle(t *testing.T) {
	container := newMockMemoryContainer(t)
	titled := *sampleStrictMemory()
	title := "Deploy checklist"
	titled.Title = &title
	untitled := *sampleStrictMemory()
	untitled.ID = memoriesTestProjectID
	untitled.Title = nil

	container.memoryService.On("ListMemories", memoriesTestUserID, mock.Anything).
		Return(&models.MemoryListResponse{
			Memories:   models.JSONArray[models.Memory]{titled, untitled},
			TotalCount: 2, Page: 1, PerPage: 20, TotalPages: 1,
		}, nil)

	srv := createMemoryTestServer(container)
	req := makeMemoryAuthenticatedRequest(
		"GET", "/api/v1/"+memoriesTestTeamID+"/memories", nil, memoriesTestUserID)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	var body struct {
		Memories []map[string]any `json:"memories"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Memories, 2)
	assert.Equal(t, "Deploy checklist", body.Memories[0]["title"])
	require.Contains(t, body.Memories[1], "title", "the key must be present even when null")
	assert.Nil(t, body.Memories[1]["title"])
}

// Without the errors.Is arm the over-long title falls through to the generic
// 500 branch, which is the failure this pins.
func TestCreateMemory_MapsInvalidTitleTo400(t *testing.T) {
	container := newMockMemoryContainer(t)
	container.projectRepository.On("GetByID", mock.Anything, memoriesTestUserID, testHandlerProjectID).
		Return(&models.Project{ID: testHandlerProjectID, UserID: memoriesTestUserID, TeamID: memoriesTestTeamID}, nil)
	container.memoryService.On("CreateMemory", memoriesTestUserID, memoriesTestTeamID, mock.Anything).
		Return(nil, services.ErrInvalidMemoryTitle)

	srv := createMemoryTestServer(container)
	req := makeMemoryAuthenticatedRequest("POST",
		"/api/v1/"+memoriesTestTeamID+"/memories",
		map[string]any{
			"project_id": testHandlerProjectID,
			"text":       "hi",
			"title":      strings.Repeat("x", services.MaxMemoryTitleLength+1),
		},
		memoriesTestUserID)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "invalid title")
}

func TestUpdateMemory_MapsInvalidTitleTo400(t *testing.T) {
	container := newMockMemoryContainer(t)
	container.memoryService.On("UpdateMemory",
		memoriesTestUserID, memoriesTestTeamID, memoriesTestMemoryID, mock.Anything).
		Return(nil, services.ErrInvalidMemoryTitle)

	srv := createMemoryTestServer(container)
	req := makeMemoryAuthenticatedRequest("PUT",
		"/api/v1/"+memoriesTestTeamID+"/memories/"+memoriesTestMemoryID,
		map[string]any{"title": strings.Repeat("x", services.MaxMemoryTitleLength+1)},
		memoriesTestUserID)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "invalid title")
}

// The handler decodes the body, so this is where the three-state semantics
// actually have to survive: it is the only layer that can tell an absent key
// from an explicit null.
func TestUpdateMemory_TitleThreeStatesReachTheService(t *testing.T) {
	tests := []struct {
		name     string
		body     map[string]any
		assertOn func(t *testing.T, req *models.UpdateMemoryRequest)
	}{
		{
			name: "absent title leaves the field unset",
			body: map[string]any{"text": "new text"},
			assertOn: func(t *testing.T, req *models.UpdateMemoryRequest) {
				t.Helper()
				assert.False(t, req.Title.Set)
			},
		},
		{
			name: "explicit null is set with no value",
			body: map[string]any{"title": nil},
			assertOn: func(t *testing.T, req *models.UpdateMemoryRequest) {
				t.Helper()
				assert.True(t, req.Title.Set)
				assert.Nil(t, req.Title.Value)
			},
		},
		{
			name: "a value is set with that value",
			body: map[string]any{"title": "Deploy checklist"},
			assertOn: func(t *testing.T, req *models.UpdateMemoryRequest) {
				t.Helper()
				require.NotNil(t, req.Title.Value)
				assert.Equal(t, "Deploy checklist", *req.Title.Value)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := newMockMemoryContainer(t)
			var captured *models.UpdateMemoryRequest
			container.memoryService.On("UpdateMemory",
				memoriesTestUserID, memoriesTestTeamID, memoriesTestMemoryID, mock.Anything).
				Run(func(args mock.Arguments) {
					captured = args.Get(3).(*models.UpdateMemoryRequest)
				}).
				Return(sampleStrictMemory(), nil)

			srv := createMemoryTestServer(container)
			req := makeMemoryAuthenticatedRequest("PUT",
				"/api/v1/"+memoriesTestTeamID+"/memories/"+memoriesTestMemoryID, tt.body, memoriesTestUserID)
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.NotNil(t, captured)
			tt.assertOn(t, captured)
		})
	}
}
