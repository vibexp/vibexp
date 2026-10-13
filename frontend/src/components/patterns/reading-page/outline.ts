/**
 * The pure half of the reading page's outline: which headings make the list,
 * what their anchors are called, and which one the reader is in. The DOM half
 * (finding the headings, observing them) is `useReadingOutline`.
 */

export interface OutlineHeading {
  id: string
  /** 2, 3 or 4 — the heading's level in the body. */
  level: number
  text: string
}

/** Fewer headings than this and a document has no structure worth listing. */
export const OUTLINE_MIN_HEADINGS = 3

/** Below this many H2/H3 entries, H4s are let in to make the outline useful. */
const OUTLINE_H4_BELOW = 4

/**
 * A URL-safe, readable anchor for a heading: lowercase words joined by
 * hyphens. Unicode letters and digits are kept, so a non-English heading still
 * gets a meaningful anchor; everything else separates words.
 */
export function slugifyHeading(text: string): string {
  return text
    .trim()
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, '-')
    .replace(/^-+|-+$/g, '')
}

/**
 * Hands out one unique anchor per heading, in document order. A repeated
 * heading ("Example", "Example") gets `example`, `example-2`, … so every
 * anchor is stable for a given document and none is ambiguous. A heading with
 * nothing sluggable in it ("???") falls back to `section`.
 */
export function createSlugger(): (text: string) => string {
  const used = new Set<string>()
  return text => {
    const base = slugifyHeading(text) || 'section'
    let candidate = base
    // Keep counting until the anchor is free: a generated `example-2` can
    // collide with a heading literally titled "Example 2".
    for (let n = 2; used.has(candidate); n += 1) {
      candidate = `${base}-${String(n)}`
    }
    used.add(candidate)
    return candidate
  }
}

/**
 * The headings an outline lists: every H2 and H3, plus the H4s when there are
 * too few of those to be useful — and nothing at all for a document with fewer
 * than {@link OUTLINE_MIN_HEADINGS} headings, which needs no outline.
 */
export function buildOutline(
  headings: readonly OutlineHeading[]
): OutlineHeading[] {
  const main = headings.filter(h => h.level === 2 || h.level === 3)
  const entries =
    main.length < OUTLINE_H4_BELOW
      ? headings.filter(h => h.level >= 2 && h.level <= 4)
      : main
  return entries.length >= OUTLINE_MIN_HEADINGS ? entries : []
}

/**
 * The section the reader is in: the last heading that has scrolled up to (or
 * past) the reading line, `offset` pixels below the top of the viewport. Before
 * the first heading reaches it, the first heading is current — the reader is
 * in the document's opening section, not in none.
 *
 * `tops` are viewport-relative (`getBoundingClientRect().top`), in document
 * order.
 */
export function pickActiveHeading(
  tops: readonly { id: string; top: number }[],
  offset: number
): string | null {
  if (tops.length === 0) return null
  let active = tops[0].id
  for (const heading of tops) {
    if (heading.top <= offset) active = heading.id
    else break
  }
  return active
}
