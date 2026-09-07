import { Check, Copy } from 'lucide-react'

import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { cn } from '@/lib/utils'

/**
 * The chip's visual treatment, exported so the reading header's read-only
 * address chip renders identically instead of re-declaring these tokens.
 */
export const COPY_CHIP_CLASS =
  'flex min-w-0 max-w-full items-center justify-end gap-1.5 rounded-sm bg-secondary px-2 py-[3px] font-mono text-xs text-secondary-foreground'

/**
 * A monospace chip that is itself the click-to-copy target — the one field
 * people actually grab (a slug, an id). The whole chip is a real `<button>`
 * (keyboard-activatable, with an `aria-label` that names the value, since the
 * label overrides the inner `<code>`), so nothing has to reserve space for a
 * separate copy button. On copy it swaps to a check affordance for ~1.5s.
 * Long values truncate to a single line with a trailing ellipsis — the full
 * value is still copied and still announced.
 *
 * Shared by the details panel's `MetaSlugRow` and the reading page's header
 * address, so a resource's address looks and behaves the same in both places.
 */
export function CopyChip({
  value,
  label = 'Slug',
  className,
}: Readonly<{
  value: string
  /** Names the thing being copied, in the tooltip and the accessible name. */
  label?: string
  className?: string
}>) {
  const { copied, copy } = useCopyToClipboard()
  const CopyIcon = copied ? Check : Copy
  const action = `Copy ${label.toLowerCase()}`

  return (
    <button
      type="button"
      onClick={() => {
        copy(value)
      }}
      aria-label={`${action}: ${value}`}
      title={copied ? 'Copied!' : action}
      className={cn(
        COPY_CHIP_CLASS,
        'cursor-pointer transition-colors hover:bg-secondary/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        className
      )}
    >
      <code className="min-w-0 truncate font-mono">{value}</code>
      <CopyIcon
        aria-hidden="true"
        className="size-3 shrink-0 text-muted-foreground"
      />
    </button>
  )
}
