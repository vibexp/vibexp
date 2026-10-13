import { ArrowRight } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router'

import {
  type Channel,
  CHANNEL_FILL,
  CHANNEL_LABEL,
  CHANNEL_ORDER,
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
        <div className="mt-0.5 flex items-baseline gap-1.5">
          <span className="text-2xl font-semibold tabular-nums">
            {figure(counts?.aiTools, 'h-8 w-12')}
          </span>
          {counts && (
            <span className="text-muted-foreground text-xs">
              of {counts.total.toLocaleString()} {noun}
            </span>
          )}
        </div>
      </div>
      <dl className="grid flex-1 grid-cols-4 gap-x-4">
        {CHANNEL_ORDER.map(channel => (
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

/** Which half of the loop is missing in the period, said in one sentence. */
function loopNotRunningMessage(
  noAiReads: boolean,
  noAiWrites: boolean,
  rangeLabel: string
): string {
  if (noAiReads && noAiWrites) {
    return `No AI tool has read from or written back to this workspace in the last ${rangeLabel}.`
  }
  if (noAiReads) {
    return `No AI tool has read from this workspace in the last ${rangeLabel}.`
  }
  return `No AI tool wrote back to this workspace in the last ${rangeLabel}.`
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

  // Creates and updates the API could not attribute to a channel, made before
  // channels were recorded. The figures leave them out, the "of N writes"
  // total included, so they can understate what happened; say so while they do.
  const unrecorded = writes?.unrecorded ?? 0

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
          {loopNotRunningMessage(noAiReads, noAiWrites, rangeLabel)}
          {noAiWrites && writes.web > 0 && ` ${webWrites(writes.web)}`}{' '}
          <ConnectLink />
        </p>
      )}

      {unrecorded > 0 && (
        <p
          className="text-muted-foreground text-xs"
          data-testid="loop-unrecorded"
        >
          {unrecorded === 1
            ? '1 write in this period has no recorded channel and is not counted above.'
            : `${unrecorded.toLocaleString()} writes in this period have no recorded channel and are not counted above.`}
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
