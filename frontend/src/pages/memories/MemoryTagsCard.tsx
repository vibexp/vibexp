import { TaxonomyInput } from '@/components/patterns/resource'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

export interface MemoryTagsCardProps {
  value: string[]
  onChange: (next: string[]) => void
  disabled?: boolean
}

/**
 * The memory form's `tags` extension slot.
 *
 * Memory declares no `taxonomy` FIELD for tags: they are lifted out of the
 * free-form `metadata` bag, exactly as `ResourceTaxonomySection` lifts them on
 * the detail page. That lift is a display choice rather than a property of the
 * resource, which is why the descriptor declares a slot and the page fills it —
 * but the chip editor inside it is the shared one, not the bespoke copy
 * `MemoryForm` carried.
 */
export function MemoryTagsCard({
  value,
  onChange,
  disabled = false,
}: Readonly<MemoryTagsCardProps>) {
  return (
    <Card data-testid="memory-tags-card">
      <CardHeader>
        <CardTitle className="text-sm">Tags</CardTitle>
      </CardHeader>
      <CardContent>
        <TaxonomyInput
          value={value}
          onChange={onChange}
          disabled={disabled}
          placeholder="Add tags (comma-separated)"
          aria-label="Tags"
          data-testid="memory-tags-input"
        />
      </CardContent>
    </Card>
  )
}
