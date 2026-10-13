import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

/**
 * One block of the dashboard: a heading, an optional one-line description and
 * an optional control beside them, then the content.
 *
 * Separation is the heading plus spacing — the section itself draws no border
 * and no shadow. Content that needs containment uses {@link DASHBOARD_GROUP},
 * a single 1px border, and nothing bordered goes inside it.
 */
export function DashboardSection({
  title,
  description,
  aside,
  children,
  className,
}: Readonly<{
  title: string
  description?: ReactNode
  aside?: ReactNode
  children: ReactNode
  className?: string
}>) {
  return (
    <section className={cn('space-y-3', className)} aria-label={title}>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0">
          <h2 className="text-base font-semibold">{title}</h2>
          {description && (
            <p className="text-muted-foreground text-sm">{description}</p>
          )}
        </div>
        {/* A wide control (the range switch) scrolls inside itself on a phone. */}
        {aside && <div className="max-w-full overflow-x-auto">{aside}</div>}
      </div>
      {children}
    </section>
  )
}

/** A contained group of rows: one border, no shadow, rows split by a rule. */
export const DASHBOARD_GROUP = 'divide-y rounded-lg border'

/** A row inside {@link DASHBOARD_GROUP}. */
export const DASHBOARD_ROW = 'flex items-center gap-3 px-4 py-2.5'
