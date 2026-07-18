import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

interface EmptyStateProps {
  icon?: LucideIcon
  title: string
  description?: ReactNode
  actions?: ReactNode
  className?: string
}

export function EmptyState({
  icon: Icon,
  title,
  description,
  actions,
  className,
}: Readonly<EmptyStateProps>) {
  return (
    <div
      data-testid="empty-state"
      className={cn(
        'flex flex-col items-center justify-center rounded-lg border border-dashed p-12 text-center',
        className
      )}
    >
      {Icon && (
        <div className="bg-muted text-muted-foreground mb-4 flex size-12 items-center justify-center rounded-full">
          <Icon className="size-6" />
        </div>
      )}
      <h3 className="text-lg font-medium">{title}</h3>
      {description && (
        <p
          data-testid="empty-state-message"
          className="text-muted-foreground mt-1 max-w-md text-sm"
        >
          {description}
        </p>
      )}
      {actions && <div className="mt-4 flex gap-2">{actions}</div>}
    </div>
  )
}
