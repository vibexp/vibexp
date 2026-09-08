/**
 * The one type treatment for a table's header cells (#909).
 *
 * The app had two: `ListTable`'s sentence-case `text-xs font-medium` head cell
 * and the version-history table's uppercase, letter-spaced 0.6875rem one. Both
 * now read this constant, so "the tables share a header style" is a fact about
 * a single source rather than two copies that happen to match.
 *
 * Type only — height, padding, sticky positioning and background stay with
 * each table, which lays its rows out differently.
 */
export const TABLE_HEAD_CLASS = 'text-muted-foreground text-xs font-medium'
