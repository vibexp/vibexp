package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/vibexp/vibexp/internal/repositories"
)

// Attachment owner-authorization errors.
var (
	// ErrAttachmentOwnerTypeUnknown is returned by the registry when no
	// authorizer is registered for an owner_type. The HTTP layer maps it to 404,
	// so the registry doubles as the allowlist of supported owner types: an
	// unregistered owner_type is indistinguishable from a non-existent route.
	ErrAttachmentOwnerTypeUnknown = errors.New("unknown attachment owner_type")
	// ErrAttachmentOwnerAccessDenied is returned by an authorizer when the user
	// may not access the owning resource (or it does not exist). Both cases
	// collapse to this one sentinel so the HTTP layer never leaks owner existence.
	ErrAttachmentOwnerAccessDenied = errors.New("attachment owner access denied")
)

// AttachmentOwnerAuthorizer verifies that userID may manage attachments of the
// resource identified by (teamID, ownerID) for a given owner_type. It returns nil
// when access is allowed and ErrAttachmentOwnerAccessDenied otherwise. Making a new
// resource type attachable is exactly one of these — no new endpoints, handlers, or
// service code below the wiring layer.
type AttachmentOwnerAuthorizer func(ctx context.Context, userID, teamID, ownerID string) error

// AttachmentAuthorizerRegistry maps owner_type to its authorizer. It is the single
// allowlist of attachable resource types for the universal attachments endpoint.
type AttachmentAuthorizerRegistry struct {
	authorizers map[string]AttachmentOwnerAuthorizer
}

// NewAttachmentAuthorizerRegistry returns an empty registry. Callers register one
// authorizer per supported owner_type at wiring time.
func NewAttachmentAuthorizerRegistry() *AttachmentAuthorizerRegistry {
	return &AttachmentAuthorizerRegistry{authorizers: make(map[string]AttachmentOwnerAuthorizer)}
}

// Register adds (or replaces) the authorizer for an owner_type.
func (r *AttachmentAuthorizerRegistry) Register(ownerType string, fn AttachmentOwnerAuthorizer) {
	r.authorizers[ownerType] = fn
}

// Authorize runs the registered authorizer for ownerType, returning
// ErrAttachmentOwnerTypeUnknown when the type is not registered.
func (r *AttachmentAuthorizerRegistry) Authorize(
	ctx context.Context, ownerType, userID, teamID, ownerID string,
) error {
	fn, ok := r.authorizers[ownerType]
	if !ok {
		return fmt.Errorf("%q: %w", ownerType, ErrAttachmentOwnerTypeUnknown)
	}
	return fn(ctx, userID, teamID, ownerID)
}

// NewArtifactAttachmentAuthorizer builds the owner_type="artifact" authorizer. It
// reuses the same team-scoped artifact access check as the artifact routes, so a
// user may only touch attachments of artifacts they can already access; "not found"
// and "forbidden" both surface as ErrAttachmentOwnerAccessDenied (no existence leak).
func NewArtifactAttachmentAuthorizer(artifacts ArtifactServiceInterface) AttachmentOwnerAuthorizer {
	return func(_ context.Context, userID, teamID, ownerID string) error {
		_, err := artifacts.GetArtifactByIDInTeam(userID, teamID, ownerID)
		if err == nil {
			return nil
		}
		if errors.Is(err, repositories.ErrArtifactNotFound) {
			// Missing and forbidden are indistinguishable here (the lookup is
			// team-membership scoped), so collapse both to access-denied — the HTTP
			// layer renders 404 and never leaks owner existence.
			return fmt.Errorf("artifact %s: %w", ownerID, ErrAttachmentOwnerAccessDenied)
		}
		// A real lookup failure (e.g. a DB error) must surface as 500, not be masked
		// as a 404 — matching the artifact route's error handling.
		return fmt.Errorf("authorize artifact attachment owner %s: %w", ownerID, err)
	}
}

// NewPromptAttachmentAuthorizer builds the owner_type="prompt" authorizer. It
// reuses the same team-scoped prompt access check as the prompt routes, so a user
// may only touch attachments of prompts they can already access; "not found" and
// "forbidden" both surface as ErrAttachmentOwnerAccessDenied (no existence leak).
func NewPromptAttachmentAuthorizer(prompts PromptServiceInterface) AttachmentOwnerAuthorizer {
	return func(_ context.Context, userID, teamID, ownerID string) error {
		_, err := prompts.GetPrompt(userID, teamID, ownerID)
		if err == nil {
			return nil
		}
		if errors.Is(err, repositories.ErrPromptNotFound) {
			// Missing and forbidden are indistinguishable here (the lookup is
			// team-membership scoped), so collapse both to access-denied — the HTTP
			// layer renders 404 and never leaks owner existence.
			return fmt.Errorf("prompt %s: %w", ownerID, ErrAttachmentOwnerAccessDenied)
		}
		// A real lookup failure (e.g. a DB error) must surface as 500, not be masked
		// as a 404 — matching the prompt route's error handling.
		return fmt.Errorf("authorize prompt attachment owner %s: %w", ownerID, err)
	}
}

// NewBlueprintAttachmentAuthorizer builds the owner_type="blueprint" authorizer. It
// reuses the same team-scoped blueprint access check the blueprint routes rely on, so a
// user may only touch attachments of blueprints they can already access; "not found"
// and "forbidden" both surface as ErrAttachmentOwnerAccessDenied (no existence leak).
func NewBlueprintAttachmentAuthorizer(blueprints BlueprintServiceInterface) AttachmentOwnerAuthorizer {
	return func(_ context.Context, userID, teamID, ownerID string) error {
		_, err := blueprints.GetBlueprintByIDInTeam(userID, teamID, ownerID)
		if err == nil {
			return nil
		}
		if errors.Is(err, repositories.ErrBlueprintNotFound) {
			// Missing and forbidden are indistinguishable here (the lookup is
			// team-membership scoped), so collapse both to access-denied — the HTTP
			// layer renders 404 and never leaks owner existence.
			return fmt.Errorf("blueprint %s: %w", ownerID, ErrAttachmentOwnerAccessDenied)
		}
		// A real lookup failure (e.g. a DB error) must surface as 500, not be masked
		// as a 404 — matching the blueprint route's error handling.
		return fmt.Errorf("authorize blueprint attachment owner %s: %w", ownerID, err)
	}
}
