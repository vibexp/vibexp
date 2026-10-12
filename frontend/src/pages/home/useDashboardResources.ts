import { artifactService } from '@/services/artifactService'
import { blueprintService } from '@/services/blueprintService'
import { memoryService } from '@/services/memoryService'
import { promptService } from '@/services/promptService'

import {
  type DashboardResource,
  type DashboardResourceType,
  type StaleResult,
  toDashboardResource,
} from './dashboardLogic'
import { type KeyedLoad, useKeyedLoad } from './useKeyedLoad'

interface ResourcePage {
  type: DashboardResourceType
  total: number
  resources: DashboardResource[]
}

interface ListQuery {
  limit: number
  freshness?: 'stale'
  /** Narrows to one project; left out, the whole workspace is listed. */
  project_id?: string
}

const RECENT_ORDER = { sort_by: 'updated_at', sort_order: 'desc' } as const

/**
 * One page of each resource type, most recently updated first. The four list
 * endpoints take the same filters (`freshness`, `project_id`), so a block asks
 * for all four with one query. `allSettled`: a type whose request fails is
 * left out of the result rather than failing the block.
 */
async function loadResourcePages(
  teamId: string,
  query: ListQuery
): Promise<ResourcePage[]> {
  const filters = { ...RECENT_ORDER, ...query }
  const settled = await Promise.allSettled([
    artifactService.getArtifacts(teamId, filters).then((r): ResourcePage => ({
      type: 'artifact',
      total: r.total_count,
      resources: r.artifacts.map(toDashboardResource.artifact),
    })),
    blueprintService.getBlueprints(teamId, filters).then((r): ResourcePage => ({
      type: 'blueprint',
      total: r.total_count,
      resources: r.blueprints.map(toDashboardResource.blueprint),
    })),
    memoryService.getMemories(teamId, filters).then((r): ResourcePage => ({
      type: 'memory',
      total: r.total_count,
      resources: r.memories.map(toDashboardResource.memory),
    })),
    promptService.getPrompts(teamId, filters).then((r): ResourcePage => ({
      type: 'prompt',
      total: r.total_count,
      resources: r.prompts.map(toDashboardResource.prompt),
    })),
  ])

  const pages = settled.flatMap(result =>
    result.status === 'fulfilled' ? [result.value] : []
  )
  // Every type failing is a failure of the block, not "nothing to show".
  if (pages.length === 0) throw new Error('No resource list could be loaded')
  return pages
}

const scopeKey = (teamId: string | undefined, projectId: string | undefined) =>
  teamId ? `${teamId}:${projectId ?? ''}` : null

/** The stale resources of each type, for "Needs your attention". */
export function useStaleResources(
  teamId: string | undefined,
  projectId: string | undefined,
  namesPerType: number
): KeyedLoad<StaleResult[]> {
  return useKeyedLoad(scopeKey(teamId, projectId), () =>
    loadResourcePages(teamId ?? '', {
      freshness: 'stale',
      limit: namesPerType,
      project_id: projectId,
    })
  )
}

/** The most recently updated resources of each type, for "What changed". */
export function useRecentResources(
  teamId: string | undefined,
  projectId: string | undefined,
  perType: number
): KeyedLoad<DashboardResource[][]> {
  return useKeyedLoad(scopeKey(teamId, projectId), () =>
    loadResourcePages(teamId ?? '', {
      limit: perType,
      project_id: projectId,
    }).then(pages => pages.map(page => page.resources))
  )
}
