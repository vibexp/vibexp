import {
  AlertCircle,
  ArrowLeft,
  FileText,
  FolderKanban,
  Rss,
  Sparkles,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { PageHeader } from '@/components/PageHeader'
import { TeamResourceAccessChart } from '@/components/TeamResourceAccessChart'
import { TeamResourceCreationChart } from '@/components/TeamResourceCreationChart'
import { TIME_SERIES_RANGE_OPTIONS } from '@/components/TimeSeriesBarChart'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import type { Team, TeamStats } from '@/services/teamService'
import { teamService } from '@/services/teamService'
import { getErrorMessage } from '@/utils/errorHandling'

const DEFAULT_RANGE = '30d'

function TeamAnalyticsSkeleton() {
  return (
    <div className="space-y-6">
      <Skeleton className="h-8 w-32" />
      <Skeleton className="h-12 w-2/3" />
      <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-6">
        {['s1', 's2', 's3', 's4', 's5', 's6'].map(key => (
          <Skeleton key={key} className="h-24 w-full" />
        ))}
      </div>
      <Skeleton className="h-64 w-full" />
      <Skeleton className="h-64 w-full" />
    </div>
  )
}

interface StatCardProps {
  label: string
  count: number
  icon: React.ElementType
}

function StatCard({ label, count, icon: Icon }: Readonly<StatCardProps>) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-muted-foreground flex items-center gap-2 text-sm font-medium">
          <Icon className="size-4" />
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-2xl font-semibold">{count}</p>
      </CardContent>
    </Card>
  )
}

export function TeamAnalyticsPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const [team, setTeam] = useState<Team | null>(null)
  const [stats, setStats] = useState<TeamStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  // One page-level range drives every chart on the page.
  const [range, setRange] = useState(DEFAULT_RANGE)

  useEffect(() => {
    const load = async () => {
      if (!id) {
        setError('Missing team context')
        setLoading(false)
        return
      }
      try {
        setLoading(true)
        setError(null)
        const [teamData, statsData] = await Promise.all([
          teamService.getTeamDetails(id),
          teamService.getTeamStats(id),
        ])
        setTeam(teamData)
        setStats(statsData)
      } catch (err) {
        setError(getErrorMessage(err, 'Failed to load team analytics'))
      } finally {
        setLoading(false)
      }
    }
    void load()
  }, [id])

  if (loading) {
    return <TeamAnalyticsSkeleton />
  }

  if (error || !team || !id) {
    return (
      <div className="space-y-4">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => {
            void navigate('/settings/teams')
          }}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back to Teams
        </Button>
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Could not load team analytics</AlertTitle>
          <AlertDescription>
            {error ?? 'The team could not be found.'}
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <Button
        variant="ghost"
        size="sm"
        onClick={() => {
          void navigate(`/settings/teams/${id}`)
        }}
      >
        <ArrowLeft className="mr-2 size-4" />
        Back to Team
      </Button>

      <PageHeader
        title="Team Analytics"
        description={`Activity and resource trends for ${team.name}`}
        actions={
          <Select value={range} onValueChange={setRange}>
            <SelectTrigger className="w-[160px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TIME_SERIES_RANGE_OPTIONS.map(option => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />

      {stats && (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold">Resources (all time)</h2>
          <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-6">
            <StatCard
              label="Projects"
              count={stats.total_projects}
              icon={FolderKanban}
            />
            <StatCard
              label="Prompts"
              count={stats.total_prompts}
              icon={Sparkles}
            />
            <StatCard
              label="Artifacts"
              count={stats.total_artifacts}
              icon={FileText}
            />
            <StatCard
              label="Blueprints"
              count={stats.total_blueprints}
              icon={FolderKanban}
            />
            <StatCard
              label="Memories"
              count={stats.total_memories}
              icon={Sparkles}
            />
            <StatCard
              label="Feed Items"
              count={stats.total_feed_items}
              icon={Rss}
            />
          </div>
        </section>
      )}

      <TeamResourceAccessChart teamId={id} range={range} />
      <TeamResourceCreationChart teamId={id} range={range} />
    </div>
  )
}
