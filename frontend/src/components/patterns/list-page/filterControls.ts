/**
 * Shared shape constants for the resource list filter bars.
 *
 * The four bars used to hand-set their trigger widths — `w-[140px]` on prompts
 * and `w-[150px]` everywhere else — which is how a row of controls that should
 * read as one set ends up half a step out of line (#908). One constant instead,
 * so a change lands on every list at once. Turning it into a design-system
 * token is Phase 5 (#921).
 */
export const FILTER_CONTROL_WIDTH = 'w-[150px]'

/**
 * The value a `select`-style filter carries when it is not filtering.
 *
 * It is a real option value rather than an empty string because Radix's
 * `SelectItem` rejects `''`, and it matches the `'all'` default every list
 * page's `FILTER_DEFAULTS` already uses — so the parameter is omitted from the
 * URL entirely (see `useUrlFilters`).
 */
export const FILTER_ALL = 'all'
