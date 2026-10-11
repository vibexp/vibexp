package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// The MCP lock tool (#1296). The lock rules belong to ResourceLockService and
// are tested there; these tests pin what the tool adds: resolving the resource
// by slug, calling the same helper as the REST endpoints (so the activity is
// recorded), and the messages an agent reads when a lock is refused.

const mcpLockToolComment = "The nightly release agent reads this slug"

// lockToolMocks is newDeleteResourceTestServer's mocks plus the lock and
// activity services the lock tool reaches.
type lockToolMocks struct {
	*deleteResourceMocks
	locks    *svcmocks.MockResourceLockServiceInterface
	activity *MockActivityService
}

func newLockToolTestServer(t *testing.T) (*Server, *lockToolMocks) {
	t.Helper()
	srv, base := newDeleteResourceTestServer(t)
	m := &lockToolMocks{
		deleteResourceMocks: base,
		locks:               svcmocks.NewMockResourceLockServiceInterface(t),
		activity:            &MockActivityService{},
	}
	t.Cleanup(func() { m.activity.AssertExpectations(t) })
	srv.container = lockServiceContainer{Container: srv.container, locks: m.locks}
	srv.activityService = m.activity
	return srv, m
}

// lockToolCase is one lockable resource type: the params that address it and
// the getter the tool loads it through.
type lockToolCase struct {
	resourceType string
	entityType   string
	resourceID   string
	slug         string
	projectID    string
	// expectLoad makes the type's getter answer with the resource carrying
	// lock, once per call.
	expectLoad func(m *lockToolMocks, lock *models.ResourceLock) *mock.Call
}

func (c lockToolCase) params(comment string) *LockResourceParams {
	return &LockResourceParams{
		TeamID: testTeamUUID, ResourceType: c.resourceType, ProjectID: c.projectID, Slug: c.slug, Comment: comment,
	}
}

func lockToolCases() []lockToolCase {
	return []lockToolCase{
		{
			resourceType: resourceTypePrompt, entityType: activities.EntityTypePrompt,
			resourceID: "prompt-1", slug: "release-notes",
			expectLoad: func(m *lockToolMocks, lock *models.ResourceLock) *mock.Call {
				return m.prompt.On("GetPromptBySlug", testMemberUserID, testTeamUUID, "release-notes").
					Return(&models.Prompt{ID: "prompt-1", Slug: "release-notes", Lock: lock}, nil).Once()
			},
		},
		{
			resourceType: resourceTypeBlueprint, entityType: activities.EntityTypeBlueprint,
			resourceID: "bp-1", slug: "go-rules", projectID: testProjectID,
			expectLoad: func(m *lockToolMocks, lock *models.ResourceLock) *mock.Call {
				return m.blueprint.On(
					"GetBlueprintByProjectIDAndSlugInTeam", testMemberUserID, testTeamUUID, testProjectID, "go-rules",
				).Return(&models.Blueprint{ID: "bp-1", Slug: "go-rules", Lock: lock}, nil).Once()
			},
		},
		{
			resourceType: resourceTypeArtifact, entityType: activities.EntityTypeArtifact,
			resourceID: "art-1", slug: "runbook", projectID: testProjectID,
			expectLoad: func(m *lockToolMocks, lock *models.ResourceLock) *mock.Call {
				return m.artifact.On(
					"GetArtifactByProjectIDAndSlugInTeam", testMemberUserID, testTeamUUID, testProjectID, "runbook",
				).Return(&models.Artifact{ID: "art-1", Slug: "runbook", Lock: lock}, nil).Once()
			},
		},
	}
}

func (m *lockToolMocks) expectLock(c lockToolCase, comment string, err error) {
	m.locks.On("Lock", mock.Anything, testMemberUserID, testTeamUUID, c.resourceType, c.resourceID, comment).
		Return(err).Once()
}

func (m *lockToolMocks) expectLockedActivity(c lockToolCase) {
	resourceID := c.resourceID
	m.activity.On("RecordResourceActivity",
		mock.Anything, testMemberUserID, activities.ActivityTypeResourceLocked, c.entityType, &resourceID,
		"Locked resource: "+c.slug,
		map[string]interface{}{"resource_type": c.resourceType, "slug": c.slug},
	).Return(nil).Once()
}

// assertLockRefused asserts an IsError result with no structured payload and
// that no lock activity was recorded, and returns the message the agent reads.
func assertLockRefused(t *testing.T, m *lockToolMocks, result *mcp.CallToolResult, structured any, err error) string {
	t.Helper()
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "expected an IsError result")
	assert.Nil(t, structured)
	m.activity.AssertNotCalled(t, "RecordResourceActivity")
	return extractText(t, result)
}

func TestLockResourceTool_LocksEachSupportedType(t *testing.T) {
	for _, c := range lockToolCases() {
		t.Run(c.resourceType, func(t *testing.T) {
			srv, m := newLockToolTestServer(t)
			c.expectLoad(m, nil)
			m.expectLock(c, mcpLockToolComment, nil)
			m.expectLockedActivity(c)

			result, structured, err := srv.lockResourceTool(
				context.Background(), nil, c.params(mcpLockToolComment), testMemberUserID,
			)

			require.NoError(t, err)
			require.False(t, result.IsError, extractText(t, result))
			want := &lockResourceResponse{
				Locked: true, ResourceType: c.resourceType, ID: c.resourceID, ProjectID: c.projectID, Slug: c.slug,
			}
			assert.Equal(t, want, structured)

			var fromJSON lockResourceResponse
			require.NoError(t, json.Unmarshal([]byte(extractText(t, result)), &fromJSON))
			assert.Equal(t, *want, fromJSON)
			// The result reports the lock was placed, never the lock itself (#1288).
			assert.NotContains(t, extractText(t, result), `"lock"`)
			assert.NotContains(t, extractText(t, result), mcpLockToolComment)
		})
	}
}

// A team slug and a mixed-case, padded resource_type and slug resolve like the
// delete tool's; the comment goes to the service untouched, which trims it.
func TestLockResourceTool_NormalizesIdentifiers(t *testing.T) {
	srv, m := newLockToolTestServer(t)
	c := lockToolCases()[0]
	c.expectLoad(m, nil)
	m.expectLock(c, "  "+mcpLockToolComment+"  ", nil)
	m.expectLockedActivity(c)

	result, structured, err := srv.lockResourceTool(context.Background(), nil, &LockResourceParams{
		TeamID: testTeamSlug, ResourceType: " Prompt ", Slug: " " + c.slug + " ", Comment: "  " + mcpLockToolComment + "  ",
		// project_id is ignored for a prompt, even an unusable one.
		ProjectID: "not-a-uuid",
	}, testMemberUserID)

	require.NoError(t, err)
	require.False(t, result.IsError, extractText(t, result))
	assert.Equal(t, &lockResourceResponse{
		Locked: true, ResourceType: resourceTypePrompt, ID: c.resourceID, Slug: c.slug,
	}, structured)
}

func TestLockResourceTool_CommentRejected(t *testing.T) {
	tests := []struct {
		name    string
		comment string
		err     error
		want    string
	}{
		{name: "missing", comment: "", err: services.ErrLockCommentRequired, want: "a lock comment is required"},
		{name: "blank", comment: "   ", err: services.ErrLockCommentRequired, want: "a lock comment is required"},
		{
			name: "too long", comment: strings.Repeat("x", services.MaxLockCommentLength+1),
			err: services.ErrLockCommentTooLong, want: "a lock comment may be at most 500 characters",
		},
	}
	for _, tt := range tests {
		for _, c := range lockToolCases() {
			t.Run(tt.name+"/"+c.resourceType, func(t *testing.T) {
				srv, m := newLockToolTestServer(t)
				c.expectLoad(m, nil)
				m.expectLock(c, tt.comment, tt.err)

				result, structured, err := srv.lockResourceTool(
					context.Background(), nil, c.params(tt.comment), testMemberUserID,
				)

				assert.Equal(t, tt.want, assertLockRefused(t, m, result, structured, err))
			})
		}
	}
}

func TestLockResourceTool_AlreadyLocked_NamesHolderAndComment(t *testing.T) {
	for _, c := range lockToolCases() {
		t.Run(c.resourceType, func(t *testing.T) {
			srv, m := newLockToolTestServer(t)
			c.expectLoad(m, heldLock())
			m.expectLock(c, mcpLockToolComment, repositories.ErrResourceAlreadyLocked)

			result, structured, err := srv.lockResourceTool(
				context.Background(), nil, c.params(mcpLockToolComment), testMemberUserID,
			)

			text := assertLockRefused(t, m, result, structured, err)
			assert.Equal(t,
				"This "+c.resourceType+" is already locked by "+lockTestLockerName+
					` with the comment "`+lockTestComment+`". There is no unlock tool: a human unlocks in the VibeXP UI.`,
				text,
			)
		})
	}
}

// The resource was unlocked when the tool loaded it and another caller locked
// it before this one: the conflict message still names that lock, read back
// with one more load.
func TestLockResourceTool_AlreadyLocked_LostRaceReadsTheLockBack(t *testing.T) {
	for _, c := range lockToolCases() {
		t.Run(c.resourceType, func(t *testing.T) {
			srv, m := newLockToolTestServer(t)
			c.expectLoad(m, nil)
			c.expectLoad(m, heldLock())
			m.expectLock(c, mcpLockToolComment, repositories.ErrResourceAlreadyLocked)

			result, structured, err := srv.lockResourceTool(
				context.Background(), nil, c.params(mcpLockToolComment), testMemberUserID,
			)

			text := assertLockRefused(t, m, result, structured, err)
			assert.Contains(t, text, "already locked by "+lockTestLockerName)
			assert.Contains(t, text, lockTestComment)
		})
	}
}

// When the lock cannot be read back either (the resource was deleted, or the
// winner unlocked again), the bare conflict message is still returned.
func TestLockResourceTool_AlreadyLocked_LockUnreadable(t *testing.T) {
	tests := []struct {
		name   string
		reload func(c lockToolCase, m *lockToolMocks)
	}{
		{name: "reload fails", reload: func(c lockToolCase, m *lockToolMocks) {
			c.expectLoad(m, nil).Return(nil, errors.New("gone"))
		}},
		{name: "unlocked again", reload: func(c lockToolCase, m *lockToolMocks) { c.expectLoad(m, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, m := newLockToolTestServer(t)
			c := lockToolCases()[0]
			c.expectLoad(m, nil)
			tt.reload(c, m)
			m.expectLock(c, mcpLockToolComment, repositories.ErrResourceAlreadyLocked)

			result, structured, err := srv.lockResourceTool(
				context.Background(), nil, c.params(mcpLockToolComment), testMemberUserID,
			)

			assert.Equal(t,
				"This prompt is already locked. There is no unlock tool: a human unlocks in the VibeXP UI.",
				assertLockRefused(t, m, result, structured, err),
			)
		})
	}
}

// A lock placed by a member whose display name did not resolve still reports
// its comment.
func TestLockResourceTool_AlreadyLocked_HolderNameUnknown(t *testing.T) {
	srv, m := newLockToolTestServer(t)
	c := lockToolCases()[0]
	held := heldLock()
	held.LockedByName = ""
	c.expectLoad(m, held)
	m.expectLock(c, mcpLockToolComment, repositories.ErrResourceAlreadyLocked)

	result, structured, err := srv.lockResourceTool(
		context.Background(), nil, c.params(mcpLockToolComment), testMemberUserID,
	)

	assert.Equal(t,
		`This prompt is already locked with the comment "`+lockTestComment+
			`". There is no unlock tool: a human unlocks in the VibeXP UI.`,
		assertLockRefused(t, m, result, structured, err),
	)
}

func TestLockResourceTool_UnsupportedTypeRejected(t *testing.T) {
	for _, resourceType := range []string{resourceTypeMemory, "feed", ""} {
		t.Run("type="+resourceType, func(t *testing.T) {
			srv, m := newLockToolTestServer(t)

			result, structured, err := srv.lockResourceTool(context.Background(), nil, &LockResourceParams{
				TeamID: testTeamUUID, ResourceType: resourceType, ProjectID: testProjectID, Slug: "x",
				Comment: mcpLockToolComment,
			}, testMemberUserID)

			assert.Equal(t,
				"resource_type must be one of: prompt, blueprint, artifact",
				assertLockRefused(t, m, result, structured, err),
			)
			m.locks.AssertNotCalled(t, "Lock")
		})
	}
}

func TestLockResourceTool_MissingIdentifiers(t *testing.T) {
	tests := []struct {
		name   string
		params *LockResourceParams
		want   string
	}{
		{
			name:   "prompt without slug",
			params: &LockResourceParams{TeamID: testTeamUUID, ResourceType: resourceTypePrompt, Slug: "  "},
			want:   "slug is required to lock a prompt",
		},
		{
			name: "blueprint without slug",
			params: &LockResourceParams{
				TeamID: testTeamUUID, ResourceType: resourceTypeBlueprint, ProjectID: testProjectID,
			},
			want: "slug is required to lock a blueprint",
		},
		{
			name:   "artifact without project_id",
			params: &LockResourceParams{TeamID: testTeamUUID, ResourceType: resourceTypeArtifact, Slug: "runbook"},
			want:   "project_id is required",
		},
		{
			name: "blueprint with a malformed project_id",
			params: &LockResourceParams{
				TeamID: testTeamUUID, ResourceType: resourceTypeBlueprint, ProjectID: "nope", Slug: "go-rules",
			},
			want: "project_id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, m := newLockToolTestServer(t)
			tt.params.Comment = mcpLockToolComment

			result, structured, err := srv.lockResourceTool(context.Background(), nil, tt.params, testMemberUserID)

			assert.Contains(t, assertLockRefused(t, m, result, structured, err), tt.want)
			m.locks.AssertNotCalled(t, "Lock")
		})
	}
}

func TestLockResourceTool_TeamNotResolved(t *testing.T) {
	t.Run("not a member", func(t *testing.T) {
		srv, m := newLockToolTestServer(t)
		params := lockToolCases()[0].params(mcpLockToolComment)
		params.TeamID = testOtherTeamUUID

		result, structured, err := srv.lockResourceTool(context.Background(), nil, params, testMemberUserID)

		require.NoError(t, err)
		assert.Nil(t, structured)
		assertGenericAccessDenied(t, result)
		m.locks.AssertNotCalled(t, "Lock")
	})

	t.Run("missing team_id", func(t *testing.T) {
		srv, m := newLockToolTestServer(t)
		params := lockToolCases()[0].params(mcpLockToolComment)
		params.TeamID = ""

		result, structured, err := srv.lockResourceTool(context.Background(), nil, params, testMemberUserID)

		assert.Contains(t, assertLockRefused(t, m, result, structured, err), "team_id is required")
		m.locks.AssertNotCalled(t, "Lock")
	})
}

func TestLockResourceTool_ResourceNotFound(t *testing.T) {
	for _, c := range lockToolCases() {
		t.Run(c.resourceType, func(t *testing.T) {
			srv, m := newLockToolTestServer(t)
			c.expectLoad(m, nil).Return(nil, errors.New(c.resourceType+" not found"))

			result, structured, err := srv.lockResourceTool(
				context.Background(), nil, c.params(mcpLockToolComment), testMemberUserID,
			)

			assert.Equal(t,
				"Failed to lock "+c.resourceType+": "+c.resourceType+" not found",
				assertLockRefused(t, m, result, structured, err),
			)
			m.locks.AssertNotCalled(t, "Lock")
		})
	}
}

// The remaining service refusals reach the agent as the same sentences the
// REST endpoint answers with, and an unexpected failure stays opaque.
func TestLockResourceTool_ServiceRefusals(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "permission denied", err: services.ErrPermissionDenied,
			want: "You do not have permission to lock this prompt",
		},
		{name: "deleted meanwhile", err: repositories.ErrPromptNotFound, want: "The prompt was not found"},
		{name: "unexpected", err: errors.New("connection reset: 10.0.0.5"), want: "Failed to lock the prompt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, m := newLockToolTestServer(t)
			c := lockToolCases()[0]
			c.expectLoad(m, nil)
			m.expectLock(c, mcpLockToolComment, tt.err)

			result, structured, err := srv.lockResourceTool(
				context.Background(), nil, c.params(mcpLockToolComment), testMemberUserID,
			)

			assert.Equal(t, tt.want, assertLockRefused(t, m, result, structured, err))
		})
	}
}

// lockFailureMessage is only ever handed lockResource's API errors; anything
// else is still reported rather than dropped.
func TestLockFailureMessage_NonAPIError(t *testing.T) {
	srv, _ := newLockToolTestServer(t)

	got := srv.lockFailureMessage(
		errors.New("boom"), testMemberUserID, "", lockTarget{resourceType: resourceTypePrompt}, nil,
	)

	assert.Equal(t, "Failed to lock prompt: boom", got)
}

// TestMCPLockTool_RegisteredWithoutAnUnlockCounterpart pins the "lock only"
// decision (#1296): agents can place a lock, and no registered tool can remove
// one. The description must say where unlocking happens instead.
func TestMCPLockTool_RegisteredWithoutAnUnlockCounterpart(t *testing.T) {
	registered := registeredMCPToolNames(t)

	assert.Contains(t, registered, lockResourceToolName)
	for name := range registered {
		assert.NotContains(t, strings.ToLower(name), "unlock", "%s: MCP must not offer an unlock tool", name)
	}
	assert.Contains(t, lockResourceToolDescription, "There is no unlock tool")
	// A lock freezes identity, not content (#1289): the description must not
	// tell an agent to stop writing to a locked resource.
	assert.Contains(t, lockResourceToolDescription, "its content stays editable")
	assert.Contains(t, mcpServerInstructions, "its content stays editable")
	assert.Contains(t, mcpServerInstructions, lockResourceToolName)
}
