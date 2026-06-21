import type { LucideIcon } from 'lucide-react'
import { ArrowUpRight, Minus } from 'lucide-react'

import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

export interface Trend {
  label: string
  tone: 'up' | 'flat'
}

export interface OverviewStat {
  label: string
  value: number
  icon: LucideIcon
  trend: Trend | null
  subtitle?: string
}

/**
 * A single Overview metric card: icon, optional trend badge, label, big number,
 * and an optional "+N this week" subtitle. Uses only semantic tokens so it
 * adapts to dark mode. One of the eight cards in the dashboard's Overview grid.
 */
export function OverviewCard({
  stat,
  loading,
}: {
  stat: OverviewStat
  loading: boolean
}) {
  const Icon = stat.icon
  return (
    <Card>
      <CardContent className="p-5">
        <div className="flex items-start justify-between">
          <div className="bg-muted text-foreground flex size-9 items-center justify-center rounded-md">
            <Icon className="size-4" />
          </div>
          {stat.trend && !loading && (
            <span
              className={cn(
                'flex items-center gap-0.5 text-xs font-medium',
                stat.trend.tone === 'up'
                  ? 'text-success'
                  : 'text-muted-foreground'
              )}
            >
              {stat.trend.tone === 'up' ? (
                <ArrowUpRight className="size-3" />
              ) : (
                <Minus className="size-3" />
              )}
              {stat.trend.label}
            </span>
          )}
        </div>
        <p className="text-muted-foreground mt-3 text-sm">{stat.label}</p>
        {loading ? (
          <Skeleton className="mt-1 h-8 w-16" />
        ) : (
          <p className="text-3xl font-bold tabular-nums">
            {stat.value.toLocaleString()}
          </p>
        )}
        {stat.subtitle && !loading && (
          <p className="text-muted-foreground mt-1 text-xs">{stat.subtitle}</p>
        )}
      </CardContent>
    </Card>
  )
}
