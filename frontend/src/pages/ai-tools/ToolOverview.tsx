import {
  Activity,
  BarChart3,
  Clock,
  History,
  type LucideIcon,
  Settings,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { EmptyState } from '@/components/EmptyState'
import type { RecentActivity } from '@/services/aiToolsService'

// Common shape read by this shared shell for both Claude Code (`OverviewStats`)
// and Cursor IDE (`CursorOverviewStats`); `top_tools[].tool_name` is optional to
// stay assignable from the generated stats types.
interface ToolStats {
  total_sessions: number
  sessions_this_week: number
  weekly_trend_percent: number
  avg_user_prompts_per_session: number
  total_unique_tools: number
  top_tools: { tool_name?: string }[]
  avg_session_duration_minutes: number
}
import { PageHeader } from '@/components/PageHeader'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'

function formatTrend(percent: number, thisWeek: number) {
  if (percent === 0) return 'No change from last week'
  const sign = percent > 0 ? '+' : ''
  return `${sign}${percent.toFixed(1)}% vs last week (${String(thisWeek)} this week)`
}

function formatDuration(minutes: number): string {
  if (minutes < 60) return `${String(Math.round(minutes))}m`
  const hours = Math.floor(minutes / 60)
  const mins = Math.round(minutes % 60)
  return `${String(hours)}h ${String(mins)}m`
}

function formatTimeAgo(dateString: string): string {
  const date = new Date(dateString)
  const diff = Math.floor((Date.now() - date.getTime()) / 1000)
  if (diff < 60) return 'just now'
  if (diff < 3600) return `${String(Math.floor(diff / 60))}m ago`
  if (diff < 86400) return `${String(Math.floor(diff / 3600))}h ago`
  return `${String(Math.floor(diff / 86400))}d ago`
}

function formatToolInput(input: Record<string, unknown> | null | undefined) {
  if (!input) return ''
  if (typeof input.command === 'string') return `(${input.command})`
  if (typeof input.pattern === 'string') return `(${input.pattern})`
  if (typeof input.file_path === 'string') return `(${input.file_path})`
  return ''
}

function formatCwd(cwd: string | null | undefined): string {
  if (!cwd) return ''
  const parts = cwd.split('/')
  return parts[parts.length - 1] || cwd
}

interface ToolOverviewProps {
  title: string
  description: string
  sessionsHref: string
  setupHref: string
  fetchStats: () => Promise<ToolStats>
  fetchActivities: () => Promise<{ activities: RecentActivity[] }>
}

export function ToolOverview({
  title,
  description,
  sessionsHref,
  setupHref,
  fetchStats,
  fetchActivities,
}: Readonly<ToolOverviewProps>) {
  const navigate = useNavigate()
  const [stats, setStats] = useState<ToolStats | null>(null)
  const [activities, setActivities] = useState<RecentActivity[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const load = async () => {
      try {
        const [statsRes, activitiesRes] = await Promise.all([
          fetchStats(),
          fetchActivities(),
        ])
        setStats(statsRes)
        setActivities(activitiesRes.activities)
      } catch (error) {
        console.error('Failed to fetch overview data:', error)
      } finally {
        setLoading(false)
      }
    }
    void load()
  }, [fetchStats, fetchActivities])

  const statsData = [
    {
      label: 'Total sessions',
      value: (stats?.total_sessions ?? 0).toString(),
      trend: formatTrend(
        stats?.weekly_trend_percent ?? 0,
        stats?.sessions_this_week ?? 0
      ),
      icon: Activity,
    },
    {
      label: 'Average duration',
      value: formatDuration(stats?.avg_session_duration_minutes ?? 0),
      trend: `${(stats?.avg_session_duration_minutes ?? 0).toFixed(1)} minutes average`,
      icon: Clock,
    },
    {
      label: 'Unique tools',
      value: (stats?.total_unique_tools ?? 0).toString(),
      trend:
        stats?.top_tools && stats.top_tools.length > 0
          ? `Top: ${stats.top_tools
              .slice(0, 3)
              .map(t => t.tool_name)
              .join(', ')}`
          : 'No tool data',
      icon: Settings,
    },
    {
      label: 'Avg user prompts',
      value: (stats?.avg_user_prompts_per_session ?? 0).toFixed(1),
      trend: 'Per session average',
      icon: BarChart3,
    },
  ]

  return (
    <div className="space-y-6">
      <PageHeader
        title={title}
        description={description}
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => {
                void navigate(sessionsHref)
              }}
            >
              <History className="mr-2 size-4" />
              Sessions
            </Button>
            <Button
              onClick={() => {
                void navigate(setupHref)
              }}
            >
              <Settings className="mr-2 size-4" />
              Setup
            </Button>
          </>
        }
      />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {statsData.map(stat => (
          <StatCard
            key={stat.label}
            label={stat.label}
            value={stat.value}
            trend={stat.trend}
            icon={stat.icon}
            loading={loading}
          />
        ))}
      </div>

      <section className="space-y-3">
        <h2 className="text-lg font-semibold">Recent activity</h2>
        <Card>
          <CardContent className="space-y-2 p-4">
            {loading ? (
              <div className="space-y-3">
                {Array.from({ length: 4 }).map((_, i) => (
                  <div key={i} className="flex items-center gap-3 py-2">
                    <Skeleton className="size-2 rounded-full" />
                    <div className="flex-1 space-y-1.5">
                      <Skeleton className="h-4 w-2/3" />
                      <Skeleton className="h-3 w-1/3" />
                    </div>
                    <Skeleton className="h-3 w-12" />
                  </div>
                ))}
              </div>
            ) : activities.length === 0 ? (
              <EmptyState
                icon={Activity}
                title="No recent activity"
                description={`${title} sessions and tool events will appear here.`}
              />
            ) : (
              <>
                <ul className="divide-y font-mono text-sm">
                  {activities.map((activity, index) => (
                    <li
                      key={`${activity.session_id}-${String(index)}`}
                      className="space-y-1 py-3"
                    >
                      <div className="flex items-center gap-2">
                        <span className="text-success">●</span>
                        <span className="font-medium">
                          {activity.tool_name ?? 'System'}
                        </span>
                        <span className="text-muted-foreground">
                          {formatToolInput(activity.tool_input)}
                        </span>
                        {activity.cwd && (
                          <span className="text-muted-foreground/70 text-xs">
                            ({formatCwd(activity.cwd)})
                          </span>
                        )}
                      </div>
                      <div className="text-muted-foreground ml-4 flex items-center gap-2 text-xs">
                        <span>⎿</span>
                        <span className="bg-muted rounded px-1.5 py-0.5 font-medium">
                          {activity.hook_event_name}
                        </span>
                        <span className="ml-auto">
                          {formatTimeAgo(activity.created_at)}
                        </span>
                      </div>
                    </li>
                  ))}
                </ul>
                <Separator />
                <div className="flex justify-center pt-2">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      void navigate(sessionsHref)
                    }}
                  >
                    View all sessions
                  </Button>
                </div>
              </>
            )}
          </CardContent>
        </Card>
      </section>
    </div>
  )
}

interface StatCardProps {
  label: string
  value: string
  trend: string
  icon: LucideIcon
  loading: boolean
}

function StatCard({
  label,
  value,
  trend,
  icon: Icon,
  loading,
}: Readonly<StatCardProps>) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="bg-muted flex size-10 items-center justify-center rounded-lg">
          <Icon className="size-5" />
        </div>
      </CardHeader>
      <CardContent className="space-y-1">
        <CardTitle className="text-muted-foreground text-sm font-normal">
          {label}
        </CardTitle>
        {loading ? (
          <Skeleton className="h-8 w-20" />
        ) : (
          <p className="text-2xl font-semibold">{value}</p>
        )}
        <p className="text-muted-foreground text-xs">
          {loading ? <Skeleton className="h-3 w-32" /> : trend}
        </p>
      </CardContent>
    </Card>
  )
}
