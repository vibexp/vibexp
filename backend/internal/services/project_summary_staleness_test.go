package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// The four update paths load the resource through the DETAIL read, which since
// #929 resolves `project` via a LEFT JOIN. `project_id` is a settable field, so
// an update that MOVES the resource would otherwise answer with the new
// `project_id` beside the OLD project's summary — a payload that contradicts
// itself, and which the detail page renders as the wrong project name.
//
// Nothing else catches this: the summary is loaded by the repository and the
// spec cannot express "these two fields agree", so the assertion has to be here.

func oldProjectSummary() *models.ProjectSummary {
	return &models.ProjectSummary{ID: "old-project", Name: "Old Project", Slug: "old-project"}
}

func TestApplyArtifactUpdates_MovingProjectDropsTheStaleSummary(t *testing.T) {
	newProject := "new-project"

	moved := &models.Artifact{ProjectID: "old-project", Project: oldProjectSummary()}
	applyArtifactUpdates(moved, &models.UpdateArtifactRequest{ProjectID: &newProject})
	require.Equal(t, newProject, moved.ProjectID)
	assert.Nil(t, moved.Project, "a moved artifact must not report the project it left")

	// An update that does NOT move it keeps the summary it was loaded with.
	title := "Retitled"
	kept := &models.Artifact{ProjectID: "old-project", Project: oldProjectSummary()}
	applyArtifactUpdates(kept, &models.UpdateArtifactRequest{Title: &title})
	assert.Equal(t, oldProjectSummary(), kept.Project)
}

func TestApplyBlueprintUpdates_MovingProjectDropsTheStaleSummary(t *testing.T) {
	newProject := "new-project"

	moved := &models.Blueprint{ProjectID: "old-project", Project: oldProjectSummary()}
	applyBlueprintUpdates(moved, &models.UpdateBlueprintRequest{ProjectID: &newProject})
	require.Equal(t, newProject, moved.ProjectID)
	assert.Nil(t, moved.Project, "a moved blueprint must not report the project it left")

	title := "Retitled"
	kept := &models.Blueprint{ProjectID: "old-project", Project: oldProjectSummary()}
	applyBlueprintUpdates(kept, &models.UpdateBlueprintRequest{Title: &title})
	assert.Equal(t, oldProjectSummary(), kept.Project)
}

func TestApplyMemoryUpdates_MovingProjectDropsTheStaleSummary(t *testing.T) {
	newProject := "new-project"

	moved := &models.Memory{ProjectID: "old-project", Project: oldProjectSummary()}
	applyMemoryUpdates(moved, &models.UpdateMemoryRequest{ProjectID: &newProject})
	require.Equal(t, newProject, moved.ProjectID)
	assert.Nil(t, moved.Project, "a moved memory must not report the project it left")

	text := "rewritten"
	kept := &models.Memory{ProjectID: "old-project", Project: oldProjectSummary()}
	applyMemoryUpdates(kept, &models.UpdateMemoryRequest{Text: &text})
	assert.Equal(t, oldProjectSummary(), kept.Project)
}
