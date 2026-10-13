/**
 * The header metrics the reading page and the edit page share (#1179): View →
 * Edit turns the title and the description into inputs and moves nothing, so
 * both pages take their title type and their meta-row height from here rather
 * than each spelling them out.
 */

/** The page title — the reading page's `<h1>` and the edit page's title input. */
export const READING_TITLE_CLASS =
  'text-2xl font-semibold tracking-tight md:text-3xl'

/**
 * The row under the title: status, type, updated time. On a reading page the
 * toolbar shares it, so the row has the toolbar's height on BOTH pages —
 * otherwise everything beneath it would sit a few pixels lower on one of them.
 */
export const READING_META_ROW_CLASS = 'flex min-h-9 flex-wrap items-center'
