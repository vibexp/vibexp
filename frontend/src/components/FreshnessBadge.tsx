import type { components } from '@vibexp/api-client'

import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatDate } from '@/lib/time'

export type ResourceFreshnessState =
  components['schemas']['ResourceFreshnessState']

/**
 * The stale marker shown on resource rows and cards (#738, epic #726).
 *
 * One shared component for all four resource types rather than a copy per list,
 * so the four surfaces cannot drift apart in wording or styling.
 *
 * **Renders nothing when `freshness` is absent**, which is how the API says
 * "fresh" — the field is optional and omitted entirely for a healthy resource
 * (#735). Callers therefore pass the payload field straight through without
 * checking it first, and a fresh row costs no markup and no layout shift.
 *
 * Styling is deliberately quiet (`outline`, muted text). Staleness is
 * informational, not an error: an over-broad rule can flag a lot at once, and a
 * loud badge would train people to ignore it.
 */
export function FreshnessBadge({
  freshness,
  className,
}: Readonly<{
  freshness?: ResourceFreshnessState | null
  className?: string
}>) {
  if (!freshness) return null

  const ruleCount = freshness.matched_rule_ids.length

  // Freshness rules carry no name — a rule IS its criteria (types, project,
  // threshold), so there is nothing to print but a count. The Resource
  // Freshness settings page is where the criteria live; naming them here would
  // mean inventing labels or dumping UUIDs at the user.
  const ruleLine =
    ruleCount > 0
      ? `Flagged by ${String(ruleCount)} freshness rule${ruleCount === 1 ? '' : 's'}.`
      : 'The rule that flagged it no longer exists.'

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge
            variant="outline"
            className={`text-muted-foreground font-normal ${className ?? ''}`}
            data-testid="freshness-badge"
          >
            Stale
          </Badge>
        </TooltipTrigger>
        <TooltipContent>
          <p>Not used since {formatDate(freshness.since)}.</p>
          <p className="text-muted-foreground">{ruleLine}</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
