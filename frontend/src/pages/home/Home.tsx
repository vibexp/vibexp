import { useEffect, useMemo, useState } from 'react'

import { PageHeader } from '@/components/PageHeader'
import {
  SegmentedControl,
  type SegmentedOption,
} from '@/components/SegmentedControl'
import { TeamFeedCreationChart } from '@/components/TeamFeedCreationChart'
import { TeamResourceCreationChart } from '@/components/TeamResourceCreationChart'
import { TeamResourceCumulativeChart } from '@/components/TeamResourceCumulativeChart'
import { useProject } from '@/contexts/ProjectContext'
import { useTeam } from '@/contexts/TeamContext'
import { useAuth } from '@/contexts/useAuth'
import { buildOverviewStats } from '@/pages/home/buildOverviewStats'
import { DashboardSection } from '@/pages/home/DashboardSection'
import { GettingStarted } from '@/pages/home/GettingStarted'
import { KnowledgeUsage } from '@/pages/home/KnowledgeUsage'
import { NeedsAttention } from '@/pages/home/NeedsAttention'
import { WhatChanged } from '@/pages/home/WhatChanged'
import { WorkspaceTotals } from '@/pages/home/WorkspaceTotals'
import type { Activity as ActivityType } from '@/services/activityService'
import { activityService } from '@/services/activityService'
import { agentService } from '@/services/agentService'
import type { RecentComment } from '@/services/commentService'
import { commentService } from '@/services/commentService'
import type { FeedItem } from '@/services/feedService'
import { feedService } from '@/services/feedService'
import type { TeamMember, TeamStats } from '@/services/teamService'
import { teamService } from '@/services/teamService'

const DEFAULT_RANGE = '30d'

/** Short range labels, also read aloud in sentences ("in the last 30 days").
 * Values map to the shared TIME_SERIES_RANGE_OPTIONS / backend ranges. */
const RANGE_TABS: readonly SegmentedOption[] = [
  { value: '7d', label: '7 days' },
  { value: '14d', label: '14 days' },
  { value: '30d', label: '30 days' },
  { value: '60d', label: '2 months' },
  { value: '90d', label: '3 months' },
  { value: '180d', label: '6 months' },
]

/** Sums a numeric field across time-series count rows. */
function sumCountField<T>(counts: readonly T[], key: keyof T): number {
  return counts.reduce((acc, c) => acc + (Number(c[key]) || 0), 0)
}

interface WeeklyDeltas {
  prompts: number
  artifacts: number
  blueprints: number
  memories: number
  feed: number
}

export function Home() {
  const { user } = useAuth()
  const { currentTeam } = useTeam()
  const { currentProject } = useProject()
  const teamId = currentTeam?.id
  const projectId = currentProject?.id

  const [range, setRange] = useState(DEFAULT_RANGE)
  const [teamStats, setTeamStats] = useState<TeamStats | null>(null)
  const [totalAgents, setTotalAgents] = useState(0)
  const [weekly, setWeekly] = useState<WeeklyDeltas | null>(null)
  const [statsLoading, setStatsLoading] = useState(true)

  const [activities, setActivities] = useState<ActivityType[]>([])
  const [activitiesLoading, setActivitiesLoading] = useState(true)
  const [activitiesError, setActivitiesError] = useState<string | null>(null)
  const [feedItems, setFeedItems] = useState<FeedItem[]>([])
  const [feedLoading, setFeedLoading] = useState(true)
  const [feedError, setFeedError] = useState<string | null>(null)
  const [recentComments, setRecentComments] = useState<RecentComment[]>([])
  const [commentMembers, setCommentMembers] = useState<Map<string, TeamMember>>(
    new Map()
  )
  const [commentsLoading, setCommentsLoading] = useState(true)
  const [commentsError, setCommentsError] = useState<string | null>(null)

  const greeting = useMemo(() => {
    const namePart = user?.name.split(' ')[0]
    const firstName = namePart && namePart.length > 0 ? namePart : 'User'
    return `Welcome back, ${firstName}!`
  }, [user?.name])

  useEffect(() => {
    const fetchRecentActivities = async () => {
      try {
        setActivitiesLoading(true)
        setActivitiesError(null)
        const response = await activityService.getActivities({ limit: 10 })
        setActivities(response.data.activities)
      } catch (error) {
        console.warn('Failed to fetch recent activities:', error)
        setActivitiesError('Failed to load recent activities')
      } finally {
        setActivitiesLoading(false)
      }
    }
    void fetchRecentActivities()
  }, [])

  // Workspace totals: pulled together so the row settles as one unit.
  // allSettled keeps a single failing source (e.g. agents) from blanking the
  // rest of the row; the finally guards against a hung skeleton.
  useEffect(() => {
    if (!teamId) return
    const fetchOverview = async () => {
      setStatsLoading(true)
      try {
        const [stats, agents, created, feed] = await Promise.allSettled([
          teamService.getTeamStats(teamId),
          agentService.getAgentStats(teamId),
          teamService.getTeamResourceCreationMetrics(teamId, '7d'),
          teamService.getTeamFeedCreationMetrics(teamId, '7d'),
        ])
        if (stats.status === 'fulfilled') setTeamStats(stats.value)
        if (agents.status === 'fulfilled') {
          const stat = agents.value
          setTotalAgents(stat.total_agents)
        }
        if (created.status === 'fulfilled') {
          const counts = created.value.data.counts
          setWeekly({
            prompts: sumCountField(counts, 'prompts'),
            artifacts: sumCountField(counts, 'artifacts'),
            blueprints: sumCountField(counts, 'blueprints'),
            memories: sumCountField(counts, 'memories'),
            feed:
              feed.status === 'fulfilled'
                ? feed.value.data.counts.reduce((a, c) => a + c.feed_items, 0)
                : 0,
          })
        }
      } catch (error) {
        console.warn('Failed to assemble overview stats:', error)
      } finally {
        setStatsLoading(false)
      }
    }
    void fetchOverview()
  }, [teamId])

  useEffect(() => {
    if (!teamId) return
    const fetchRecentFeedItems = async () => {
      try {
        setFeedLoading(true)
        setFeedError(null)
        const response = await feedService.getFeedItems(teamId, {
          limit: 10,
          archived: 'false',
          page: 1,
          project_id: projectId,
        })
        setFeedItems(response.items)
      } catch (error) {
        console.warn('Failed to fetch recent feed items:', error)
        setFeedError('Failed to load recent feed items')
      } finally {
        setFeedLoading(false)
      }
    }
    void fetchRecentFeedItems()
  }, [teamId, projectId])

  useEffect(() => {
    if (!teamId) return
    const fetchRecentComments = async () => {
      try {
        setCommentsLoading(true)
        setCommentsError(null)
        // Recent activity + members resolved together (member map → author names,
        // feed-reply pattern). allSettled so a members miss doesn't blank the card.
        const [commentsResult, membersResult] = await Promise.allSettled([
          commentService.recent(teamId, 10),
          teamService.getTeamMembers(teamId),
        ])
        if (commentsResult.status === 'fulfilled') {
          setRecentComments(commentsResult.value.comments)
        } else {
          setCommentsError('Failed to load recent comments')
        }
        if (membersResult.status === 'fulfilled') {
          setCommentMembers(
            new Map(membersResult.value.map(m => [m.user_id, m]))
          )
        }
      } catch (error) {
        console.warn('Failed to fetch recent comments:', error)
        setCommentsError('Failed to load recent comments')
      } finally {
        setCommentsLoading(false)
      }
    }
    void fetchRecentComments()
  }, [teamId])

  // Resources only: every team has a default project, so counting projects
  // would mean no workspace is ever empty.
  const totalResources = teamStats
    ? teamStats.total_prompts +
      teamStats.total_artifacts +
      teamStats.total_blueprints +
      teamStats.total_memories
    : 0
  const isEmptyWorkspace =
    !statsLoading && teamStats != null && totalResources === 0

  const overviewStats = buildOverviewStats({ teamStats, totalAgents, weekly })

  const rangeLabel = RANGE_TABS.find(tab => tab.value === range)?.label ?? range
  const rangeControl = (
    <SegmentedControl
      size="sm"
      options={RANGE_TABS}
      value={range}
      onChange={setRange}
      aria-label="Select analytics time range"
    />
  )

  const totals = (
    <DashboardSection
      title="Workspace totals"
      description={projectId ? 'Counts across the whole workspace.' : undefined}
    >
      <WorkspaceTotals stats={overviewStats} loading={statsLoading} />
    </DashboardSection>
  )

  if (isEmptyWorkspace || !teamId) {
    return (
      <div className="space-y-8">
        <PageHeader title={greeting} className="pb-0" />
        {teamId && <GettingStarted />}
        {teamId && totals}
      </div>
    )
  }

  return (
    <div className="space-y-8">
      <PageHeader title={greeting} className="pb-0" />

      <NeedsAttention
        teamId={teamId}
        projectId={projectId}
        feedItems={feedItems}
        feedLoading={feedLoading}
        feedError={feedError !== null}
      />

      <WhatChanged
        teamId={teamId}
        projectId={projectId}
        members={commentMembers}
        feed={{ items: feedItems, loading: feedLoading, error: feedError }}
        comments={{
          items: recentComments,
          loading: commentsLoading,
          error: commentsError,
        }}
        activities={{
          items: activities,
          loading: activitiesLoading,
          error: activitiesError,
        }}
      />

      <KnowledgeUsage
        teamId={teamId}
        range={range}
        rangeLabel={rangeLabel}
        rangeControl={rangeControl}
        projectSelected={projectId !== undefined}
      />

      {totals}

      <DashboardSection
        title="Growth"
        description={`What was created in the last ${rangeLabel}, and the running total.`}
        aside={rangeControl}
      >
        <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
          <TeamResourceCreationChart teamId={teamId} range={range} />
          <TeamResourceCumulativeChart teamId={teamId} range={range} />
          <TeamFeedCreationChart teamId={teamId} range={range} />
        </div>
      </DashboardSection>
    </div>
  )
}
