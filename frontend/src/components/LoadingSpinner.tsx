import { Loader2 } from 'lucide-react'

import { cn } from '@/lib/utils'

interface LoadingSpinnerProps {
  size?: 'sm' | 'md' | 'lg'
  label?: string
  className?: string
}

const SIZE_CLASS = {
  sm: 'size-4',
  md: 'size-6',
  lg: 'size-8',
} as const

export function LoadingSpinner({
  size = 'md',
  label,
  className,
}: Readonly<LoadingSpinnerProps>) {
  return (
    <div
      className={cn('text-muted-foreground flex items-center gap-2', className)}
      role="status"
    >
      <Loader2 className={cn('animate-spin', SIZE_CLASS[size])} />
      {label && <span className="text-sm">{label}</span>}
      <span className="sr-only">Loading</span>
    </div>
  )
}
