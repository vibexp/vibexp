import { ArrowRight } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router'

import { TeamResourceAccessChart } from '@/components/TeamResourceAccessChart'
import { Skeleton } from '@/components/ui/skeleton'
import { teamService } from '@/services/teamService'

import { summarizeAccessChannels } from './dashboardLogic'
import { DashboardSection } from './DashboardSection'
import { MCP_SETUP_PATH } from './GettingStarted'
import { TopAccessedResources } from './TopAccessedResources'
import { useKeyedLoad } from './useKeyedLoad'

/** The channels in reading order: the ones an AI tool uses first, then people. */
const CHANNELS = [
  { key: 'mcp', label: 'MCP' },
  { key: 'cli', label: 'CLI' },
  { key: 'api', label: 'API' },
  { key: 'web', label: 'Web' },
] as const

/**
 * The third block of the dashboard, and the one where the loop shows: how the
 * workspace's knowledge is being read, by channel. It leads with the plain
 * numbers, and when nothing has read through MCP, the CLI or the API it says
 * so and points at the MCP setup — the most useful thing a new team can learn
 * here.
 *
 * It reports reads only. Whether something was WRITTEN by an AI is not known
 * to the API, so no write-back figure is shown or inferred.
 */
export function KnowledgeUsage({
  teamId,
  range,
  rangeLabel,
  rangeControl,
  projectSelected,
}: Readonly<{
  teamId: string
  range: string
  /** The range in words, as in "in the last 30 days". */
  rangeLabel: string
  rangeControl: ReactNode
  /** Access metrics are workspace-wide; say so when a project is selected. */
  projectSelected: boolean
}>) {
  const access = useKeyedLoad(`${teamId}:${range}`, () =>
    teamService.getTeamResourceAccessMetrics(teamId, range)
  )
  const summary = access.data
    ? summarizeAccessChannels(access.data.data.counts)
    : null

  return (
    <DashboardSection
      title="How your knowledge is used"
      description={
        projectSelected
          ? `Reads by channel in the last ${rangeLabel}, across the whole workspace.`
          : `Reads by channel in the last ${rangeLabel}.`
      }
      aside={rangeControl}
    >
      <dl
        className="grid grid-cols-2 gap-x-6 gap-y-3 rounded-lg border px-4 py-3 sm:grid-cols-4"
        aria-busy={access.loading}
      >
        {CHANNELS.map(channel => (
          <div key={channel.key}>
            <dt className="text-muted-foreground text-xs">{channel.label}</dt>
            <dd className="mt-0.5 text-lg font-semibold tabular-nums">
              {summary ? (
                summary[channel.key].toLocaleString()
              ) : access.loading ? (
                <Skeleton className="h-7 w-10" />
              ) : (
                '—'
              )}
            </dd>
          </div>
        ))}
      </dl>

      {access.error && (
        <p className="text-muted-foreground text-sm">
          Could not load reads by channel just now.
        </p>
      )}

      {summary?.noAiToolReads && (
        <p className="text-sm">
          No AI tool has read from this workspace in the last {rangeLabel}.{' '}
          <Link
            to={MCP_SETUP_PATH}
            className="hover:text-primary inline-flex items-center font-medium underline underline-offset-2"
          >
            Connect one over MCP
            <ArrowRight className="ml-0.5 size-3.5" aria-hidden="true" />
          </Link>
        </p>
      )}

      <TeamResourceAccessChart teamId={teamId} range={range} />

      <div className="space-y-3 pt-2">
        <h3 className="text-sm font-semibold">Top accessed resources</h3>
        <TopAccessedResources teamId={teamId} range={range} />
      </div>
    </DashboardSection>
  )
}
