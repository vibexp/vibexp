package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
)

// lockResourceToolDescription documents the single generic lock tool (#1296).
// One tool covers the three lockable resource types, like the delete tool, so
// the MCP tool list stays small. There is deliberately no unlock counterpart:
// an agent can place a guard, only a person can remove one.
const lockResourceToolDescription = "Lock a single resource so it cannot be changed or deleted until a human " +
	"unlocks it. Supported resource_type values and their required identifiers: \"prompt\" needs slug; " +
	"\"artifact\" and \"blueprint\" need project_id and slug. A comment saying why the resource is locked is " +
	"required and is shown to the team. There is no unlock tool: a human unlocks in the VibeXP UI or through " +
	"the REST API."

// LockResourceParams defines the parameters for the generic lock_resource tool.
// The required identifier fields vary by resource_type and are validated per type.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type LockResourceParams struct {
	TeamID       string `json:"team_id" jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	ResourceType string `json:"resource_type" jsonschema:"REQUIRED. The resource type to lock: one of \"prompt\", \"blueprint\", or \"artifact\"."`
	ProjectID    string `json:"project_id,omitempty" jsonschema:"Project UUID. REQUIRED when resource_type is \"artifact\" or \"blueprint\"; ignored otherwise."`
	Slug         string `json:"slug" jsonschema:"REQUIRED. Slug of the resource to lock."`
	Comment      string `json:"comment" jsonschema:"REQUIRED. Why the resource is locked, shown to the team next to the lock. At most 500 characters."`
}

// lockResourceResponse is the slim, structured result returned on a successful
// lock. It carries no lock object: lock state stays out of MCP payloads (#1288).
type lockResourceResponse struct {
	Locked       bool   `json:"locked"`
	ResourceType string `json:"resource_type"`
	ID           string `json:"id"`
	ProjectID    string `json:"project_id,omitempty"`
	Slug         string `json:"slug"`
}

const lockResourceToolName = "vibexp_io_lock_resource"

// lockableResource is what the lock tool needs from a loaded prompt, blueprint
// or artifact: the id the lock service takes and the lock it already carries.
type lockableResource struct {
	id   string
	lock *models.ResourceLock
}

// lockResourceTool implements the generic lock tool. It resolves and
// membership-checks the team, loads the resource through the same team-scoped
// getter the delete tool uses, then locks it through lockResource, the helper
// behind the REST lock endpoints, so the rules, the history event and the
// activity record are the same from either entry point.
func (s *Server) lockResourceTool(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *LockResourceParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	resourceType := strings.ToLower(strings.TrimSpace(params.ResourceType))
	slug := strings.TrimSpace(params.Slug)
	if r := validateLockIdentifiers(resourceType, params.ProjectID, slug); r != nil {
		return r, nil, nil
	}

	resource, err := s.loadLockableResource(userID, teamID, resourceType, params.ProjectID, slug)
	if err != nil {
		return mcpLockLoadError(resourceType, userID, teamID, err), nil, nil
	}

	target := lockTarget{resourceType: resourceType, teamID: teamID, resourceID: resource.id, slug: slug}
	if err := s.lockResource(ctx, userID, target, params.Comment); err != nil {
		return mcpTextError(s.lockFailureMessage(err, userID, params.ProjectID, target, resource.lock)), nil, nil
	}

	result := &lockResourceResponse{Locked: true, ResourceType: resourceType, ID: resource.id, Slug: slug}
	if resourceType != resourceTypePrompt {
		result.ProjectID = params.ProjectID
	}
	return mcpJSONResult(result)
}

// validateLockIdentifiers rejects an unsupported resource type and a missing
// identifier before anything is loaded. The lock rules themselves (comment,
// permission, conflict) belong to the service.
func validateLockIdentifiers(resourceType, projectID, slug string) *mcp.CallToolResult {
	switch resourceType {
	case resourceTypePrompt:
	case resourceTypeArtifact, resourceTypeBlueprint:
		if r := validateProjectID(projectID); r != nil {
			return r
		}
	default:
		return mcpTextError(fmt.Sprintf(
			"resource_type must be one of: %s, %s, %s",
			resourceTypePrompt, resourceTypeBlueprint, resourceTypeArtifact,
		))
	}
	if slug == "" {
		return mcpTextError("slug is required to lock a " + resourceType)
	}
	return nil
}

// loadLockableResource loads the resource through its team-scoped getter.
// resourceType has already passed validateLockIdentifiers.
func (s *Server) loadLockableResource(
	userID, teamID, resourceType, projectID, slug string,
) (*lockableResource, error) {
	switch resourceType {
	case resourceTypePrompt:
		prompt, err := s.container.PromptService().GetPromptBySlug(userID, teamID, slug)
		if err != nil {
			return nil, err
		}
		return &lockableResource{id: prompt.ID, lock: prompt.Lock}, nil
	case resourceTypeBlueprint:
		blueprint, err := s.container.BlueprintService().GetBlueprintByProjectIDAndSlugInTeam(
			userID, teamID, projectID, slug,
		)
		if err != nil {
			return nil, err
		}
		return &lockableResource{id: blueprint.ID, lock: blueprint.Lock}, nil
	default:
		artifact, err := s.container.ArtifactService().GetArtifactByProjectIDAndSlugInTeam(
			userID, teamID, projectID, slug,
		)
		if err != nil {
			return nil, err
		}
		return &lockableResource{id: artifact.ID, lock: artifact.Lock}, nil
	}
}

// lockFailureMessage turns a lockResource error into the text the agent reads.
// lockResource has already mapped and, where needed, logged it, so the API
// error's detail is the message. For the "already locked" conflict the detail
// alone would leave the agent guessing, so the existing lock's holder and
// comment are appended; held is the lock the resource carried when it was
// loaded, and a nil one means another caller locked it in between, so the
// resource is read once more.
func (s *Server) lockFailureMessage(
	err error, userID, projectID string, target lockTarget, held *models.ResourceLock,
) string {
	var apiErr *apierrors.APIError
	if !errors.As(err, &apiErr) {
		return fmt.Sprintf("Failed to lock %s: %v", target.resourceType, err)
	}
	if apiErr.Code != apierrors.CodeResourceLocked {
		return apiErr.Detail
	}

	if held == nil {
		reloaded, loadErr := s.loadLockableResource(userID, target.teamID, target.resourceType, projectID, target.slug)
		if loadErr == nil {
			held = reloaded.lock
		}
	}
	return apiErr.Detail + describeHeldLock(held) + ". There is no unlock tool: a human unlocks in the VibeXP UI."
}

// describeHeldLock renders the holder and comment of an existing lock as a
// continuation of the "already locked" sentence. It returns "" when the lock
// could not be read, which leaves the bare conflict message.
func describeHeldLock(held *models.ResourceLock) string {
	if held == nil {
		return ""
	}
	description := ""
	if held.LockedByName != "" {
		description = " by " + held.LockedByName
	}
	return description + fmt.Sprintf(" with the comment %q", held.Comment)
}

// mcpLockLoadError logs a failure to load the resource to lock and returns the
// IsError result to hand back.
func mcpLockLoadError(resourceType, userID, teamID string, err error) *mcp.CallToolResult {
	slog.Error(
		"Failed to load resource to lock via MCP",
		"tool", lockResourceToolName,
		"resource_type", resourceType,
		"user_id", userID,
		"team_id", teamID,
		"error", fmt.Sprintf("%+v", err),
	)
	return mcpTextError(fmt.Sprintf("Failed to lock %s: %v", resourceType, err))
}
