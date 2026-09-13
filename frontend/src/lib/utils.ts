import { type ClassValue, clsx } from 'clsx'
import { extendTailwindMerge } from 'tailwind-merge'

/**
 * The design-system sizing roles mapped to `--container-*` in the `@theme`
 * block of `styles/index.css` (#921). tailwind-merge only recognises the
 * default container scale, so without these `cn('w-full', 'w-control-sm')`
 * keeps BOTH widths and stylesheet order picks the winner — `SelectTrigger`'s
 * own `w-full` would silently beat every filter control's token width.
 */
const DESIGN_SYSTEM_CONTAINERS = [
  'control-sm',
  'control-search-min',
  'control-search-max',
  'rail-collapsed',
  'rail-expanded',
  'details-column',
]

const twMerge = extendTailwindMerge({
  extend: { theme: { container: DESIGN_SYSTEM_CONTAINERS } },
})

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
