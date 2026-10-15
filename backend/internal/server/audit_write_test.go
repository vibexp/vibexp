package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/audit"
	"github.com/vibexp/vibexp/internal/services/projectmigration"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
)

// capturedAudit is one Record call: the event, and the request-scoped facts
// the real recorder would read from the context.
type capturedAudit struct {
	event  audit.Event
	origin *models.WriteOrigin
	apiKey string
}

// auditCapture is an audit.Recorder that keeps what it was given.
type auditCapture struct {
	mu  sync.Mutex
	got []capturedAudit
}

func (c *auditCapture) Record(ctx context.Context, e audit.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	apiKey, _ := ctx.Value(contextkeys.APIKeyID).(string)
	c.got = append(c.got, capturedAudit{event: e, origin: resourceaccess.WriteOriginFromContext(ctx), apiKey: apiKey})
}

func (c *auditCapture) events() []capturedAudit {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]capturedAudit(nil), c.got...)
}

// only returns the single recorded event.
func (c *auditCapture) only(t *testing.T) capturedAudit {
	t.Helper()
	got := c.events()
	require.Len(t, got, 1, "exactly one audit event")
	return got[0]
}

// assertNoContentLeak fails when any of the secret strings appears anywhere
// in the event's metadata.
func assertNoContentLeak(t *testing.T, metadata map[string]any, secrets ...string) {
	t.Helper()
	doc, err := json.Marshal(metadata)
	require.NoError(t, err)
	for _, secret := range secrets {
		assert.NotContains(t, string(doc), secret, "resource content must never reach audit metadata")
	}
}

// TestAuditResourceOp_EveryCallSiteNamesARealOperation pins every
// resource-type and verb pair a call site builds to the closed vocabulary: an
// operation outside it would be dropped by the recorder.
func TestAuditResourceOp_EveryCallSiteNamesARealOperation(t *testing.T) {
	pairs := map[string][]string{
		resourceTypePrompt: {
			auditVerbCreate, auditVerbUpdate, auditVerbMove, auditVerbDelete, auditVerbLock, auditVerbUnlock,
		},
		resourceTypeArtifact: {
			auditVerbCreate, auditVerbUpdate, auditVerbMove, auditVerbDelete, auditVerbLock, auditVerbUnlock,
		},
		resourceTypeBlueprint: {
			auditVerbCreate, auditVerbUpdate, auditVerbMove, auditVerbDelete, auditVerbLock, auditVerbUnlock,
		},
		resourceTypeMemory:   {auditVerbCreate, auditVerbUpdate, auditVerbMove, auditVerbDelete},
		resourceTypeFeed:     {auditVerbCreate, auditVerbUpdate, auditVerbDelete},
		resourceTypeFeedItem: {auditVerbCreate, auditVerbDelete, auditVerbArchive, auditVerbUnarchive},
		resourceTypeProject:  {auditVerbCreate, auditVerbUpdate, auditVerbDelete},
	}
	for resourceType, verbs := range pairs {
		for _, verb := range verbs {
			op := auditResourceOp(resourceType, verb)
			assert.True(t, op.IsValid(), "%s is not in the audit vocabulary", op)
		}
	}
	assert.False(t, auditResourceOp(resourceTypeMemory, auditVerbLock).IsValid(), "memories cannot be locked")
}

func TestAuditUpdate(t *testing.T) {
	base := func() *models.Prompt {
		return &models.Prompt{
			ID: "p1", Name: "Release checklist", Slug: "release", Body: "SECRET-BODY", Status: "active",
			ProjectID: "proj-a", Project: &models.ProjectSummary{ID: "proj-a", Name: "Alpha"},
			Labels: []string{"ops"},
		}
	}

	t.Run("a status change records old and new status", func(t *testing.T) {
		after := base()
		after.Status = "archived"

		op, meta := auditUpdate(resourceTypePrompt, promptAuditState(base()), promptAuditState(after))

		assert.Equal(t, models.AuditOpPromptUpdate, op)
		assert.Equal(t, []string{"status"}, meta[auditMetaChangedFields])
		assert.Equal(t, map[string]any{"old": "active", "new": "archived"}, meta["status"])
	})

	t.Run("a rename records old and new name", func(t *testing.T) {
		after := base()
		after.Name = "Launch checklist"

		_, meta := auditUpdate(resourceTypePrompt, promptAuditState(base()), promptAuditState(after))

		assert.Equal(t, map[string]any{"old": "Release checklist", "new": "Launch checklist"}, meta["name"])
	})

	t.Run("a content change records the field name only", func(t *testing.T) {
		after := base()
		after.Body = "SECRET-NEW-BODY"
		after.Labels = []string{"ops", "release"}

		op, meta := auditUpdate(resourceTypePrompt, promptAuditState(base()), promptAuditState(after))

		assert.Equal(t, models.AuditOpPromptUpdate, op)
		assert.Equal(t, []string{"body", "labels"}, meta[auditMetaChangedFields])
		assert.NotContains(t, meta, "body")
		assertNoContentLeak(t, meta, "SECRET-BODY", "SECRET-NEW-BODY")
	})

	t.Run("a project change is a move with both projects", func(t *testing.T) {
		after := base()
		after.ProjectID = "proj-b"
		after.Project = &models.ProjectSummary{ID: "proj-b", Name: "Beta"}
		after.Status = "draft"

		op, meta := auditUpdate(resourceTypePrompt, promptAuditState(base()), promptAuditState(after))

		assert.Equal(t, models.AuditOpPromptMove, op)
		assert.Equal(t, "proj-a", meta[auditMetaFromProjectID])
		assert.Equal(t, "Alpha", meta[auditMetaFromProjectName])
		assert.Equal(t, "proj-b", meta[auditMetaToProjectID])
		assert.Equal(t, "Beta", meta[auditMetaToProjectName])
		assert.Equal(t, []string{"status"}, meta[auditMetaChangedFields], "project_id is the move, not a field")
		assert.Equal(t, map[string]any{"old": "active", "new": "draft"}, meta["status"])
	})

	t.Run("a move whose target name is unknown omits it (no recorder, no lookup)", func(t *testing.T) {
		after := base()
		after.ProjectID = "proj-b"
		after.Project = nil

		_, meta := auditUpdate(resourceTypePrompt, promptAuditState(base()), promptAuditState(after))

		assert.NotContains(t, meta, auditMetaToProjectName)
	})

	t.Run("no change records an empty field list", func(t *testing.T) {
		op, meta := auditUpdate(resourceTypePrompt, promptAuditState(base()), promptAuditState(base()))

		assert.Equal(t, models.AuditOpPromptUpdate, op)
		assert.Equal(t, []string{}, meta[auditMetaChangedFields])
	})

	t.Run("an unknown before records a bare update", func(t *testing.T) {
		op, meta := auditUpdate(resourceTypePrompt, nil, promptAuditState(base()))

		assert.Equal(t, models.AuditOpPromptUpdate, op)
		assert.Empty(t, meta)
	})
}

func TestChangedAuditFields_FieldOnOneSideOnly(t *testing.T) {
	got := changedAuditFields(map[string]string{"a": "1", "gone": "x"}, map[string]string{"a": "1", "new": "y"})
	assert.Equal(t, []string{"gone", "new"}, got)
}

// TestAuditStates_NeverCarryContentIntoMetadata changes the content field of
// every type and requires that only its name reaches the metadata.
func TestAuditStates_NeverCarryContentIntoMetadata(t *testing.T) {
	const before, after = "SECRET-OLD-CONTENT", "SECRET-NEW-CONTENT"
	title := "T"
	cases := []struct {
		name         string
		resourceType string
		field        string
		states       func() (*auditState, *auditState)
	}{
		{"prompt body", resourceTypePrompt, "body", func() (*auditState, *auditState) {
			return promptAuditState(&models.Prompt{Body: before}), promptAuditState(&models.Prompt{Body: after})
		}},
		{"artifact content", resourceTypeArtifact, "content", func() (*auditState, *auditState) {
			return artifactAuditState(&models.Artifact{Content: before}),
				artifactAuditState(&models.Artifact{Content: after})
		}},
		{"artifact metadata", resourceTypeArtifact, "metadata", func() (*auditState, *auditState) {
			return artifactAuditState(&models.Artifact{Metadata: map[string]interface{}{"k": before}}),
				artifactAuditState(&models.Artifact{Metadata: map[string]interface{}{"k": after}})
		}},
		{"blueprint content", resourceTypeBlueprint, "content", func() (*auditState, *auditState) {
			return blueprintAuditState(&models.Blueprint{Content: before}),
				blueprintAuditState(&models.Blueprint{Content: after})
		}},
		{"memory text", resourceTypeMemory, "text", func() (*auditState, *auditState) {
			return memoryAuditState(&models.Memory{Title: &title, Text: before}),
				memoryAuditState(&models.Memory{Title: &title, Text: after})
		}},
		{"feed description", resourceTypeFeed, "description", func() (*auditState, *auditState) {
			b, a := before, after
			return feedAuditState(&models.Feed{Description: &b}), feedAuditState(&models.Feed{Description: &a})
		}},
		{"project description", resourceTypeProject, "description", func() (*auditState, *auditState) {
			return projectAuditState(&models.Project{Description: before}),
				projectAuditState(&models.Project{Description: after})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, a := tc.states()
			_, meta := auditUpdate(tc.resourceType, b, a)

			assert.Equal(t, []string{tc.field}, meta[auditMetaChangedFields])
			assertNoContentLeak(t, meta, before, after)
		})
	}
}

func TestAuditStates_NilResource(t *testing.T) {
	assert.Nil(t, promptAuditState(nil))
	assert.Nil(t, artifactAuditState(nil))
	assert.Nil(t, blueprintAuditState(nil))
	assert.Nil(t, memoryAuditState(nil))
	assert.Nil(t, feedAuditState(nil))
	assert.Nil(t, feedItemAuditState(nil))
	assert.Nil(t, projectAuditState(nil))
}

func TestAuditStates_NamesAndProjects(t *testing.T) {
	subtype := "rule"
	assert.Equal(t, "B", blueprintAuditState(&models.Blueprint{Title: "B", Subtype: &subtype}).name)
	assert.Empty(t, memoryAuditState(&models.Memory{}).name, "a memory may have no title")

	project := projectAuditState(&models.Project{ID: "proj", Name: "Alpha"})
	assert.Equal(t, "proj", project.projectID, "a project event sits in its own project")
	assert.Equal(t, "Alpha", project.projectName)

	projectID := "proj"
	item := feedItemAuditState(&models.FeedItem{Title: "Release", FeedID: "feed-1", ProjectID: &projectID})
	assert.Equal(t, "Release", item.name)
	assert.Equal(t, "proj", item.projectID)
}

func TestAuditRecords(t *testing.T) {
	state := &auditState{projectID: "proj", projectName: "Alpha", name: "Release"}

	created := auditRecord("team", resourceTypePrompt, auditVerbCreate, "p1", state)
	assert.Equal(t, auditedWrite{
		op: models.AuditOpPromptCreate, teamID: "team", projectID: "proj", projectName: "Alpha",
		resourceType: resourceTypePrompt, resourceID: "p1", resourceName: "Release", metadata: map[string]any{},
	}, created)

	deleted := auditDelete("team", resourceTypeMemory, "m1", nil)
	assert.Equal(t, models.AuditOpMemoryDelete, deleted.op)
	assert.Empty(t, deleted.resourceName, "a memory that could not be read first records no name")

	restored := auditRestored("team", resourceTypeArtifact, "a1", state, 3)
	assert.Equal(t, models.AuditOpArtifactUpdate, restored.op)
	assert.Equal(t, map[string]any{auditMetaRestoredVersion: 3}, restored.metadata)

	item := auditFeedItem("team", auditVerbArchive, "i1", feedItemAuditState(&models.FeedItem{FeedID: "feed-1"}))
	assert.Equal(t, models.AuditOpFeedItemArchive, item.op)
	assert.Equal(t, "feed-1", item.metadata["feed_id"])
	assert.NotContains(t, auditFeedItem("team", auditVerbDelete, "i1", nil).metadata, "feed_id")
}

func TestAuditProjectDeleted(t *testing.T) {
	read := auditProjectDeleted("team", "alpha", projectAuditState(&models.Project{ID: "proj", Name: "Alpha"}))
	assert.Equal(t, models.AuditOpProjectDelete, read.op)
	assert.Equal(t, "proj", read.resourceID)
	assert.Equal(t, "Alpha", read.resourceName)
	assert.Equal(t, map[string]any{auditMetaCascade: true}, read.metadata)

	unread := auditProjectDeleted("team", "alpha", nil)
	assert.Empty(t, unread.resourceID)
	assert.Equal(t, "alpha", unread.resourceName, "an unread project is named by its slug")
}

func TestAuditProjectMigrated(t *testing.T) {
	got := auditProjectMigrated("team", "src", "dst", &projectmigration.MigrationResult{
		SourceProjectName: "Alpha", DestinationProjectName: "Beta",
		Migrated: projectmigration.ResourceMigrationCounts{Prompts: 2, Artifacts: 1, Blueprints: 3, FeedItems: 4},
	})

	assert.Equal(t, models.AuditOpProjectMigrated, got.op)
	assert.Equal(t, "src", got.projectID)
	assert.Equal(t, "Alpha", got.projectName)
	assert.Equal(t, map[string]any{"id": "src", "name": "Alpha"}, got.metadata["source_project"])
	assert.Equal(t, map[string]any{"id": "dst", "name": "Beta"}, got.metadata["target_project"])
	assert.Equal(t, map[string]any{"prompts": 2, "artifacts": 1, "blueprints": 3, "feed_items": 4},
		got.metadata["counts"])

	bare := auditProjectMigrated("team", "src", "dst", nil)
	assert.Equal(t, map[string]any{"id": "dst"}, bare.metadata["target_project"])
	assert.NotContains(t, bare.metadata, "counts")
}

func TestRecordAuditWrite_NoRecorderIsSafe(t *testing.T) {
	srv := newServerWithNullLogger(t)
	srv.container = &BaseMockContainer{}
	assert.NotPanics(t, func() { srv.recordAuditWrite(context.Background(), auditedWrite{}) })

	srv.container = nil
	assert.Nil(t, srv.auditRecorder())
}

// The before-reads cost nothing when no recorder would use them: they return
// nil without touching a service (the mock container has none to touch).
func TestAuditBefore_SkippedWithoutRecorder(t *testing.T) {
	srv := newServerWithNullLogger(t)
	srv.container = &BaseMockContainer{}
	ctx := context.Background()

	assert.Nil(t, srv.promptAuditBefore("u", "t", "s"))
	assert.Nil(t, srv.artifactAuditBefore("u", "t", "p", "s"))
	assert.Nil(t, srv.blueprintAuditBefore("u", "t", "p", "s"))
	assert.Nil(t, srv.memoryAuditBefore("u", "t", "m"))
	assert.Nil(t, srv.feedAuditBefore(ctx, "u", "t", "f"))
	assert.Nil(t, srv.feedItemAuditBefore(ctx, "u", "t", "i"))
	assert.Nil(t, srv.projectAuditBefore("t", "u", "s"))
}

func TestAnnotateWriteSourceOnWrites(t *testing.T) {
	srv := newServerWithNullLogger(t)
	var origin *models.WriteOrigin
	handler := srv.annotateWriteSourceOnWrites(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		origin = resourceaccess.WriteOriginFromContext(r.Context())
	}))

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		origin = nil
		req := httptest.NewRequest(method, "/api/v1/t/prompts/p/lock", nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)
		assert.Nil(t, origin, "%s is a read and is not annotated", method)
	}

	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		origin = nil
		req := httptest.NewRequest(method, "/api/v1/t/prompts/p/lock", strings.NewReader(""))
		req.Header.Set("User-Agent", writeTestCLIAgent)
		req = req.WithContext(context.WithValue(req.Context(), contextkeys.AuthType, "api_key"))
		handler.ServeHTTP(httptest.NewRecorder(), req)
		require.NotNil(t, origin, "%s can write and is annotated", method)
		assert.Equal(t, resourceaccess.SourceCLI, origin.Source)
	}
}
