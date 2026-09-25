import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { PROJECT_RESOURCE_BREAKDOWN } from '@/pages/admin/projects/projectListParams'
import type { AdminProjectResourceCounts } from '@/services/adminService'

const count = (n: number, singular: string, plural: string) =>
  `${String(n)} ${n === 1 ? singular : plural}`

/**
 * The "Resources" cell of the admin projects list (#1144): the project-scoped
 * total, with the per-type breakdown in a tooltip. Counts only — never titles
 * or slugs.
 *
 * The breakdown is also the trigger's accessible name, so it is not
 * tooltip-only. The trigger is a real inline-flex box: a `display: contents`
 * trigger has no box for floating-ui to measure and anchors the tooltip on the
 * viewport origin (#891).
 */
export function ProjectResourceSummary({
  counts,
}: Readonly<{ counts: AdminProjectResourceCounts }>) {
  const breakdown = PROJECT_RESOURCE_BREAKDOWN.map(type =>
    count(counts[type.key], type.singular, type.plural)
  ).join(', ')
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            role="img"
            aria-label={`${count(counts.total, 'resource', 'resources')}: ${breakdown}`}
            className="inline-flex text-sm tabular-nums"
          >
            {counts.total}
          </span>
        </TooltipTrigger>
        <TooltipContent>
          <dl className="grid grid-cols-[auto_auto] gap-x-3 gap-y-0.5">
            {PROJECT_RESOURCE_BREAKDOWN.map(type => (
              <div key={type.key} className="contents">
                <dt>{type.label}</dt>
                <dd className="text-right tabular-nums">{counts[type.key]}</dd>
              </div>
            ))}
          </dl>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
