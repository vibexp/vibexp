import { cn } from '@/lib/utils'

export interface SegmentedOption {
  value: string
  label: string
}

interface SegmentedControlProps {
  options: readonly SegmentedOption[]
  value: string
  onChange: (value: string) => void
  /** `sm` is used for the compact in-card channel filters. */
  size?: 'sm' | 'md'
  'aria-label'?: string
}

/**
 * A token-driven segmented button group (the design's range / access-channel
 * selector). Uses only semantic design-system tokens so it adapts to dark mode.
 * The active segment lifts onto the card surface; the rest stay muted.
 */
export function SegmentedControl({
  options,
  value,
  onChange,
  size = 'md',
  'aria-label': ariaLabel,
}: Readonly<SegmentedControlProps>) {
  return (
    <div
      role="tablist"
      aria-label={ariaLabel}
      className={cn(
        'bg-muted inline-flex items-center rounded-md',
        size === 'sm' ? 'gap-0.5 p-0.5' : 'gap-1 p-1'
      )}
    >
      {options.map(option => {
        const active = option.value === value
        return (
          <button
            key={option.value}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => {
              onChange(option.value)
            }}
            className={cn(
              'rounded font-medium whitespace-nowrap transition-colors',
              size === 'sm' ? 'px-2 py-1 text-xs' : 'px-3 py-1.5 text-sm',
              active
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
            )}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}
