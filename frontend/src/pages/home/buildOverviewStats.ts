import { BookOpen, Bot, FileText, HardDrive, Rss, Sparkles } from 'lucide-react'

import type { TeamStats } from '@/services/teamService'

import type { OverviewStat } from './WorkspaceTotals'

export interface OverviewInputs {
  teamStats: TeamStats | null
  totalAgents: number
  /** Per-type resource creations over the last 7 days; null until loaded. The
   * single source for every "+N this week" delta, so the types stay consistent. */
  weekly: {
    prompts: number
    artifacts: number
    blueprints: number
    memories: number
    feed: number
  } | null
}

/**
 * Assemble the workspace totals from the various metric sources. Only facts
 * about THIS workspace belong here — the number of MCP tools the product
 * ships is a constant, not a total, and is not one of them. Kept as a pure
 * function so the page component stays simple and this stays unit-testable.
 */
export function buildOverviewStats(inputs: OverviewInputs): OverviewStat[] {
  const { teamStats, totalAgents, weekly } = inputs

  return [
    {
      label: 'Prompts',
      value: teamStats?.total_prompts ?? 0,
      icon: Sparkles,
      weeklyDelta: weekly?.prompts,
    },
    {
      label: 'Artifacts',
      value: teamStats?.total_artifacts ?? 0,
      icon: FileText,
      weeklyDelta: weekly?.artifacts,
    },
    {
      label: 'Memories',
      value: teamStats?.total_memories ?? 0,
      icon: HardDrive,
      weeklyDelta: weekly?.memories,
    },
    {
      label: 'Blueprints',
      value: teamStats?.total_blueprints ?? 0,
      icon: BookOpen,
      weeklyDelta: weekly?.blueprints,
    },
    { label: 'Agents', value: totalAgents, icon: Bot },
    {
      label: 'Feed updates',
      value: teamStats?.total_feed_items ?? 0,
      icon: Rss,
      weeklyDelta: weekly?.feed,
    },
  ]
}
