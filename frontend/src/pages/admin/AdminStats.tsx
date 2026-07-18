import {
  FileText,
  HardDrive,
  type LucideIcon,
  Package,
  Tag,
  Users,
  UsersRound,
} from 'lucide-react'
import { useEffect, useState } from 'react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { AdminStatsResponse } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

function StatCard({
  title,
  value,
  icon: Icon,
}: Readonly<{ title: string; value: string; icon: LucideIcon }>) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-muted-foreground flex items-center gap-2 text-sm font-medium">
          <Icon className="size-4" />
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-2xl font-semibold tabular-nums">{value}</p>
      </CardContent>
    </Card>
  )
}

/** Instance statistics dashboard — the `/admin` landing page (#316). */
export function AdminStats() {
  const [stats, setStats] = useState<AdminStatsResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    adminService
      .getStats()
      .then(result => {
        if (!cancelled) setStats(result)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(getErrorMessage(err, 'Failed to load stats'))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (loading) {
    return (
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {Array.from({ length: 6 }, (_, i) => `card-${String(i)}`).map(slot => (
          <Skeleton
            key={slot}
            data-testid="stat-skeleton"
            className="h-28 w-full"
          />
        ))}
      </div>
    )
  }

  if (error) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Failed to load stats</AlertTitle>
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    )
  }

  if (!stats) return null

  const { counts, version } = stats
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
      <StatCard
        title="Users"
        value={counts.users.toLocaleString()}
        icon={Users}
      />
      <StatCard
        title="Teams"
        value={counts.teams.toLocaleString()}
        icon={UsersRound}
      />
      <StatCard
        title="Prompts"
        value={counts.prompts.toLocaleString()}
        icon={FileText}
      />
      <StatCard
        title="Artifacts"
        value={counts.artifacts.toLocaleString()}
        icon={Package}
      />
      <StatCard
        title="Memories"
        value={counts.memories.toLocaleString()}
        icon={HardDrive}
      />
      <StatCard title="Version" value={version} icon={Tag} />
    </div>
  )
}
