package server

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/projectmigration"
)

// Audit log records of resource writes (#1513, epic #1510).
//
// A write is recorded where it arrives (the REST handlers and MCP tools), not
// in the services: the content services take no context, so they cannot see
// the actor, the channel or the API key. This file holds what every call site
// shares: the operation names and the before/after comparison that turns an
// update into a move or an update with its changed fields.

// resourceTypeFeed and resourceTypeFeedItem are the audit resource types of
// feeds and feed items. The other types reuse the resource-access constants.
const (
	resourceTypeFeed     = "feed"
	resourceTypeFeedItem = "feed_item"
)

// Audit verbs. A resource's audit operation is "<resource type>.<verb>"; the
// combinations that exist are the models.AuditOp* constants, which
// TestAuditResourceOp_EveryCallSiteNamesARealOperation pins.
const (
	auditVerbCreate    = "create"
	auditVerbUpdate    = "update"
	auditVerbMove      = "move"
	auditVerbDelete    = "delete"
	auditVerbLock      = "lock"
	auditVerbUnlock    = "unlock"
	auditVerbArchive   = "archive"
	auditVerbUnarchive = "unarchive"
)

// auditResourceOp names the audit operation of verb on resourceType.
func auditResourceOp(resourceType, verb string) models.AuditOperation {
	return models.AuditOperation(resourceType + "." + verb)
}

// Metadata keys of a resource-write audit event.
const (
	auditMetaChangedFields   = "changed_fields"
	auditMetaFromProjectID   = "from_project_id"
	auditMetaFromProjectName = "from_project_name"
	auditMetaToProjectID     = "to_project_id"
	auditMetaToProjectName   = "to_project_name"
	auditMetaRestoredVersion = "restored_version"
	auditMetaVia             = "via"
	auditMetaCascade         = "cascade"
	auditMetaOld             = "old"
	auditMetaNew             = "new"
)

// auditViaGitHubImport marks a write made by a GitHub import.
const auditViaGitHubImport = "github_import"

// Field names compared by auditState. auditFieldStatus and the resource's
// name field are the only ones whose values are recorded.
const (
	auditFieldStatus    = "status"
	auditFieldName      = "name"
	auditFieldTitle     = "title"
	auditFieldProjectID = "project_id"
)

// auditState is what an update is compared on: a resource's project, its
// display name, and the value of every field a write can change. Values in
// fields are compared and never recorded, except status and the name field;
// for a content field only the field's name can reach the audit log.
type auditState struct {
	projectID   string
	projectName string
	// nameField is the field that holds the display name: "name" for prompts,
	// feeds and projects, "title" for the other types.
	nameField string
	name      string
	fields    map[string]string
}

// auditUpdate compares a resource before and after an update and returns the
// operation to record and its metadata.
//
// A changed project makes the write a move, with the projects on both sides.
// Otherwise it is an update listing the names of the changed fields, plus the
// old and new values of the status and the display name. A nil before (the
// resource could not be read ahead of the update) records a bare update.
func auditUpdate(resourceType string, before *auditState, after *auditState) (models.AuditOperation, map[string]any) {
	if before == nil || after == nil {
		return auditResourceOp(resourceType, auditVerbUpdate), map[string]any{}
	}

	changed := changedAuditFields(before.fields, after.fields)
	metadata := map[string]any{}
	op := auditResourceOp(resourceType, auditVerbUpdate)

	if before.projectID != after.projectID {
		op = auditResourceOp(resourceType, auditVerbMove)
		metadata[auditMetaFromProjectID] = before.projectID
		metadata[auditMetaToProjectID] = after.projectID
		if before.projectName != "" {
			metadata[auditMetaFromProjectName] = before.projectName
		}
		if after.projectName != "" {
			metadata[auditMetaToProjectName] = after.projectName
		}
		changed = slices.DeleteFunc(changed, func(f string) bool { return f == auditFieldProjectID })
	}

	metadata[auditMetaChangedFields] = changed
	for _, field := range []string{auditFieldStatus, before.nameField} {
		if slices.Contains(changed, field) {
			metadata[field] = map[string]any{auditMetaOld: before.fields[field], auditMetaNew: after.fields[field]}
		}
	}
	return op, metadata
}

// changedAuditFields returns the sorted names of the fields whose values
// differ, including a field present on one side only.
func changedAuditFields(before, after map[string]string) []string {
	changed := []string{}
	for _, field := range slices.Sorted(maps.Keys(after)) {
		if old, ok := before[field]; !ok || old != after[field] {
			changed = append(changed, field)
		}
	}
	for field := range before {
		if _, ok := after[field]; !ok {
			changed = append(changed, field)
		}
	}
	slices.Sort(changed)
	return changed
}

func projectSummaryName(p *models.ProjectSummary) string {
	if p == nil {
		return ""
	}
	return p.Name
}

// auditLabels renders labels for comparison. Order counts: reordering labels
// is an update.
func auditLabels(labels []string) string {
	return strings.Join(labels, "\x00")
}

// auditJSON renders a metadata map for comparison. encoding/json sorts map
// keys, so equal maps render equal.
func auditJSON(v map[string]interface{}) string {
	if len(v) == 0 {
		return ""
	}
	doc, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(doc)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func promptAuditState(p *models.Prompt) *auditState {
	if p == nil {
		return nil
	}
	return &auditState{
		projectID: p.ProjectID, projectName: projectSummaryName(p.Project),
		nameField: auditFieldName, name: p.Name,
		fields: map[string]string{
			auditFieldName: p.Name, "slug": p.Slug, "description": p.Description, "body": p.Body,
			auditFieldStatus: p.Status, "labels": auditLabels(p.Labels),
			"mcp_expose": strconv.FormatBool(p.MCPExpose), auditFieldProjectID: p.ProjectID,
		},
	}
}

func artifactAuditState(a *models.Artifact) *auditState {
	if a == nil {
		return nil
	}
	return &auditState{
		projectID: a.ProjectID, projectName: projectSummaryName(a.Project),
		nameField: auditFieldTitle, name: a.Title,
		fields: map[string]string{
			auditFieldTitle: a.Title, "slug": a.Slug, "description": a.Description, "content": a.Content,
			auditFieldStatus: a.Status, "type": a.Type, "labels": auditLabels(a.Labels),
			"metadata": auditJSON(a.Metadata), auditFieldProjectID: a.ProjectID,
		},
	}
}

func blueprintAuditState(b *models.Blueprint) *auditState {
	if b == nil {
		return nil
	}
	return &auditState{
		projectID: b.ProjectID, projectName: projectSummaryName(b.Project),
		nameField: auditFieldTitle, name: b.Title,
		fields: map[string]string{
			auditFieldTitle: b.Title, "slug": b.Slug, "description": b.Description, "content": b.Content,
			auditFieldStatus: b.Status, "type": b.Type, "subtype": derefString(b.Subtype), "path": b.Path,
			"labels": auditLabels(b.Labels), "metadata": auditJSON(b.Metadata), auditFieldProjectID: b.ProjectID,
		},
	}
}

func memoryAuditState(m *models.Memory) *auditState {
	if m == nil {
		return nil
	}
	return &auditState{
		projectID: m.ProjectID, projectName: projectSummaryName(m.Project),
		nameField: auditFieldTitle, name: derefString(m.Title),
		fields: map[string]string{
			auditFieldTitle: derefString(m.Title), "text": m.Text, auditFieldStatus: m.Status,
			"labels": auditLabels(m.Labels), "metadata": auditJSON(m.Metadata), auditFieldProjectID: m.ProjectID,
		},
	}
}

func feedAuditState(f *models.Feed) *auditState {
	if f == nil {
		return nil
	}
	return &auditState{
		nameField: auditFieldName, name: f.Name,
		fields: map[string]string{auditFieldName: f.Name, "description": derefString(f.Description)},
	}
}

func projectAuditState(p *models.Project) *auditState {
	if p == nil {
		return nil
	}
	return &auditState{
		projectID: p.ID, projectName: p.Name,
		nameField: auditFieldName, name: p.Name,
		fields: map[string]string{
			auditFieldName: p.Name, "slug": p.Slug, "description": p.Description,
			"git_url": p.GitURL, "homepage": p.Homepage,
		},
	}
}

// auditCreate is the audit record of a resource that a write created.
func auditCreate(teamID, resourceType, id string, state *auditState) auditedWrite {
	w := auditedWrite{
		op: auditResourceOp(resourceType, auditVerbCreate), teamID: teamID,
		resourceType: resourceType, resourceID: id, metadata: map[string]any{},
	}
	if state != nil {
		w.projectID, w.projectName, w.resourceName = state.projectID, state.projectName, state.name
	}
	return w
}

// auditDelete is the audit record of a deleted resource; state is the
// resource as it was read before the delete, for its name and project.
func auditDelete(teamID, resourceType, id string, state *auditState) auditedWrite {
	w := auditCreate(teamID, resourceType, id, state)
	w.op = auditResourceOp(resourceType, auditVerbDelete)
	return w
}

// auditUpdated is the audit record of an update: a move or an update with its
// changed fields (see auditUpdate). The project and name are the resource's
// after the write.
func auditUpdated(teamID, resourceType, id string, before, after *auditState) auditedWrite {
	w := auditCreate(teamID, resourceType, id, after)
	w.op, w.metadata = auditUpdate(resourceType, before, after)
	return w
}

// auditBlueprintUpdated is auditUpdated for a blueprint written by an update.
func auditBlueprintUpdated(teamID string, before *auditState, b *models.Blueprint) auditedWrite {
	return auditUpdated(teamID, resourceTypeBlueprint, b.ID, before, blueprintAuditState(b))
}

// auditRestored is the audit record of a version restore: an update naming
// the version it went back to. The restored content itself is not compared.
func auditRestored(teamID, resourceType, id string, state *auditState, version int) auditedWrite {
	w := auditCreate(teamID, resourceType, id, state)
	w.op = auditResourceOp(resourceType, auditVerbUpdate)
	w.metadata = map[string]any{auditMetaRestoredVersion: version}
	return w
}

// The *AuditBefore helpers read a resource ahead of an update, for auditUpdate
// to compare against. They read only when an audit recorder is wired, so an
// unaudited server pays no extra query, and they return nil on any error: the
// update then reports its own error, or records a bare update.

func (s *Server) promptAuditBefore(userID, teamID, slug string) *auditState {
	if s.auditRecorder() == nil {
		return nil
	}
	p, err := s.container.PromptService().GetPromptBySlug(userID, teamID, slug)
	if err != nil {
		return nil
	}
	return promptAuditState(p)
}

func (s *Server) artifactAuditBefore(userID, teamID, projectID, slug string) *auditState {
	if s.auditRecorder() == nil {
		return nil
	}
	a, err := s.container.ArtifactService().GetArtifactByProjectIDAndSlugInTeam(userID, teamID, projectID, slug)
	if err != nil {
		return nil
	}
	return artifactAuditState(a)
}

func (s *Server) blueprintAuditBefore(userID, teamID, projectID, slug string) *auditState {
	if s.auditRecorder() == nil {
		return nil
	}
	b, err := s.container.BlueprintService().GetBlueprintByProjectIDAndSlugInTeam(userID, teamID, projectID, slug)
	if err != nil {
		return nil
	}
	return blueprintAuditState(b)
}

func (s *Server) memoryAuditBefore(userID, teamID, memoryID string) *auditState {
	if s.auditRecorder() == nil {
		return nil
	}
	m, err := s.container.MemoryService().GetMemory(userID, teamID, memoryID)
	if err != nil {
		return nil
	}
	return memoryAuditState(m)
}

// feedItemAuditState is a feed item's project and title. Feed items are not
// updated, so no fields are compared.
func feedItemAuditState(item *models.FeedItem) *auditState {
	if item == nil {
		return nil
	}
	return &auditState{
		projectID: derefString(item.ProjectID), nameField: auditFieldTitle, name: item.Title,
		fields: map[string]string{"feed_id": item.FeedID},
	}
}

// auditFeedItem is the audit record of verb on a feed item. Its metadata names
// the feed the item belongs to.
func auditFeedItem(teamID, verb, id string, state *auditState) auditedWrite {
	w := auditCreate(teamID, resourceTypeFeedItem, id, state)
	w.op = auditResourceOp(resourceTypeFeedItem, verb)
	if state != nil && state.fields["feed_id"] != "" {
		w.metadata["feed_id"] = state.fields["feed_id"]
	}
	return w
}

func (s *Server) feedAuditBefore(ctx context.Context, userID, teamID, feedID string) *auditState {
	if s.auditRecorder() == nil {
		return nil
	}
	f, err := s.container.FeedService().GetFeed(ctx, userID, teamID, feedID)
	if err != nil {
		return nil
	}
	return feedAuditState(f)
}

func (s *Server) feedItemAuditBefore(ctx context.Context, userID, teamID, itemID string) *auditState {
	if s.auditRecorder() == nil {
		return nil
	}
	item, err := s.container.FeedItemService().GetFeedItem(ctx, userID, teamID, itemID)
	if err != nil {
		return nil
	}
	return feedItemAuditState(item)
}

func (s *Server) projectAuditBefore(teamID, userID, slug string) *auditState {
	if s.auditRecorder() == nil {
		return nil
	}
	p, err := s.container.ProjectService().GetProjectBySlugOrID(teamID, userID, slug)
	if err != nil {
		return nil
	}
	return projectAuditState(p)
}

// auditProjectDeleted is the audit record of a project delete. A project that
// could not be read first is still recorded, under the slug it was deleted by.
func auditProjectDeleted(teamID, slug string, before *auditState) auditedWrite {
	id := ""
	if before != nil {
		id = before.projectID
	}
	w := auditDelete(teamID, resourceTypeProject, id, before)
	if before == nil {
		w.resourceName = slug
	}
	w.metadata[auditMetaCascade] = true
	return w
}

// auditProjectMigrated is the audit record of a project migration: one event
// on the source project naming both projects and the counts moved. The
// migrated resources get no rows of their own.
func auditProjectMigrated(
	teamID, sourceID, targetID string, result *projectmigration.MigrationResult,
) auditedWrite {
	w := auditedWrite{
		op: models.AuditOpProjectMigrated, teamID: teamID,
		projectID: sourceID, resourceType: resourceTypeProject, resourceID: sourceID,
		metadata: map[string]any{
			"source_project": map[string]any{"id": sourceID},
			"target_project": map[string]any{"id": targetID},
		},
	}
	if result == nil {
		return w
	}
	w.projectName, w.resourceName = result.SourceProjectName, result.SourceProjectName
	w.metadata["source_project"] = map[string]any{"id": sourceID, "name": result.SourceProjectName}
	w.metadata["target_project"] = map[string]any{"id": targetID, "name": result.DestinationProjectName}
	w.metadata["counts"] = map[string]any{
		"prompts": result.Migrated.Prompts, "artifacts": result.Migrated.Artifacts,
		"blueprints": result.Migrated.Blueprints, "feed_items": result.Migrated.FeedItems,
	}
	return w
}
