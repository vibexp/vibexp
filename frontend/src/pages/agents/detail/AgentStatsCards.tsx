import {
  Activity,
  Calendar,
  CheckCircle,
  Clock,
  type LucideIcon,
} from 'lucide-react'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { Agent } from '@/services/agentService'

import { formatDate, successRateColor } from '../helpers'

interface AgentStatsCardsProps {
  agent: Agent
}

function StatCard({
  title,
  value,
  icon: Icon,
  valueClassName,
}: {
  title: string
  value: string
  icon: LucideIcon
  valueClassName?: string
}) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-muted-foreground flex items-center gap-2 text-sm font-medium">
          <Icon className="size-4" />
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className={`text-2xl font-semibold ${valueClassName ?? ''}`}>
          {value}
        </p>
      </CardContent>
    </Card>
  )
}

export function AgentStatsCards({ agent }: AgentStatsCardsProps) {
  const percentage = Math.round(agent.success_rate)
  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
      <StatCard
        title="Success rate"
        value={`${String(percentage)}%`}
        icon={CheckCircle}
        valueClassName={successRateColor(percentage)}
      />
      <StatCard
        title="Total runs"
        value={agent.total_runs.toLocaleString()}
        icon={Activity}
      />
      <StatCard
        title="Last run"
        value={formatDate(agent.last_run)}
        icon={Clock}
      />
      <StatCard
        title="Created"
        value={formatDate(agent.created_at)}
        icon={Calendar}
      />
    </div>
  )
}
