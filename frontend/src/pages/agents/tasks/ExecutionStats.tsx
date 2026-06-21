import {
  Activity,
  AlertTriangle,
  CheckCircle,
  Clock,
  type LucideIcon,
} from 'lucide-react'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

interface ExecutionStatsProps {
  stats: {
    totalExecutions: number
    successfulExecutions: number
    failedExecutions: number
    runningExecutions: number
    avgDuration: number
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
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-muted-foreground flex items-center gap-2 text-sm font-medium">
          <Icon className="size-4" />
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-2xl font-semibold">{value}</p>
      </CardContent>
    </Card>
  )
}

export function ExecutionStats({ stats }: ExecutionStatsProps) {
  return (
    <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
      <StatCard
        title="Total executions"
        value={stats.totalExecutions.toLocaleString()}
        icon={Activity}
      />
      <StatCard
        title="Successful"
        value={stats.successfulExecutions.toLocaleString()}
        icon={CheckCircle}
      />
      <StatCard
        title="Failed"
        value={stats.failedExecutions.toLocaleString()}
        icon={AlertTriangle}
      />
      <StatCard
        title="Avg duration"
        value={`${stats.avgDuration.toFixed(2)}s`}
        icon={Clock}
      />
    </div>
  )
}
