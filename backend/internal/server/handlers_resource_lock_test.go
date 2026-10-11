package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Resource locks over REST (issue #1288, epic #1285): the three lock
// operations, the `lock` field on the read payloads and the `locked` list
// filter, for prompts, blueprints and artifacts. Every test runs against all
// three resource types through the real route tree, and every response body is
// validated against backend/openapi.yaml.

const (
	lockTestLockerID   = "550e8400-e29b-41d4-a716-446655440091"
	lockTestLockerName = "Ada Lovelace"
	lockTestComment    = "Used by the nightly release agent"
)

func lockTestTime() time.Time { return time.Date(2026, 10, 11, 10, 15, 30, 0, time.UTC) }

func heldLock() *models.ResourceLock {
	return &models.ResourceLock{
		LockedBy:     lockTestLockerID,
		LockedByName: lockTestLockerName,
		Comment:      lockTestComment,
		LockedAt:     lockTestTime(),
	}
}

// lockServiceContainer overrides ResourceLockService on whichever of the three
// domain test containers it wraps, so the lock tests need no change to them.
type lockServiceContainer struct {
	container.Container
	locks services.ResourceLockServiceInterface
}

func (c lockServiceContainer) ResourceLockService() services.ResourceLockServiceInterface {
	return c.locks
}

// lockFixture is one resource type's server, with its resource service, the
// lock service and the activity service mocked.
type lockFixture struct {
	srv      *Server
	locks    *svcmocks.MockResourceLockServiceInterface
	activity *MockActivityService

	userID, teamID, resourceID, slug string
	resourceType, entityType         string
	// url is the resource's own URL; lock and lock/history hang off it.
	url string

	// expectLoad queues one detail read answering with the resource carrying
	// the given lock (nil for unlocked), or failing with err.
	expectLoad func(lock *models.ResourceLock, err error)
	// expectList makes every list read answer with one resource carrying the
	// given lock, and returns where the `Locked` filter it received is stored.
	expectList func(lock *models.ResourceLock) **bool
	request    func(method, path, body string) *http.Request
}

func (f *lockFixture) do(method, path, body string) (*http.Request, *httptest.ResponseRecorder) {
	req := f.request(method, path, body)
	w := httptest.NewRecorder()
	f.srv.router.ServeHTTP(w, req)
	return req, w
}

func (f *lockFixture) install(t *testing.T) {
	t.Helper()
	f.locks = svcmocks.NewMockResourceLockServiceInterface(t)
	f.activity = &MockActivityService{}
	t.Cleanup(func() { f.activity.AssertExpectations(t) })
	f.srv.container = lockServiceContainer{Container: f.srv.container, locks: f.locks}
	f.srv.activityService = f.activity
}

// expectActivity lets exactly one lock activity of the given type be recorded
// against the resource.
func (f *lockFixture) expectActivity(activityType string) {
	resourceID := f.resourceID
	f.activity.On("RecordResourceActivity",
		mock.Anything, f.userID, activityType, f.entityType, &resourceID, mock.Anything,
		map[string]interface{}{"resource_type": f.resourceType, "slug": f.slug},
	).Return(nil).Once()
}

type lockDomain struct {
	name  string
	setup func(t *testing.T) *lockFixture
	// listURLs are the list operations of this resource type: five in all.
	listURLs []string
}

func lockDomains() []lockDomain {
	return []lockDomain{
		{name: "prompt", setup: promptLockFixture, listURLs: []string{
			"/api/v1/" + strictPrTeamID + "/prompts",
		}},
		{name: "blueprint", setup: blueprintLockFixture, listURLs: []string{
			"/api/v1/" + strictBpTeamID + "/blueprints",
			"/api/v1/" + strictBpTeamID + "/blueprints/" + strictBpProject,
		}},
		{name: "artifact", setup: artifactLockFixture, listURLs: []string{
			"/api/v1/" + strictArtTeamID + "/artifacts",
			"/api/v1/" + strictArtTeamID + "/artifacts/" + strictArtProject,
		}},
	}
}

func promptLockFixture(t *testing.T) *lockFixture {
	t.Helper()
	srv, c := strictPromptServer(t)
	f := &lockFixture{
		srv:    srv,
		userID: strictPrUserID, teamID: strictPrTeamID, resourceID: "prompt-1", slug: strictPrSlug,
		resourceType: models.RelationResourceTypePrompt, entityType: activities.EntityTypePrompt,
		url: "/api/v1/" + strictPrTeamID + "/prompts/" + strictPrSlug,
		request: func(method, path, body string) *http.Request {
			if body == "" {
				return makeAuthenticatedRequest(method, path, nil, strictPrUserID)
			}
			return makeAuthenticatedRequest(method, path, json.RawMessage(body), strictPrUserID)
		},
	}
	f.expectLoad = func(lock *models.ResourceLock, err error) {
		call := c.promptService.On("GetPromptBySlug", strictPrUserID, strictPrTeamID, strictPrSlug).Once()
		if err != nil {
			call.Return(nil, err)
			return
		}
		prompt := samplePrompt()
		prompt.Lock = lock
		call.Return(prompt, nil)
	}
	f.expectList = func(lock *models.ResourceLock) **bool {
		var got *bool
		prompt := samplePrompt()
		prompt.Lock = lock
		c.promptService.On("ListPrompts", strictPrUserID,
			mock.MatchedBy(func(filters services.PromptFilters) bool { got = filters.Locked; return true })).
			Return(&models.PromptListResponse{
				Prompts: []models.Prompt{*prompt}, TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
			}, nil)
		return &got
	}
	f.install(t)
	return f
}

func blueprintLockFixture(t *testing.T) *lockFixture {
	t.Helper()
	svc := svcmocks.NewMockBlueprintServiceInterface(t)
	f := &lockFixture{
		srv:    strictBlueprintServer(t, svc),
		userID: strictBpUserID, teamID: strictBpTeamID, resourceID: "spec-1", slug: strictBpSlug,
		resourceType: models.RelationResourceTypeBlueprint, entityType: activities.EntityTypeBlueprint,
		url: "/api/v1/" + strictBpTeamID + "/blueprints/" + strictBpProject + "/" + strictBpSlug,
		request: func(method, path, body string) *http.Request {
			return createBlueprintAuthenticatedRequest(method, path, body, strictBpUserID)
		},
	}
	f.expectLoad = func(lock *models.ResourceLock, err error) {
		call := svc.On("GetBlueprintByProjectIDAndSlugInTeam",
			strictBpUserID, strictBpTeamID, strictBpProject, strictBpSlug).Once()
		if err != nil {
			call.Return(nil, err)
			return
		}
		blueprint := sampleStrictBlueprint()
		blueprint.Lock = lock
		call.Return(blueprint, nil)
	}
	f.expectList = func(lock *models.ResourceLock) **bool {
		var got *bool
		blueprint := sampleStrictBlueprint()
		blueprint.Lock = lock
		svc.On("ListBlueprints", strictBpUserID,
			mock.MatchedBy(func(filters services.BlueprintFilters) bool { got = filters.Locked; return true })).
			Return(&models.BlueprintListResponse{
				Blueprints: []models.Blueprint{*blueprint}, TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
			}, nil)
		return &got
	}
	f.install(t)
	return f
}

func artifactLockFixture(t *testing.T) *lockFixture {
	t.Helper()
	svc := svcmocks.NewMockArtifactServiceInterface(t)
	f := &lockFixture{
		srv:    strictArtifactServer(t, svc),
		userID: strictArtUserID, teamID: strictArtTeamID, resourceID: "art-1", slug: strictArtSlug,
		resourceType: models.RelationResourceTypeArtifact, entityType: activities.EntityTypeArtifact,
		url: "/api/v1/" + strictArtTeamID + "/artifacts/" + strictArtProject + "/" + strictArtSlug,
		request: func(method, path, body string) *http.Request {
			return createAuthenticatedRequest(method, path, body, strictArtUserID)
		},
	}
	f.expectLoad = func(lock *models.ResourceLock, err error) {
		call := svc.On("GetArtifactByProjectIDAndSlugInTeam",
			strictArtUserID, strictArtTeamID, strictArtProject, strictArtSlug).Once()
		if err != nil {
			call.Return(nil, err)
			return
		}
		artifact := sampleStrictArtifact()
		artifact.Lock = lock
		call.Return(artifact, nil)
	}
	f.expectList = func(lock *models.ResourceLock) **bool {
		var got *bool
		artifact := sampleStrictArtifact()
		artifact.Lock = lock
		svc.On("ListArtifacts", strictArtUserID,
			mock.MatchedBy(func(filters services.ArtifactFilters) bool { got = filters.Locked; return true })).
			Return(&models.ArtifactListResponse{
				Artifacts: []models.Artifact{*artifact}, TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
			}, nil)
		return &got
	}
	f.install(t)
	return f
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return body
}

// wantHeldLock is heldLock() as the wire renders it.
func wantHeldLock() map[string]any {
	return map[string]any{
		"locked_by": map[string]any{"user_id": lockTestLockerID, "display_name": lockTestLockerName},
		"comment":   lockTestComment,
		"locked_at": "2026-10-11T10:15:30Z",
	}
}

// lockRequestBody is a well-formed lock request carrying lockTestComment.
const lockRequestBody = `{"comment":"` + lockTestComment + `"}`

// Locking answers 200 with the resource as the lock left it, hands the comment
// to the service untouched (trimming is the service's rule) and records exactly
// one activity.
func TestResourceLock_LockReturnsTheLockedResource(t *testing.T) {
	for _, d := range lockDomains() {
		t.Run(d.name, func(t *testing.T) {
			f := d.setup(t)
			f.expectLoad(nil, nil)
			f.locks.EXPECT().
				Lock(mock.Anything, f.userID, f.teamID, f.resourceType, f.resourceID, lockTestComment).
				Return(nil).Once()
			f.expectLoad(heldLock(), nil)
			f.expectActivity(activities.ActivityTypeResourceLocked)

			req, w := f.do(http.MethodPost, f.url+"/lock", lockRequestBody)

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			specconformance.AssertConformsToSpec(t, req, w)
			body := decodeBody(t, w)
			assert.Equal(t, f.resourceID, body["id"])
			assert.Equal(t, wantHeldLock(), body["lock"])
		})
	}
}

// Unlocking answers 200 with the resource carrying `"lock": null` and records
// exactly one activity.
func TestResourceLock_UnlockReturnsTheUnlockedResource(t *testing.T) {
	for _, d := range lockDomains() {
		t.Run(d.name, func(t *testing.T) {
			f := d.setup(t)
			f.expectLoad(heldLock(), nil)
			f.locks.EXPECT().
				Unlock(mock.Anything, f.userID, f.teamID, f.resourceType, f.resourceID).
				Return(nil).Once()
			f.expectLoad(nil, nil)
			f.expectActivity(activities.ActivityTypeResourceUnlocked)

			req, w := f.do(http.MethodDelete, f.url+"/lock", "")

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			specconformance.AssertConformsToSpec(t, req, w)
			body := decodeBody(t, w)
			lock, present := body["lock"]
			assert.True(t, present, "`lock` is required: the key must be present when unlocked")
			assert.Nil(t, lock)
		})
	}
}

// Every way a lock or unlock can be refused maps to its documented status and
// code, the body conforms to the spec, and nothing is recorded as an activity
// (the activity mock has no expectation, so a call would panic).
func TestResourceLock_RefusalsMapToTheirDocumentedStatuses(t *testing.T) {
	cases := []struct {
		name       string
		unlock     bool
		serviceErr error
		wantStatus int
		wantCode   string
	}{
		{name: "lock: comment missing", serviceErr: services.ErrLockCommentRequired,
			wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "lock: comment too long", serviceErr: services.ErrLockCommentTooLong,
			wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "lock: already locked", serviceErr: repositories.ErrResourceAlreadyLocked,
			wantStatus: http.StatusConflict, wantCode: "RESOURCE_LOCKED"},
		{name: "lock: not a member", serviceErr: services.ErrPermissionDenied,
			wantStatus: http.StatusForbidden, wantCode: "FORBIDDEN"},
		{name: "lock: unexpected failure", serviceErr: errors.New("database exploded"),
			wantStatus: http.StatusInternalServerError, wantCode: "INTERNAL_ERROR"},
		{name: "unlock: someone else's lock", unlock: true, serviceErr: services.ErrPermissionDenied,
			wantStatus: http.StatusForbidden, wantCode: "FORBIDDEN"},
		{name: "unlock: not locked", unlock: true, serviceErr: repositories.ErrResourceNotLocked,
			wantStatus: http.StatusConflict, wantCode: "RESOURCE_NOT_LOCKED"},
		{name: "unlock: unexpected failure", unlock: true, serviceErr: errors.New("database exploded"),
			wantStatus: http.StatusInternalServerError, wantCode: "INTERNAL_ERROR"},
	}
	for _, d := range lockDomains() {
		for _, c := range cases {
			t.Run(d.name+"/"+c.name, func(t *testing.T) {
				f := d.setup(t)
				f.expectLoad(nil, nil)

				var req *http.Request
				var w *httptest.ResponseRecorder
				if c.unlock {
					f.locks.EXPECT().Unlock(mock.Anything, f.userID, f.teamID, f.resourceType, f.resourceID).
						Return(c.serviceErr).Once()
					req, w = f.do(http.MethodDelete, f.url+"/lock", "")
				} else {
					f.locks.EXPECT().
						Lock(mock.Anything, f.userID, f.teamID, f.resourceType, f.resourceID, mock.Anything).
						Return(c.serviceErr).Once()
					req, w = f.do(http.MethodPost, f.url+"/lock", lockRequestBody)
				}

				require.Equal(t, c.wantStatus, w.Code, w.Body.String())
				specconformance.AssertConformsToSpec(t, req, w)
				assert.Equal(t, c.wantCode, decodeBody(t, w)["code"])
				assert.NotContains(t, w.Body.String(), "database exploded", "an internal error must stay opaque")
			})
		}
	}
}

// A resource deleted between the handler's read and the lock write reports the
// repository's own not-found sentinel, which is a 404 for every type.
func TestResourceLock_DeleteRacingTheLockIsA404(t *testing.T) {
	notFound := map[string]error{
		"prompt":    repositories.ErrPromptNotFound,
		"blueprint": repositories.ErrBlueprintNotFound,
		"artifact":  repositories.ErrArtifactNotFound,
	}
	for _, d := range lockDomains() {
		t.Run(d.name, func(t *testing.T) {
			f := d.setup(t)
			f.expectLoad(nil, nil)
			f.locks.EXPECT().
				Lock(mock.Anything, f.userID, f.teamID, f.resourceType, f.resourceID, mock.Anything).
				Return(notFound[d.name]).Once()

			req, w := f.do(http.MethodPost, f.url+"/lock", lockRequestBody)

			require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
			specconformance.AssertConformsToSpec(t, req, w)
		})
	}
}

// A slug that resolves to nothing is the same 404 the detail read answers, for
// all three operations, and the lock service is never reached.
func TestResourceLock_UnknownResourceIsA404(t *testing.T) {
	notFound := map[string]error{
		"prompt":    repositories.ErrPromptNotFound,
		"blueprint": errors.New("blueprint not found"),
		"artifact":  repositories.ErrArtifactNotFound,
	}
	ops := []struct{ method, suffix, body string }{
		{http.MethodPost, "/lock", lockRequestBody},
		{http.MethodDelete, "/lock", ""},
		{http.MethodGet, "/lock/history", ""},
	}
	for _, d := range lockDomains() {
		for _, op := range ops {
			t.Run(d.name+"/"+op.method+op.suffix, func(t *testing.T) {
				f := d.setup(t)
				f.expectLoad(nil, notFound[d.name])

				req, w := f.do(op.method, f.url+op.suffix, op.body)

				require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
				specconformance.AssertConformsToSpec(t, req, w)
			})
		}
	}
}

// The generated decoder enforces neither `required` nor `minLength`: a body
// with no comment reaches the service as "", and it is the service's rejection
// that produces the 400. A body that is not JSON never reaches a handler.
func TestResourceLock_LockRequestBodyEdges(t *testing.T) {
	for _, d := range lockDomains() {
		t.Run(d.name+"/no comment key", func(t *testing.T) {
			f := d.setup(t)
			f.expectLoad(nil, nil)
			f.locks.EXPECT().Lock(mock.Anything, f.userID, f.teamID, f.resourceType, f.resourceID, "").
				Return(services.ErrLockCommentRequired).Once()

			req, w := f.do(http.MethodPost, f.url+"/lock", `{}`)

			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			specconformance.AssertConformsToSpec(t, req, w)
			assert.Contains(t, w.Body.String(), "a lock comment is required")
		})
		t.Run(d.name+"/malformed JSON", func(t *testing.T) {
			f := d.setup(t)

			// Built by hand: the request helpers marshal their body, and this
			// one must not be valid JSON.
			req := f.request(http.MethodPost, f.url+"/lock", `{}`)
			malformed := httptest.NewRequest(http.MethodPost, f.url+"/lock", strings.NewReader(`{"comment":`))
			malformed.Header = req.Header
			malformed = malformed.WithContext(req.Context())
			w := httptest.NewRecorder()
			f.srv.router.ServeHTTP(w, malformed)

			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			specconformance.AssertConformsToSpec(t, malformed, w)
		})
	}
}

// The history lists lock and unlock events newest first, with the actor and
// the comment: the comment is null on an unlock, and the actor is null once the
// account is gone.
func TestResourceLock_HistoryListsEvents(t *testing.T) {
	actorID := lockTestLockerID
	comment := lockTestComment
	events := []*models.ResourceLockEvent{
		{
			ID: "550e8400-e29b-41d4-a716-446655440093", Action: models.LockActionUnlock,
			CreatedAt: lockTestTime().Add(time.Hour),
		},
		{
			ID: "550e8400-e29b-41d4-a716-446655440092", Action: models.LockActionLock,
			ActorUserID: &actorID, ActorName: lockTestLockerName, Comment: &comment,
			CreatedAt: lockTestTime(),
		},
	}
	for _, d := range lockDomains() {
		t.Run(d.name, func(t *testing.T) {
			f := d.setup(t)
			f.expectLoad(nil, nil)
			f.locks.EXPECT().History(mock.Anything, f.userID, f.teamID, f.resourceType, f.resourceID).
				Return(events, nil).Once()

			req, w := f.do(http.MethodGet, f.url+"/lock/history", "")

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			specconformance.AssertConformsToSpec(t, req, w)
			assert.Equal(t, map[string]any{"events": []any{
				map[string]any{
					"id": "550e8400-e29b-41d4-a716-446655440093", "action": "unlock",
					"actor": nil, "comment": nil, "created_at": "2026-10-11T11:15:30Z",
				},
				map[string]any{
					"id": "550e8400-e29b-41d4-a716-446655440092", "action": "lock",
					"actor":   map[string]any{"user_id": lockTestLockerID, "display_name": lockTestLockerName},
					"comment": lockTestComment, "created_at": "2026-10-11T10:15:30Z",
				},
			}}, decodeBody(t, w))
		})
	}
}

// `events` is a required array: a resource that was never locked answers `[]`,
// never `null`, even when the service hands back a nil slice.
func TestResourceLock_EmptyHistoryIsAnEmptyArray(t *testing.T) {
	for _, d := range lockDomains() {
		t.Run(d.name, func(t *testing.T) {
			f := d.setup(t)
			f.expectLoad(nil, nil)
			f.locks.EXPECT().History(mock.Anything, f.userID, f.teamID, f.resourceType, f.resourceID).
				Return(nil, nil).Once()

			req, w := f.do(http.MethodGet, f.url+"/lock/history", "")

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			specconformance.AssertConformsToSpec(t, req, w)
			assert.JSONEq(t, `{"events":[]}`, w.Body.String())
		})
	}
}

// History failures: a non-member is a 403, anything else an opaque 500 -- and
// so is an event whose id the spec's uuid format cannot carry.
func TestResourceLock_HistoryFailures(t *testing.T) {
	cases := map[string]struct {
		events     []*models.ResourceLockEvent
		err        error
		wantStatus int
	}{
		"not a member":       {err: services.ErrPermissionDenied, wantStatus: http.StatusForbidden},
		"unexpected failure": {err: errors.New("database exploded"), wantStatus: http.StatusInternalServerError},
		"malformed event id": {
			events:     []*models.ResourceLockEvent{{ID: "not-a-uuid", Action: models.LockActionLock}},
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, d := range lockDomains() {
		for name, c := range cases {
			t.Run(d.name+"/"+name, func(t *testing.T) {
				f := d.setup(t)
				f.expectLoad(nil, nil)
				f.locks.EXPECT().History(mock.Anything, f.userID, f.teamID, f.resourceType, f.resourceID).
					Return(c.events, c.err).Once()

				req, w := f.do(http.MethodGet, f.url+"/lock/history", "")

				require.Equal(t, c.wantStatus, w.Code, w.Body.String())
				specconformance.AssertConformsToSpec(t, req, w)
				assert.NotContains(t, w.Body.String(), "database exploded")
			})
		}
	}
}

// The detail read and every list operation carry `lock`: the lock object when
// the resource is locked, and an explicit null when it is not.
func TestResourceLock_ReadPayloadsCarryTheLock(t *testing.T) {
	states := map[string]struct {
		lock *models.ResourceLock
		want any
	}{
		"locked":   {lock: heldLock(), want: wantHeldLock()},
		"unlocked": {lock: nil, want: nil},
	}
	for _, d := range lockDomains() {
		for name, state := range states {
			t.Run(d.name+"/get/"+name, func(t *testing.T) {
				f := d.setup(t)
				f.expectLoad(state.lock, nil)

				req, w := f.do(http.MethodGet, f.url, "")

				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				specconformance.AssertConformsToSpec(t, req, w)
				lock, present := decodeBody(t, w)["lock"]
				assert.True(t, present, "`lock` must be present on the detail read")
				assert.Equal(t, state.want, lock)
			})

			for _, listURL := range d.listURLs {
				t.Run(d.name+"/list "+listURL+"/"+name, func(t *testing.T) {
					f := d.setup(t)
					f.expectList(state.lock)

					req, w := f.do(http.MethodGet, listURL, "")

					require.Equal(t, http.StatusOK, w.Code, w.Body.String())
					specconformance.AssertConformsToSpec(t, req, w)
					item := firstListItem(t, decodeBody(t, w))
					lock, present := item["lock"]
					assert.True(t, present, "`lock` must be present on every list item")
					assert.Equal(t, state.want, lock)
				})
			}
		}
	}
}

// firstListItem digs the first resource out of a list response, whichever of
// the three shapes it has (prompts keep their legacy {data: {prompts}} envelope).
func firstListItem(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	if data, ok := body["data"].(map[string]any); ok {
		body = data
	}
	for _, key := range []string{"prompts", "blueprints", "artifacts"} {
		if items, ok := body[key].([]any); ok {
			require.Len(t, items, 1)
			return items[0].(map[string]any)
		}
	}
	t.Fatalf("no resource list in %v", body)
	return nil
}

// `locked` reaches the service on all five list operations as true, false or
// (omitted or empty) no filter at all.
func TestResourceLock_LockedFilterReachesTheService(t *testing.T) {
	yes, no := true, false
	cases := map[string]*bool{
		"?locked=true":  &yes,
		"?locked=false": &no,
		"":              nil,
		"?locked=":      nil,
	}
	for _, d := range lockDomains() {
		for _, listURL := range d.listURLs {
			for query, want := range cases {
				t.Run(d.name+" "+listURL+query, func(t *testing.T) {
					f := d.setup(t)
					got := f.expectList(nil)

					req, w := f.do(http.MethodGet, listURL+query, "")

					require.Equal(t, http.StatusOK, w.Code, w.Body.String())
					specconformance.AssertConformsToSpec(t, req, w)
					assert.Equal(t, want, *got)
				})
			}
		}
	}
}

// Any other value of `locked` is a 400: a silently ignored filter would return
// the full list, which reads as a legitimate answer.
func TestResourceLock_LockedFilterRejectsNonBooleans(t *testing.T) {
	for _, d := range lockDomains() {
		for _, listURL := range d.listURLs {
			t.Run(d.name+" "+listURL, func(t *testing.T) {
				f := d.setup(t)

				req, w := f.do(http.MethodGet, listURL+"?locked=maybe", "")

				require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
				specconformance.AssertConformsToSpec(t, req, w)
				assert.Contains(t, w.Body.String(), "locked")
			})
		}
	}
}

// Recording the activity is best effort: a failure is logged and the lock
// change still answers 200.
func TestResourceLock_ActivityFailureDoesNotFailTheRequest(t *testing.T) {
	f := promptLockFixture(t)
	f.expectLoad(nil, nil)
	f.locks.EXPECT().Lock(mock.Anything, f.userID, f.teamID, f.resourceType, f.resourceID, lockTestComment).
		Return(nil).Once()
	f.expectLoad(heldLock(), nil)
	f.activity.On("RecordResourceActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything).Return(errors.New("activity store down")).Once()

	_, w := f.do(http.MethodPost, f.url+"/lock", lockRequestBody)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// With no activity service wired (as in most handler tests) a lock change
// simply records nothing.
func TestResourceLock_NoActivityServiceIsANoOp(t *testing.T) {
	f := promptLockFixture(t)
	f.srv.activityService = nil
	f.expectLoad(heldLock(), nil)
	f.locks.EXPECT().Unlock(mock.Anything, f.userID, f.teamID, f.resourceType, f.resourceID).Return(nil).Once()
	f.expectLoad(nil, nil)

	_, w := f.do(http.MethodDelete, f.url+"/lock", "")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// The blueprint routes take project_id as a plain string, so the lock
// operations validate it themselves, exactly as the detail read does.
func TestResourceLock_BlueprintRejectsAMalformedProjectID(t *testing.T) {
	f := blueprintLockFixture(t)
	bad := "/api/v1/" + strictBpTeamID + "/blueprints/not-a-uuid/" + strictBpSlug

	for _, op := range []struct{ method, suffix, body string }{
		{http.MethodPost, "/lock", lockRequestBody},
		{http.MethodDelete, "/lock", ""},
		{http.MethodGet, "/lock/history", ""},
	} {
		req, w := f.do(op.method, bad+op.suffix, op.body)

		require.Equal(t, http.StatusBadRequest, w.Code, op.method+op.suffix+": "+w.Body.String())
		specconformance.AssertConformsToSpec(t, req, w)
	}
}
