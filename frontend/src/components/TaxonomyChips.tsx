import { Badge } from '@/components/ui/badge'

interface TaxonomyChipsProps {
  /** The grouping values, in payload order. */
  values: readonly string[]
}

/**
 * A row of chips for one list-shaped taxonomy value — prompt labels, memory
 * tags, a blueprint's subtype.
 *
 * Before #904 each page owned a copy of this markup and they had drifted:
 * prompt labels were `outline` badges in a "Labels" card, memory tags were
 * `secondary` badges carrying a tag icon in a "Tags" card. One treatment now,
 * so a new taxonomy field renders like every other one for free.
 *
 * Renders nothing when there is nothing to show; the emptiness rule for the
 * whole section belongs to `ResourceTaxonomySection`, not here.
 */
export function TaxonomyChips({ values }: Readonly<TaxonomyChipsProps>) {
  if (values.length === 0) return null
  return (
    <div className="flex flex-wrap gap-1.5">
      {values.map(value => (
        <Badge key={value} variant="secondary">
          {value}
        </Badge>
      ))}
    </div>
  )
}
