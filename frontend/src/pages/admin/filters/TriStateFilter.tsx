import { useId } from 'react'

import { Button } from '@/components/ui/button'

export interface TriStateFilterProps {
  label: string
  /** `undefined` = any. */
  value: boolean | undefined
  onChange: (next: boolean | undefined) => void
}

const OPTIONS: readonly { label: string; value: boolean | undefined }[] = [
  { label: 'Any', value: undefined },
  { label: 'Yes', value: true },
  { label: 'No', value: false },
]

/**
 * Any / Yes / No as a three-button radio group. Deliberately not a Radix
 * `Select`: one click shorter, and much cheaper to render under jsdom.
 */
export function TriStateFilter({
  label,
  value,
  onChange,
}: Readonly<TriStateFilterProps>) {
  const labelId = useId()
  return (
    <div className="flex flex-col gap-1.5">
      <span id={labelId} className="text-sm font-medium leading-none">
        {label}
      </span>
      <div role="radiogroup" aria-labelledby={labelId} className="flex gap-1">
        {OPTIONS.map(option => {
          const checked = option.value === value
          return (
            <Button
              key={option.label}
              type="button"
              role="radio"
              aria-checked={checked}
              variant={checked ? 'secondary' : 'ghost'}
              size="sm"
              onClick={() => {
                if (!checked) onChange(option.value)
              }}
            >
              {option.label}
            </Button>
          )
        })}
      </div>
    </div>
  )
}
