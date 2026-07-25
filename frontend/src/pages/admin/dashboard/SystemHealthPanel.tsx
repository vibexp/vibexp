import { Database } from 'lucide-react'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { formatBytes } from '@/pages/admin/dashboard/format'
import type { AdminSystemHealth } from '@/services/adminService'

export function SystemHealthPanel({
  health,
  version,
  loading,
}: Readonly<{
  health: AdminSystemHealth | null
  version: string
  loading: boolean
}>) {
  if (loading) return <Skeleton className="h-64 w-full" />
  if (!health) return null

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-muted-foreground flex items-center gap-2 text-xs font-medium">
            <Database className="size-4" aria-hidden />
            Database size
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-1">
          <p className="text-2xl font-semibold tabular-nums">
            {formatBytes(health.database_size_bytes)}
          </p>
          <p className="text-muted-foreground text-xs">
            Backend version {version}
          </p>
        </CardContent>
      </Card>

      <Card className="lg:col-span-2">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium">Largest tables</CardTitle>
          {/* Labelled as an estimate because it is one: the API reads
              pg_stat_user_tables.n_live_tup, whose freshness depends on
              autovacuum. Presenting it as an exact count would be a lie an admin
              might act on. */}
          <p className="text-muted-foreground text-xs">
            Estimated row counts from table statistics, not exact counts.
          </p>
        </CardHeader>
        <CardContent>
          {health.tables.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              No table statistics available yet.
            </p>
          ) : (
            <ul className="divide-y">
              {health.tables.map(table => (
                <li
                  key={table.table}
                  className="flex items-center justify-between py-1.5 text-sm"
                >
                  <span className="font-mono text-xs">{table.table}</span>
                  <span className="tabular-nums">
                    ~{table.estimated_rows.toLocaleString()}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
