import type { LucideIcon } from 'lucide-react'

import { Skeleton } from '@/components/ui/skeleton'

export interface OverviewStat {
  label: string
  value: number
  icon: LucideIcon
  /** New in the last 7 days; 0 or undefined shows nothing. */
  weeklyDelta?: number
}

/**
 * The workspace's counts as one compact row of small stats. Counts say how
 * much exists, which nobody can act on, so they sit low on the page and take
 * one line instead of a grid of large cards.
 */
export function WorkspaceTotals({
  stats,
  loading,
}: Readonly<{ stats: readonly OverviewStat[]; loading: boolean }>) {
  return (
    <dl className="grid grid-cols-2 gap-x-6 gap-y-4 rounded-lg border px-4 py-3 sm:grid-cols-3 lg:grid-cols-6">
      {stats.map(stat => {
        const Icon = stat.icon
        return (
          <div key={stat.label} className="min-w-0">
            <dt className="text-muted-foreground flex items-center gap-1.5 text-xs">
              <Icon className="size-3.5 shrink-0" aria-hidden="true" />
              <span className="truncate">{stat.label}</span>
            </dt>
            <dd className="mt-0.5 flex items-baseline gap-2">
              {loading ? (
                <Skeleton className="h-6 w-10" />
              ) : (
                <>
                  <span className="text-lg font-semibold tabular-nums">
                    {stat.value.toLocaleString()}
                  </span>
                  {stat.weeklyDelta !== undefined && stat.weeklyDelta > 0 && (
                    <span className="text-success text-xs">
                      +{stat.weeklyDelta.toLocaleString()} this week
                    </span>
                  )}
                </>
              )}
            </dd>
          </div>
        )
      })}
    </dl>
  )
}
