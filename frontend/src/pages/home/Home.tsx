import { BookOpen, Server, Sparkles } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { PageHeader } from '@/components/PageHeader'
import {
  SegmentedControl,
  type SegmentedOption,
} from '@/components/SegmentedControl'
import { TeamFeedCreationChart } from '@/components/TeamFeedCreationChart'
import { TeamResourceAccessChart } from '@/components/TeamResourceAccessChart'
import { TeamResourceCreationChart } from '@/components/TeamResourceCreationChart'
import { TeamResourceCumulativeChart } from '@/components/TeamResourceCumulativeChart'
import { useTeam } from '@/contexts/TeamContext'
import { useAuth } from '@/contexts/useAuth'
import { AnalyticsEmptyState } from '@/pages/home/AnalyticsEmptyState'
import { buildOverviewStats } from '@/pages/home/buildOverviewStats'
import { OverviewCard } from '@/pages/home/OverviewCard'
import { type QuickAction, QuickActionCard } from '@/pages/home/QuickActionCard'
import { RecentActivityList, RecentFeedList } from '@/pages/home/RecentLists'
import { TopAccessedResources } from '@/pages/home/TopAccessedResources'
import { mcpTools } from '@/pages/mcp/mcp-tools'
import type { Activity as ActivityType } from '@/services/activityService'
import { activityService } from '@/services/activityService'
import { agentService } from '@/services/agentService'
import { aiToolsService } from '@/services/aiToolsService'
import type { FeedItem } from '@/services/feedService'
import { feedService } from '@/services/feedService'
import type { TeamStats } from '@/services/teamService'
import { teamService } from '@/services/teamService'

const DEFAULT_RANGE = '30d'

/** Short range labels matching the design's segmented control. Values map to the
 * shared TIME_SERIES_RANGE_OPTIONS / backend ranges. */
const RANGE_TABS: readonly SegmentedOption[] = [
  { value: '7d', label: '7 days' },
  { value: '14d', label: '14 days' },
  { value: '30d', label: '30 days' },
  { value: '60d', label: '2 months' },
  { value: '90d', label: '3 months' },
  { value: '180d', label: '6 months' },
]

const QUICK_ACTIONS: QuickAction[] = [
  {
    title: 'Create Prompt',
    description: 'Write and organize reusable prompts for your team.',
    icon: Sparkles,
    to: '/prompts/new',
    buttonText: 'Create',
  },
  {
    title: 'Setup MCP Server',
    description: 'Connect VibeXP to Claude and your tools over MCP.',
    icon: Server,
    to: '/mcp-servers/vibexp-mcp',
    buttonText: 'Configure',
  },
  {
    title: 'Manage Blueprints',
    description: 'Build and reuse structured blueprints for new work.',
    icon: BookOpen,
    to: '/blueprints',
    buttonText: 'Open',
  },
]

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
  const navigate = useNavigate()
  const teamId = currentTeam?.id

  const [range, setRange] = useState(DEFAULT_RANGE)
  const [teamStats, setTeamStats] = useState<TeamStats | null>(null)
  const [totalSessions, setTotalSessions] = useState(0)
  const [sessionsTrendPct, setSessionsTrendPct] = useState(0)
  const [totalAgents, setTotalAgents] = useState(0)
  const [weekly, setWeekly] = useState<WeeklyDeltas | null>(null)
  const [statsLoading, setStatsLoading] = useState(true)

  const [activities, setActivities] = useState<ActivityType[]>([])
  const [activitiesLoading, setActivitiesLoading] = useState(true)
  const [activitiesError, setActivitiesError] = useState<string | null>(null)
  const [feedItems, setFeedItems] = useState<FeedItem[]>([])
  const [feedLoading, setFeedLoading] = useState(true)
  const [feedError, setFeedError] = useState<string | null>(null)

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

  // Overview metrics: pulled together so the eight cards settle as one unit.
  // allSettled keeps a single failing source (e.g. agents) from blanking the
  // rest of the row; the finally guards against a hung skeleton.
  useEffect(() => {
    if (!teamId) return
    const fetchOverview = async () => {
      setStatsLoading(true)
      try {
        const [stats, overview, agents, created, feed] =
          await Promise.allSettled([
            teamService.getTeamStats(teamId),
            aiToolsService.getClaudeCodeOverviewStats(),
            agentService.getAgentStats(teamId),
            teamService.getTeamResourceCreationMetrics(teamId, '7d'),
            teamService.getTeamFeedCreationMetrics(teamId, '7d'),
          ])
        if (stats.status === 'fulfilled') setTeamStats(stats.value)
        if (overview.status === 'fulfilled') {
          setTotalSessions(overview.value.total_sessions)
          setSessionsTrendPct(overview.value.weekly_trend_percent)
        }
        if (agents.status === 'fulfilled') {
          const stat = agents.value
          setTotalAgents(stat.total_agents)
        }
        if (created.status === 'fulfilled') {
          const sum = (k: keyof (typeof created.value.data.counts)[number]) =>
            created.value.data.counts.reduce(
              (acc, c) => acc + (Number(c[k]) || 0),
              0
            )
          setWeekly({
            prompts: sum('prompts'),
            artifacts: sum('artifacts'),
            blueprints: sum('blueprints'),
            memories: sum('memories'),
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
  }, [teamId])

  const totalResources = teamStats
    ? teamStats.total_prompts +
      teamStats.total_artifacts +
      teamStats.total_blueprints +
      teamStats.total_memories +
      teamStats.total_projects
    : 0
  const isEmptyWorkspace =
    !statsLoading && teamStats != null && totalResources === 0

  const overviewStats = buildOverviewStats({
    teamStats,
    totalSessions,
    sessionsTrendPct,
    totalAgents,
    mcpToolsCount: mcpTools.length,
    weekly,
    isEmptyWorkspace,
  })

  return (
    <div className="space-y-10">
      <PageHeader
        title={greeting}
        description="Your AI command center is ready to boost your productivity."
      />

      <section className="space-y-4">
        <div>
          <h2 className="text-lg font-semibold">Overview</h2>
          <p className="text-muted-foreground text-sm">
            Everything in your workspace at a glance.
          </p>
        </div>
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          {overviewStats.map(stat => (
            <OverviewCard key={stat.label} stat={stat} loading={statsLoading} />
          ))}
        </div>
      </section>

      <section className="space-y-4">
        <h2 className="text-lg font-semibold">Quick actions</h2>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {QUICK_ACTIONS.map(action => (
            <QuickActionCard key={action.title} action={action} />
          ))}
        </div>
      </section>

      <section className="space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <h2 className="text-lg font-semibold">Analytics</h2>
            <p className="text-muted-foreground text-sm">
              Access, creation and growth across every resource type.
            </p>
          </div>
          {!isEmptyWorkspace && teamId && (
            <SegmentedControl
              options={RANGE_TABS}
              value={range}
              onChange={setRange}
              aria-label="Select analytics time range"
            />
          )}
        </div>
        {isEmptyWorkspace || !teamId ? (
          <AnalyticsEmptyState />
        ) : (
          <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
            <TeamResourceAccessChart teamId={teamId} range={range} />
            <TeamResourceCreationChart teamId={teamId} range={range} />
            <TeamResourceCumulativeChart teamId={teamId} range={range} />
            <TeamFeedCreationChart teamId={teamId} range={range} />
          </div>
        )}
      </section>

      <section className="space-y-4">
        <div>
          <h2 className="text-lg font-semibold">Top accessed resources</h2>
          <p className="text-muted-foreground text-sm">
            Most-opened resources in the selected range. Filter by how they were
            accessed.
          </p>
        </div>
        {teamId ? <TopAccessedResources teamId={teamId} range={range} /> : null}
      </section>

      <section className="space-y-4">
        <h2 className="text-lg font-semibold">Activity</h2>
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <RecentFeedList
            items={feedItems}
            loading={feedLoading}
            error={feedError}
            onViewAll={() => {
              void navigate('/feeds')
            }}
          />
          <RecentActivityList
            activities={activities}
            loading={activitiesLoading}
            error={activitiesError}
            onViewAll={() => {
              void navigate('/settings/activities')
            }}
          />
        </div>
      </section>
    </div>
  )
}
