import {
  BookOpen,
  FileText,
  HardDrive,
  type LucideIcon,
  Package,
  Search,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'

import {
  SegmentedControl,
  type SegmentedOption,
} from '@/components/SegmentedControl'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { TopAccessedResource } from '@/services/teamService'
import { teamService } from '@/services/teamService'

/** Access-channel filter shown on every list (matches the design). */
const CHANNELS: readonly SegmentedOption[] = [
  { value: 'all', label: 'All' },
  { value: 'web', label: 'WEB' },
  { value: 'cli', label: 'CLI' },
  { value: 'mcp', label: 'MCP' },
  { value: 'api', label: 'API' },
]

/** The four resource types that get their own ranked list, in design order. */
const LIST_TYPES: { type: string; title: string; icon: LucideIcon }[] = [
  { type: 'artifact', title: 'Top accessed artifacts', icon: Package },
  { type: 'memory', title: 'Top accessed memories', icon: HardDrive },
  { type: 'blueprint', title: 'Top accessed blueprints', icon: BookOpen },
  { type: 'prompt', title: 'Top accessed prompts', icon: FileText },
]

const TYPE_LABEL: Record<string, string> = {
  artifact: 'Artifact',
  memory: 'Memory',
  blueprint: 'Blueprint',
  prompt: 'Prompt',
}

interface TopAccessedListProps {
  title: string
  icon: LucideIcon
  type: string
  teamId: string
  range: string
  /** Baseline (channel = 'all') items for this type, already capped at the top 5. */
  baselineItems: TopAccessedResource[]
  baselineLoading: boolean
}

/** A single ranked list (one resource type) with its access-channel filter. */
function TopAccessedList({
  title,
  icon: Icon,
  type,
  teamId,
  range,
  baselineItems,
  baselineLoading,
}: TopAccessedListProps) {
  const [channel, setChannel] = useState('all')
  // Per-channel override of the baseline: `null` means "show the parent's
  // all-channels data" (so the common 'all' case adds no extra request).
  const [filtered, setFiltered] = useState<TopAccessedResource[] | null>(null)
  const [filteredLoading, setFilteredLoading] = useState(false)
  const [filteredError, setFilteredError] = useState(false)

  // Re-query the endpoint with a `source` filter whenever a concrete channel is
  // selected; the endpoint returns a mixed ranked list, so bucket it to this
  // list's resource type and keep the top 5. 'all' falls back to the baseline.
  useEffect(() => {
    if (channel === 'all') {
      setFiltered(null)
      setFilteredLoading(false)
      setFilteredError(false)
      return
    }
    const controller = new AbortController()
    setFilteredLoading(true)
    setFilteredError(false)
    teamService
      .getTeamTopAccessedResources(
        teamId,
        range,
        50,
        channel,
        controller.signal
      )
      .then(response => {
        setFiltered(
          response.data.items.filter(i => i.resource_type === type).slice(0, 5)
        )
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted) return
        console.error('Failed to fetch top accessed resources:', err)
        // Distinguish a failed fetch from a genuinely empty channel so the list
        // shows an error rather than a misleading "no access yet".
        setFiltered([])
        setFilteredError(true)
      })
      .finally(() => {
        if (!controller.signal.aborted) setFilteredLoading(false)
      })
    return () => {
      controller.abort()
    }
  }, [channel, teamId, range, type])

  const items = channel === 'all' ? baselineItems : (filtered ?? [])
  const loading = channel === 'all' ? baselineLoading : filteredLoading
  const showError = channel !== 'all' && filteredError
  const max = Math.max(1, ...items.map(i => i.access_count))

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3 space-y-0">
        <div className="flex items-center gap-2">
          <Icon className="text-muted-foreground size-4" />
          <h3 className="text-sm font-semibold">{title}</h3>
        </div>
        <SegmentedControl
          options={CHANNELS}
          value={channel}
          onChange={setChannel}
          size="sm"
          aria-label={`Filter ${title} by access channel`}
        />
      </CardHeader>
      <CardContent>
        {loading ? (
          <div className="space-y-3">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-5 w-full" />
            ))}
          </div>
        ) : showError ? (
          <div className="text-muted-foreground flex h-24 items-center justify-center text-sm">
            Couldn&apos;t load this channel
          </div>
        ) : items.length === 0 ? (
          <div className="text-muted-foreground flex h-24 items-center justify-center text-sm">
            No {(TYPE_LABEL[type] || type).toLowerCase()} access yet
          </div>
        ) : (
          <ol className="space-y-2.5">
            {items.map((item, index) => (
              <li
                key={`${item.resource_id}-${String(index)}`}
                className="flex items-center gap-3 text-sm"
              >
                <span className="text-muted-foreground w-4 shrink-0 tabular-nums">
                  {index + 1}
                </span>
                <span className="min-w-0 flex-1 truncate">
                  {item.name || TYPE_LABEL[type] || type}
                </span>
                <span
                  aria-hidden
                  className="bg-muted hidden h-1.5 w-24 shrink-0 overflow-hidden rounded-full sm:block"
                >
                  <span
                    className="bg-foreground block h-full rounded-full"
                    style={{
                      width: `${String((item.access_count / max) * 100)}%`,
                    }}
                  />
                </span>
                <span className="w-8 shrink-0 text-right font-semibold tabular-nums">
                  {item.access_count}
                </span>
              </li>
            ))}
          </ol>
        )}
      </CardContent>
    </Card>
  )
}

interface TopAccessedResourcesProps {
  teamId: string
  /** Range is owned by the page-level filter. */
  range: string
}

/**
 * "Top accessed resources" — four per-type ranked lists (artifacts, memories,
 * blueprints, prompts) in a 2×2 grid, each with an access-channel filter. Fetches
 * the team's most-accessed resources once and buckets them by type client-side
 * (the endpoint returns a mixed, ranked list), showing the top 5 per type.
 * Re-fetches on team/range change; aborts in-flight work on cleanup.
 */
export function TopAccessedResources({
  teamId,
  range,
}: TopAccessedResourcesProps) {
  const [items, setItems] = useState<TopAccessedResource[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    const fetchData = async () => {
      setLoading(true)
      try {
        // Request a generous page so each type has enough to fill its top 5.
        // Baseline is all-channels; per-list channel filtering re-queries below.
        const response = await teamService.getTeamTopAccessedResources(
          teamId,
          range,
          50,
          undefined,
          controller.signal
        )
        setItems(response.data.items)
        setError(false)
      } catch (err) {
        if (controller.signal.aborted) return
        console.error('Failed to fetch top accessed resources:', err)
        setItems([])
        setError(true)
      } finally {
        if (!controller.signal.aborted) setLoading(false)
      }
    }
    void fetchData()
    return () => {
      controller.abort()
    }
  }, [teamId, range])

  const byType = useMemo(() => {
    const buckets: Record<string, TopAccessedResource[]> = {}
    for (const item of items) {
      ;(buckets[item.resource_type] ??= []).push(item)
    }
    return buckets
  }, [items])

  const isEmpty = !loading && !error && items.length === 0

  if (isEmpty) {
    return (
      <Card>
        <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
          <div className="bg-muted text-muted-foreground flex size-10 items-center justify-center rounded-full">
            <Search className="size-5" />
          </div>
          <p className="text-sm font-medium">No accessed resources yet</p>
          <p className="text-muted-foreground text-sm">
            Once your resources are opened from web, CLI, MCP or API, the
            most-accessed ones will rank here.
          </p>
        </CardContent>
      </Card>
    )
  }

  if (error) {
    return (
      <Card>
        <CardContent className="text-muted-foreground flex h-24 items-center justify-center text-sm">
          Couldn&apos;t load top accessed resources
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
      {LIST_TYPES.map(({ type, title, icon }) => (
        <TopAccessedList
          key={type}
          type={type}
          title={title}
          icon={icon}
          teamId={teamId}
          range={range}
          baselineItems={(byType[type] ?? []).slice(0, 5)}
          baselineLoading={loading}
        />
      ))}
    </div>
  )
}
