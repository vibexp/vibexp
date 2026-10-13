/**
 * The dashboard's derivations, kept pure so the page blocks stay thin and the
 * rules are testable without rendering anything: what needs a human, what
 * changed lately, and through which channels the workspace is being read.
 */

import { memoryDisplayName } from '@/lib/memoryTitle'
import { buildResourceUrl } from '@/lib/resourceUrl'
import type { Artifact } from '@/services/artifactService'
import type { Blueprint } from '@/services/blueprintService'
import type { FeedItem } from '@/services/feedService'
import type { Memory } from '@/services/memoryService'
import type { Prompt } from '@/services/promptService'
import type { AccessCountByDate } from '@/services/resourceAccessService'

export type DashboardResourceType =
  'artifact' | 'blueprint' | 'memory' | 'prompt'

/** One resource as the dashboard names and links it, whatever its type. */
export interface DashboardResource {
  type: DashboardResourceType
  id: string
  name: string
  /** True for an untitled memory, whose name is a body-derived stand-in. */
  nameIsFallback: boolean
  /** Detail URL, or `null` when the row lacks what the route needs. */
  url: string | null
  userId: string
  createdAt: string
  updatedAt: string
}

export const toDashboardResource = {
  artifact: (a: Artifact): DashboardResource => ({
    type: 'artifact',
    id: a.id,
    name: a.title,
    nameIsFallback: false,
    url: buildResourceUrl({
      type: 'artifact',
      id: a.id,
      slug: a.slug,
      projectId: a.project_id,
    }),
    userId: a.user_id,
    createdAt: a.created_at,
    updatedAt: a.updated_at,
  }),
  blueprint: (b: Blueprint): DashboardResource => ({
    type: 'blueprint',
    id: b.id,
    name: b.title,
    nameIsFallback: false,
    url: buildResourceUrl({
      type: 'blueprint',
      id: b.id,
      slug: b.slug,
      projectId: b.project_id,
    }),
    userId: b.user_id,
    createdAt: b.created_at,
    updatedAt: b.updated_at,
  }),
  memory: (m: Memory): DashboardResource => {
    const { name, isFallback } = memoryDisplayName(m)
    return {
      type: 'memory',
      id: m.id,
      name,
      nameIsFallback: isFallback,
      url: buildResourceUrl({ type: 'memory', id: m.id }),
      userId: m.user_id,
      createdAt: m.created_at,
      updatedAt: m.updated_at,
    }
  },
  prompt: (p: Prompt): DashboardResource => ({
    type: 'prompt',
    id: p.id,
    name: p.name,
    nameIsFallback: false,
    url: buildResourceUrl({ type: 'prompt', id: p.id, slug: p.slug }),
    userId: p.user_id,
    createdAt: p.created_at,
    updatedAt: p.updated_at,
  }),
}

// ---------------------------------------------------------------------------
// What changed
// ---------------------------------------------------------------------------

export interface RecentChange extends DashboardResource {
  /**
   * `created` when the resource has not been edited since it was made. The API
   * records who created a resource but not who last edited it, so only a
   * `created` change can honestly name a person.
   */
  action: 'created' | 'updated'
}

/** Timestamps this close together are one write, not a create then an edit. */
const SAME_WRITE_MS = 2000

const time = (iso: string): number => new Date(iso).getTime() || 0

/**
 * The most recent creations and updates across every resource type, newest
 * first. Each input is one type's own most-recently-updated page; a type that
 * failed to load is simply absent.
 */
export function mergeRecentChanges(
  lists: readonly (readonly DashboardResource[])[],
  limit: number
): RecentChange[] {
  return lists
    .flat()
    .map((resource): RecentChange => ({
      ...resource,
      action:
        time(resource.updatedAt) - time(resource.createdAt) <= SAME_WRITE_MS
          ? 'created'
          : 'updated',
    }))
    .sort(
      (a, b) =>
        time(b.updatedAt) - time(a.updatedAt) ||
        // A stable order for identical timestamps (a bulk import).
        a.type.localeCompare(b.type) ||
        a.id.localeCompare(b.id)
    )
    .slice(0, limit)
}

// ---------------------------------------------------------------------------
// Needs your attention
// ---------------------------------------------------------------------------

/** One type's page of stale resources, with the total the server reports. */
export interface StaleResult {
  type: DashboardResourceType
  total: number
  resources: readonly DashboardResource[]
}

export interface StaleGroup extends StaleResult {
  /** The type's list page, pre-filtered to stale. */
  listUrl: string
  /** How many stale resources are not named in `resources`. */
  more: number
}

const LIST_PATH: Record<DashboardResourceType, string> = {
  artifact: '/artifacts',
  blueprint: '/blueprints',
  memory: '/memories',
  prompt: '/prompts',
}

/** The list page of a type, as its own freshness filter would link to it. */
export const staleListUrl = (type: DashboardResourceType): string =>
  `${LIST_PATH[type]}?freshness=stale`

/** The types that have something stale, in a fixed reading order. */
export function buildStaleGroups(
  results: readonly StaleResult[],
  namesPerGroup: number
): StaleGroup[] {
  const order: DashboardResourceType[] = [
    'artifact',
    'blueprint',
    'memory',
    'prompt',
  ]
  return order.flatMap(type => {
    const result = results.find(r => r.type === type)
    if (!result || result.total <= 0) return []
    const resources = result.resources.slice(0, namesPerGroup)
    return [
      {
        type,
        total: result.total,
        resources,
        listUrl: staleListUrl(type),
        more: Math.max(0, result.total - resources.length),
      },
    ]
  })
}

/**
 * Feed posts nobody has answered: active (not archived) and with zero replies,
 * newest first.
 *
 * Zero replies is the only signal used on purpose. The API does not say who
 * replied last, so a post with replies cannot be called "awaiting you" without
 * guessing.
 */
export function unansweredFeedItems(items: readonly FeedItem[]): FeedItem[] {
  return items
    .filter(item => !item.archived_at && item.reply_count === 0)
    .sort((a, b) => time(b.posted_at) - time(a.posted_at))
}

// ---------------------------------------------------------------------------
// How your knowledge is used
// ---------------------------------------------------------------------------

export interface AccessChannelSummary {
  mcp: number
  cli: number
  api: number
  web: number
  total: number
  /** Reads that arrived through MCP, the CLI or the API: the AI's share. */
  aiTools: number
  /**
   * True when nothing read the workspace through MCP, the CLI or the API in
   * the range — the channels an AI tool reads through. Web reads are a person
   * in this app.
   */
  noAiToolReads: boolean
}

/** Reads per access channel over a range of daily counts. */
export function summarizeAccessChannels(
  counts: readonly AccessCountByDate[]
): AccessChannelSummary {
  const sum = (key: 'mcp' | 'cli' | 'api' | 'web') =>
    counts.reduce((acc, day) => acc + day[key], 0)
  const mcp = sum('mcp')
  const cli = sum('cli')
  const api = sum('api')
  const web = sum('web')
  return {
    mcp,
    cli,
    api,
    web,
    total: mcp + cli + api + web,
    aiTools: mcp + cli + api,
    noAiToolReads: mcp + cli + api === 0,
  }
}

export interface WriteChannelSummary {
  mcp: number
  cli: number
  api: number
  web: number
  total: number
  /** Writes that arrived through MCP, the CLI or the API: the AI's share. */
  aiTools: number
  /**
   * True when nothing was written through MCP, the CLI or the API in the
   * range: the loop's write-back half did not run. Web writes are a person in
   * this app.
   */
  noAiToolWrites: boolean
}

/**
 * Writes (creates and updates) per channel over a range. The API reports the
 * range's totals, not a daily series, and counts only writes whose channel was
 * recorded.
 */
export function summarizeWriteChannels(
  writes: Readonly<Record<'mcp' | 'cli' | 'api' | 'web', number>>
): WriteChannelSummary {
  const { mcp, cli, api, web } = writes
  const aiTools = mcp + cli + api
  return {
    mcp,
    cli,
    api,
    web,
    total: aiTools + web,
    aiTools,
    noAiToolWrites: aiTools === 0,
  }
}
