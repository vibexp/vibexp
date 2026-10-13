import { ArrowRight } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router'

import {
  type Channel,
  CHANNEL_FILL,
  CHANNEL_LABEL,
} from '@/components/channelSeries'
import { TeamResourceAccessChart } from '@/components/TeamResourceAccessChart'
import { Skeleton } from '@/components/ui/skeleton'
import { teamService } from '@/services/teamService'

import {
  summarizeAccessChannels,
  summarizeWriteChannels,
} from './dashboardLogic'
import { DASHBOARD_GROUP, DashboardSection } from './DashboardSection'
import { MCP_SETUP_PATH } from './GettingStarted'
import { TopAccessedResources } from './TopAccessedResources'
import { useKeyedLoad } from './useKeyedLoad'

/** The channels in reading order: the ones an AI tool uses first, then people. */
const CHANNELS: readonly Channel[] = ['mcp', 'cli', 'api', 'web']

interface ChannelCounts extends Record<Channel, number> {
  total: number
  aiTools: number
}

/** What people wrote, said beside "no AI tool wrote back" so it is not lost. */
function webWrites(count: number): string {
  return count === 1
    ? 'People made 1 write in the web app.'
    : `People made ${count.toLocaleString()} writes in the web app.`
}

function ConnectLink() {
  return (
    <Link
      to={MCP_SETUP_PATH}
      className="hover:text-primary inline-flex items-center font-medium underline underline-offset-2"
    >
      Connect one over MCP
      <ArrowRight className="ml-0.5 size-3.5" aria-hidden="true" />
    </Link>
  )
}

/**
 * One half of the loop: the AI tools' share as the headline, then the same
 * four channels in the same order and colours as the other half.
 */
function LoopRow({
  label,
  noun,
  counts,
  loading,
  testId,
}: Readonly<{
  /** What the headline counts, as in "Read by AI tools". */
  label: string
  /** The unit of the total, as in "of 120 reads". */
  noun: string
  counts: ChannelCounts | null
  loading: boolean
  testId: string
}>) {
  const figure = (value: number | undefined, skeleton: string) => {
    if (value !== undefined) return value.toLocaleString()
    return loading ? <Skeleton className={skeleton} /> : '—'
  }
  return (
    <div
      className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:gap-6"
      data-testid={testId}
      aria-busy={loading}
    >
      <div className="sm:w-52 sm:shrink-0">
        <p className="text-muted-foreground text-xs">{label}</p>
        <p className="mt-0.5 flex items-baseline gap-1.5">
          <span className="text-2xl font-semibold tabular-nums">
            {figure(counts?.aiTools, 'h-8 w-12')}
          </span>
          {counts && (
            <span className="text-muted-foreground text-xs">
              of {counts.total.toLocaleString()} {noun}
            </span>
          )}
        </p>
      </div>
      <dl className="grid flex-1 grid-cols-4 gap-x-4">
        {CHANNELS.map(channel => (
          <div key={channel}>
            <dt className="text-muted-foreground flex items-center gap-1.5 text-xs">
              <span
                className="size-2 shrink-0 rounded-full"
                style={{ backgroundColor: CHANNEL_FILL[channel] }}
                aria-hidden="true"
              />
              {CHANNEL_LABEL[channel]}
            </dt>
            <dd className="mt-0.5 text-lg font-semibold tabular-nums">
              {figure(counts?.[channel], 'h-7 w-10')}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

/**
 * The third block of the dashboard, and the one where the loop shows: what
 * read the workspace's knowledge and what wrote back to it, by channel, over
 * one shared period. Each half leads with the AI tools' share (MCP, CLI and
 * API), because "is the AI reading, and is it writing back?" is the question
 * this block answers. When a half did not run, it says so in words and points
 * at the MCP setup rather than showing a row of zeros.
 *
 * Writes are creates and updates of prompts, artifacts, blueprints and
 * memories. The API reports them as totals for the period, not per day, so the
 * chart below stays a chart of reads.
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
  /** These metrics are workspace-wide; say so when a project is selected. */
  projectSelected: boolean
}>) {
  const access = useKeyedLoad(`${teamId}:${range}`, () =>
    teamService.getTeamResourceAccessMetrics(teamId, range)
  )
  const creation = useKeyedLoad(`${teamId}:${range}`, () =>
    teamService.getTeamResourceCreationMetrics(teamId, range)
  )
  const reads = access.data
    ? summarizeAccessChannels(access.data.data.counts)
    : null
  const writes = creation.data
    ? summarizeWriteChannels(creation.data.data.writes_by_source)
    : null

  const noAiReads = reads?.noAiToolReads === true
  const noAiWrites = writes?.noAiToolWrites === true

  return (
    <DashboardSection
      title="How your knowledge is used"
      description={
        projectSelected
          ? `Reads and writes by channel in the last ${rangeLabel}, across the whole workspace.`
          : `Reads and writes by channel in the last ${rangeLabel}.`
      }
      aside={rangeControl}
    >
      <div className={DASHBOARD_GROUP}>
        <LoopRow
          label="Read by AI tools"
          noun={reads?.total === 1 ? 'read' : 'reads'}
          counts={reads}
          loading={access.loading}
          testId="loop-reads"
        />
        {!noAiWrites && !creation.error && (
          <LoopRow
            label="Written back by AI tools"
            noun={writes?.total === 1 ? 'write' : 'writes'}
            counts={writes}
            loading={creation.loading}
            testId="loop-writes"
          />
        )}
      </div>

      {access.error && (
        <p className="text-muted-foreground text-sm">
          Could not load reads by channel just now.
        </p>
      )}
      {creation.error && (
        <p className="text-muted-foreground text-sm">
          Could not load writes by channel just now.
        </p>
      )}

      {(noAiReads || noAiWrites) && (
        <p className="text-sm" data-testid="loop-not-running">
          {noAiReads && noAiWrites
            ? `No AI tool has read from or written back to this workspace in the last ${rangeLabel}.`
            : noAiReads
              ? `No AI tool has read from this workspace in the last ${rangeLabel}.`
              : `No AI tool wrote back to this workspace in the last ${rangeLabel}.`}
          {noAiWrites && writes.web > 0 && ` ${webWrites(writes.web)}`}{' '}
          <ConnectLink />
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
