import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

type StatusTone =
  | 'default'
  | 'success'
  | 'warning'
  | 'destructive'
  | 'info'
  | 'neutral'

// Status roles render as solid fills — the design-system documents `--x` as the
// solid badge fill, paired with its WCAG-checked `--x-foreground`. Tokens flip
// automatically under `.dark`, so no per-mode text variants are needed.
const TONE_CLASS: Record<StatusTone, string> = {
  default: '',
  success: 'border-transparent bg-success text-success-foreground',
  warning: 'border-transparent bg-warning text-warning-foreground',
  destructive: 'border-transparent bg-destructive text-destructive-foreground',
  info: 'border-transparent bg-info text-info-foreground',
  neutral: 'border-transparent bg-muted text-muted-foreground',
}

interface StatusBadgeProps {
  tone?: StatusTone
  className?: string
  children: React.ReactNode
}

export function StatusBadge({
  tone = 'default',
  className,
  children,
}: StatusBadgeProps) {
  if (tone === 'default') {
    return <Badge className={className}>{children}</Badge>
  }
  return (
    <Badge variant="outline" className={cn(TONE_CLASS[tone], className)}>
      {children}
    </Badge>
  )
}
