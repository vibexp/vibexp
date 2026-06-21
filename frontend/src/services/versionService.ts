import type { VersionHistorySource } from '@/features/version-history'

import { artifactService } from './artifactService'
import { blueprintService } from './blueprintService'
import { memoryService } from './memoryService'
import { promptService } from './promptService'

// Resource-agnostic version-history wiring. Each resource type provides a
// `VersionHistorySource` the generic `VersionHistoryPage` consumes; Artifacts are
// the first adopter, Blueprints the second, Memory the third, Prompts the fourth.
// Adding a resource = another factory here + a route, not a rewrite of the UI.

interface ResourceVersionSourceArgs {
  teamId: string
  projectId: string
  slug: string
  // react-router target for the "Back to <resource>" button.
  backHref: string
}

type ArtifactVersionSourceArgs = ResourceVersionSourceArgs

export function createArtifactVersionSource(
  args: ArtifactVersionSourceArgs
): VersionHistorySource {
  const { teamId, projectId, slug, backHref } = args
  return {
    resourceType: 'artifact',
    resourceLabel: 'artifact',
    backHref,
    load: async () => {
      const [artifact, history] = await Promise.all([
        artifactService.getArtifact(teamId, projectId, slug),
        artifactService.getArtifactVersions(teamId, projectId, slug),
      ])
      return {
        currentContent: artifact.content ?? '',
        currentUpdatedAt: artifact.updated_at,
        resourceName: artifact.title,
        versions: history.versions,
      }
    },
    restore: async (versionNumber: number) => {
      await artifactService.restoreArtifactVersion(
        teamId,
        projectId,
        slug,
        versionNumber
      )
    },
  }
}

type BlueprintVersionSourceArgs = ResourceVersionSourceArgs

export function createBlueprintVersionSource(
  args: BlueprintVersionSourceArgs
): VersionHistorySource {
  const { teamId, projectId, slug, backHref } = args
  return {
    resourceType: 'blueprint',
    resourceLabel: 'blueprint',
    backHref,
    load: async () => {
      const [blueprint, history] = await Promise.all([
        blueprintService.getBlueprint(teamId, projectId, slug),
        blueprintService.getBlueprintVersions(teamId, projectId, slug),
      ])
      return {
        currentContent: blueprint.content,
        currentUpdatedAt: blueprint.updated_at,
        resourceName: blueprint.title,
        versions: history.versions,
      }
    },
    restore: async (versionNumber: number) => {
      await blueprintService.restoreBlueprintVersion(
        teamId,
        projectId,
        slug,
        versionNumber
      )
    },
  }
}

// Memory is addressed by id (no project/slug, no title), so its source args differ
// from the project/slug resources above.
interface MemoryVersionSourceArgs {
  teamId: string
  id: string
  // react-router target for the "Back to memory" button.
  backHref: string
}

export function createMemoryVersionSource(
  args: MemoryVersionSourceArgs
): VersionHistorySource {
  const { teamId, id, backHref } = args
  return {
    resourceType: 'memory',
    resourceLabel: 'memory',
    backHref,
    load: async () => {
      const [memory, history] = await Promise.all([
        memoryService.getMemory(teamId, id),
        memoryService.getMemoryVersions(teamId, id),
      ])
      return {
        currentContent: memory.text,
        currentUpdatedAt: memory.updated_at,
        // Memory has no title; derive a stable display name from its id.
        resourceName: `Memory #${id}`,
        versions: history.versions,
      }
    },
    restore: async (versionNumber: number) => {
      await memoryService.restoreMemoryVersion(teamId, id, versionNumber)
    },
  }
}

// Prompts are addressed by slug within a team (no project id), so the source args
// differ from the project/slug resources above.
interface PromptVersionSourceArgs {
  teamId: string
  slug: string
  // react-router target for the "Back to prompt" button.
  backHref: string
}

export function createPromptVersionSource(
  args: PromptVersionSourceArgs
): VersionHistorySource {
  const { teamId, slug, backHref } = args
  return {
    resourceType: 'prompt',
    resourceLabel: 'prompt',
    backHref,
    load: async () => {
      const [prompt, history] = await Promise.all([
        promptService.getPrompt(teamId, slug),
        promptService.getPromptVersions(teamId, slug),
      ])
      return {
        // Diff the raw body template (placeholders / @slug refs), not rendered output.
        currentContent: prompt.body,
        currentUpdatedAt: prompt.updated_at,
        resourceName: prompt.name,
        versions: history.versions,
      }
    },
    restore: async (versionNumber: number) => {
      await promptService.restorePromptVersion(teamId, slug, versionNumber)
    },
  }
}
