import {
  Activity as ActivityIcon,
  BookOpen,
  Bot,
  FileText,
  Rss,
  Server,
  Sparkles,
  Zap,
} from 'lucide-react'

import type { TeamStats } from '@/services/teamService'

import type { OverviewStat, Trend } from './OverviewCard'

export interface OverviewInputs {
  teamStats: TeamStats | null
  totalSessions: number
  sessionsTrendPct: number
  totalAgents: number
  mcpToolsCount: number
  /** Per-type resource creations over the last 7 days; null until loaded. The
   * single source for every "+N this week" trend, so the types stay consistent. */
  weekly: {
    prompts: number
    artifacts: number
    blueprints: number
    memories: number
    feed: number
  } | null
  /** Suppresses every trend badge for a brand-new / empty workspace. */
  isEmptyWorkspace: boolean
}

/**
 * Assemble the eight Overview cards from the various metric sources. Trend badges
 * are data-driven: a positive weekly delta shows a green "↗ +N" badge, sessions
 * show a percentage (or a muted "0%"), and an empty workspace shows none. Kept as
 * a pure function so the page component stays simple and this stays unit-testable.
 */
export function buildOverviewStats(inputs: OverviewInputs): OverviewStat[] {
  const {
    teamStats,
    totalSessions,
    sessionsTrendPct,
    totalAgents,
    mcpToolsCount,
    weekly,
    isEmptyWorkspace,
  } = inputs

  const upTrend = (n: number): Trend | null =>
    !isEmptyWorkspace && n > 0 ? { label: `+${String(n)}`, tone: 'up' } : null

  const weekLabel = (n: number): string | undefined =>
    !isEmptyWorkspace && n > 0 ? `+${String(n)} this week` : undefined

  const sessionsTrend: Trend | null = isEmptyWorkspace
    ? null
    : sessionsTrendPct > 0
      ? { label: `+${String(sessionsTrendPct)}%`, tone: 'up' }
      : { label: '0%', tone: 'flat' }

  return [
    {
      label: 'AI Sessions',
      value: totalSessions,
      icon: ActivityIcon,
      trend: sessionsTrend,
    },
    {
      label: 'Total Prompts',
      value: teamStats?.total_prompts ?? 0,
      icon: Sparkles,
      trend: upTrend(weekly?.prompts ?? 0),
    },
    {
      label: 'Total Artifacts',
      value: teamStats?.total_artifacts ?? 0,
      icon: FileText,
      trend: upTrend(weekly?.artifacts ?? 0),
      subtitle: weekLabel(weekly?.artifacts ?? 0),
    },
    {
      label: 'Total Memories',
      value: teamStats?.total_memories ?? 0,
      icon: Zap,
      trend: upTrend(weekly?.memories ?? 0),
    },
    {
      label: 'Total Blueprints',
      value: teamStats?.total_blueprints ?? 0,
      icon: BookOpen,
      trend: upTrend(weekly?.blueprints ?? 0),
    },
    {
      label: 'Total Agents',
      value: totalAgents,
      icon: Bot,
      trend: null,
    },
    {
      label: 'AI Feed updates',
      value: teamStats?.total_feed_items ?? 0,
      icon: Rss,
      trend: upTrend(weekly?.feed ?? 0),
      subtitle: weekLabel(weekly?.feed ?? 0),
    },
    {
      label: 'MCP Tools',
      value: mcpToolsCount,
      icon: Server,
      trend: null,
    },
  ]
}
