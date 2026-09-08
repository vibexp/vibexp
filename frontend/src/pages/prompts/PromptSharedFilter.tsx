import { FILTER_CONTROL_WIDTH } from '@/components/patterns/list-page'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export type SharedFilter = 'all' | 'shared' | 'not_shared'

/**
 * The prompts list's "Shared" tri-state — the one control on the four resource
 * list bars that is not generated from the descriptor (#908).
 *
 * It stays page-local on purpose: `is_shared` is not a property of the prompt
 * but of an active share, so it has no `FieldSpec` to hang a `FilterSpec` off.
 * `ResourceFilterBar` renders it through its `extras` slot, which is what that
 * slot is for — a resource-specific control, not an escape hatch for filters
 * that simply have not been declared yet.
 */
export function PromptSharedFilter({
  value,
  onChange,
}: Readonly<{
  value: SharedFilter
  onChange: (value: SharedFilter) => void
}>) {
  return (
    <Select
      value={value}
      onValueChange={next => {
        onChange(next as SharedFilter)
      }}
    >
      <SelectTrigger
        className={FILTER_CONTROL_WIDTH}
        aria-label="Filter by shared"
        data-testid="prompt-shared-filter"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">All</SelectItem>
        <SelectItem value="shared">Shared</SelectItem>
        <SelectItem value="not_shared">Not shared</SelectItem>
      </SelectContent>
    </Select>
  )
}
