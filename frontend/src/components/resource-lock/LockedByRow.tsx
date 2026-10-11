import { Lock } from 'lucide-react'

import { MetaRow } from '@/components/metadata/MetadataPanel'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { ResourceLock } from '@/types/resourceLock'

/**
 * The Metadata section's "Locked by" row: who holds the lock, with the lock
 * comment in a tooltip reachable by hover and by keyboard focus.
 *
 * The trigger is a real `<button>`: it is focusable, and it has a box. A
 * `display: contents` wrapper has none for floating-ui to measure and would
 * anchor the tooltip on the viewport origin (#891).
 */
export function LockedByRow({ lock }: Readonly<{ lock: ResourceLock }>) {
  return (
    <MetaRow label="Locked by">
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              data-testid="locked-by"
              className="inline-flex min-w-0 cursor-default items-center gap-1.5 rounded-sm focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
            >
              <Lock
                aria-hidden="true"
                className="size-[13px] shrink-0 text-muted-foreground"
              />
              <span className="truncate">{lock.locked_by.display_name}</span>
            </button>
          </TooltipTrigger>
          <TooltipContent className="max-w-xs whitespace-pre-wrap break-words">
            {lock.comment}
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    </MetaRow>
  )
}
