import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { AdminEntityBreakdown } from '@/services/adminService'

/** Turns `prompts`/`status` into "Prompts by status". */
function breakdownTitle(breakdown: AdminEntityBreakdown): string {
  const entity =
    breakdown.entity.charAt(0).toUpperCase() + breakdown.entity.slice(1)
  return `${entity} by ${breakdown.field.replace(/_/g, ' ')}`
}

export function BreakdownPanels({
  breakdowns,
  loading,
}: Readonly<{
  breakdowns: readonly AdminEntityBreakdown[] | null
  loading: boolean
}>) {
  if (loading) {
    return (
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
        {['a', 'b', 'c'].map(slot => (
          <Skeleton key={slot} className="h-40 w-full" />
        ))}
      </div>
    )
  }
  if (!breakdowns) return null
  if (breakdowns.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        No status or type columns to break down on this instance.
      </p>
    )
  }

  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
      {breakdowns.map(breakdown => {
        const total = breakdown.buckets.reduce(
          (sum, bucket) => sum + bucket.count,
          0
        )
        return (
          <Card key={`${breakdown.entity}.${breakdown.field}`}>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium">
                {breakdownTitle(breakdown)}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              {breakdown.buckets.length === 0 ? (
                <p className="text-muted-foreground text-sm">No rows yet.</p>
              ) : (
                breakdown.buckets.map(bucket => (
                  <div key={bucket.value || '(unset)'} className="space-y-1">
                    <div className="flex items-baseline justify-between gap-2 text-sm">
                      {/* The API reports NULL as an empty string, which would
                          render as a nameless row. */}
                      <span className="truncate">
                        {bucket.value === '' ? 'Not set' : bucket.value}
                      </span>
                      <span className="tabular-nums">
                        {bucket.count.toLocaleString()}
                      </span>
                    </div>
                    <div className="bg-muted h-1.5 w-full overflow-hidden rounded-full">
                      <div
                        className="bg-primary h-full rounded-full"
                        style={{
                          width: `${String(total === 0 ? 0 : Math.round((bucket.count / total) * 100))}%`,
                        }}
                      />
                    </div>
                  </div>
                ))
              )}
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}
