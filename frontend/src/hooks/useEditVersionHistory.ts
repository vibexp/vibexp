import type { VersionHistoryMeta } from '@/components/metadata/MetadataPanel'
import { useTeam } from '@/contexts/TeamContext'
import {
  buildResourceVersionsUrl,
  type ResourceUrlType,
} from '@/lib/resourceUrl'
import { artifactService } from '@/services/artifactService'
import { blueprintService } from '@/services/blueprintService'
import { memoryService } from '@/services/memoryService'
import { promptService } from '@/services/promptService'
import type { ResourceVersionListResponse } from '@/types/version'

import { useResourceVersions } from './useResourceVersions'

/** The loaded resource an edit page is editing, by how its kind is addressed. */
export interface EditedResource {
  type: ResourceUrlType
  id: string
  slug?: string
  projectId?: string
  updatedAt?: string
}

/** The kind's version-list call, or null when its address is incomplete. */
function versionLoader(
  teamId: string,
  r: EditedResource
): (() => Promise<ResourceVersionListResponse>) | null {
  const { slug = '', projectId = '' } = r
  switch (r.type) {
    case 'artifact':
      return () => artifactService.getArtifactVersions(teamId, projectId, slug)
    case 'blueprint':
      return () =>
        blueprintService.getBlueprintVersions(teamId, projectId, slug)
    case 'prompt':
      return () => promptService.getPromptVersions(teamId, slug)
    case 'memory':
      return () => memoryService.getMemoryVersions(teamId, r.id)
  }
}

/**
 * The Metadata section's Version row and history link on an edit page
 * (#1180), so they stay visible while editing exactly as on the reading page.
 *
 * Keyed off the LOADED resource rather than the route, because an edit page
 * has nothing to show until the resource resolves anyway. The link is the one
 * the reading page builds (`buildResourceVersionsUrl`). Best-effort, like the
 * reading page's: a failed load only drops the link.
 */
export function useEditVersionHistory(
  resource: EditedResource | null
): VersionHistoryMeta | undefined {
  const { currentTeam } = useTeam()
  const to = resource ? buildResourceVersionsUrl(resource) : null
  const load =
    resource && currentTeam && to
      ? versionLoader(currentTeam.id, resource)
      : null
  const { versionHistory } = useResourceVersions({
    loadVersions: load,
    to: to ?? undefined,
    editedAt: resource?.updatedAt,
    deps: [currentTeam?.id, to],
  })
  return versionHistory
}
