import { BarChart3 } from 'lucide-react'

import { Panel, PanelHeader, PanelRow, PanelTitle } from '@/components/ui/panel'
import type { Agent } from '@/services/agentService'

import { formatDate, successRateColor } from '../helpers'

interface AgentStatsPanelProps {
  agent: Agent
}

function StatRow({
  label,
  value,
  valueClassName,
}: Readonly<{ label: string; value: string; valueClassName?: string }>) {
  return (
    <PanelRow as="li">
      <span className="text-muted-foreground shrink-0">{label}</span>
      <span className={`font-medium ${valueClassName ?? ''}`}>{value}</span>
    </PanelRow>
  )
}

/**
 * Run statistics for the details column (#918).
 *
 * This was a four-across grid of `Card`s under the page header. In the reading
 * shell it is a details section, so it is built on the `ui/panel` primitives
 * instead: one hairline-divided row list that goes flat inside the details
 * column and stays a card anywhere else. "Created" is gone — the generated
 * metadata section above it already renders Created and Updated.
 */
export function AgentStatsPanel({ agent }: Readonly<AgentStatsPanelProps>) {
  const percentage = Math.round(agent.success_rate)

  return (
    <Panel data-testid="agent-stats-panel">
      <PanelHeader>
        <div className="flex min-w-0 items-center gap-2.5">
          <BarChart3 className="text-muted-foreground size-4 shrink-0" />
          <PanelTitle>Stats</PanelTitle>
        </div>
      </PanelHeader>
      <ul className="divide-border border-border divide-y border-t">
        <StatRow
          label="Success rate"
          value={`${String(percentage)}%`}
          valueClassName={successRateColor(percentage)}
        />
        <StatRow label="Total runs" value={agent.total_runs.toLocaleString()} />
        <StatRow label="Last run" value={formatDate(agent.last_run)} />
      </ul>
    </Panel>
  )
}
