import {
  Activity,
  BarChart3,
  CheckCircle,
  type LucideIcon,
  Settings,
} from 'lucide-react'

interface AgentStatsProps {
  stats: {
    totalAgents: number
    activeAgents: number
    pausedAgents: number
    errorAgents: number
    totalRuns: number
    avgSuccessRate: number
  }
}

function StatCard({
  title,
  value,
  icon: Icon,
}: {
  title: string
  value: string
  icon: LucideIcon
}) {
  return (
    <div className="bg-muted/50 rounded-lg p-5">
      <div className="flex items-center gap-3">
        <div className="bg-background text-foreground flex size-10 shrink-0 items-center justify-center rounded-lg shadow-sm">
          <Icon className="size-5" />
        </div>
        <div className="space-y-0.5 min-w-0">
          <p className="text-muted-foreground text-sm">{title}</p>
          <p className="text-2xl font-semibold">{value}</p>
        </div>
      </div>
    </div>
  )
}

export function AgentStats({ stats }: AgentStatsProps) {
  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
      <StatCard
        title="Total agents"
        value={stats.totalAgents.toLocaleString()}
        icon={BarChart3}
      />
      <StatCard
        title="Active agents"
        value={stats.activeAgents.toLocaleString()}
        icon={CheckCircle}
      />
      <StatCard
        title="Total runs"
        value={stats.totalRuns.toLocaleString()}
        icon={Activity}
      />
      <StatCard
        title="Success rate"
        value={`${String(Math.round(stats.avgSuccessRate))}%`}
        icon={Settings}
      />
    </div>
  )
}
