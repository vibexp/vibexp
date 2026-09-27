import { TaxonomyInput } from '@/components/patterns/resource'
import { labelVariants } from '@/components/ui/label'

export interface MemoryTagsFieldProps {
  value: string[]
  onChange: (next: string[]) => void
  disabled?: boolean
}

/**
 * The memory form's `tags` extension slot: a field inside Labels & metadata,
 * labelled like the labels input beside it, where the reading page shows the
 * tags as chips — so the create and edit pages add no card or heading of
 * their own (#1180).
 *
 * Memory declares no `taxonomy` FIELD for tags: they are lifted out of the
 * free-form `metadata` bag, exactly as `ResourceTaxonomySection` lifts them on
 * the detail page. That lift is a display choice rather than a property of the
 * resource, which is why the descriptor declares a slot and the page fills it.
 */
export function MemoryTagsField({
  value,
  onChange,
  disabled = false,
}: Readonly<MemoryTagsFieldProps>) {
  return (
    <div className="space-y-2" data-testid="memory-tags-field">
      <p className={labelVariants()}>Tags</p>
      <TaxonomyInput
        value={value}
        onChange={onChange}
        disabled={disabled}
        placeholder="Add tags (comma-separated)"
        aria-label="Tags"
        data-testid="memory-tags-input"
      />
    </div>
  )
}
