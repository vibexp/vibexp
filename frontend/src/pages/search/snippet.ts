/**
 * Search excerpts are raw slices of a resource's markdown body, so they arrive
 * with their syntax attached — and, being slices, often with only half of a
 * pair (an opening fence, a lone `**`). These helpers turn one into plain text
 * for display and mark where the query matched.
 */

/** A run of display text; `match` is true where a query term matched. */
export interface SnippetSegment {
  text: string
  match: boolean
}

/**
 * Strips markdown syntax from an excerpt, keeping the words: heading and list
 * markers, emphasis, code fences and backticks, and link syntax (the link text
 * stays, the target goes). Whitespace collapses to single spaces, since an
 * excerpt is shown as one paragraph.
 */
export function stripMarkdown(markdown: string): string {
  return (
    markdown
      // Fence lines, with or without a language: the code itself stays.
      .replace(/^[ \t]*(?:```|~~~)[^\n]*$/gm, ' ')
      // Images and links keep their text.
      .replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1')
      .replace(/!?\[([^\]]*)\]\[[^\]]*\]/g, '$1')
      // Line-leading markers: headings, quotes, bullets, numbered items, rules.
      .replace(/^[ \t]*#{1,6}[ \t]+/gm, '')
      .replace(/^[ \t]*(?:>[ \t]?)+/gm, '')
      .replace(/^[ \t]*(?:[-*+]|\d+[.)])[ \t]+(?:\[[ xX]\][ \t]+)?/gm, '')
      .replace(/^[ \t]*[-*_][-*_ \t]{2,}$/gm, ' ')
      // Emphasis. Underscores only count at word edges, so snake_case survives.
      .replace(/(\*\*|__)(?=\S)(.+?)(?<=\S)\1/g, '$2')
      .replace(/~~(?=\S)(.+?)(?<=\S)~~/g, '$1')
      .replace(/\*(?=\S)([^*\n]+?)(?<=\S)\*/g, '$1')
      .replace(/(?<![\w])_(?=\S)([^_\n]+?)(?<=\S)_(?![\w])/g, '$1')
      // Whatever a slice left unpaired.
      .replace(/\*\*|~~|`+/g, '')
      // Table pipes and the divider row under a table header.
      .replace(/^(?=.*-{3})[ \t|:-]+$/gm, ' ')
      .replace(/[ \t]*\|[ \t]*/g, ' ')
      .replace(/\s+/g, ' ')
      .trim()
  )
}

const TERM_MIN_LENGTH = 2

/**
 * The words of a query worth highlighting: split on whitespace, stripped of
 * the quotes and operators search syntax adds, de-duplicated, and longest
 * first so a longer term wins over a shorter one it contains.
 */
export function queryTerms(query: string): string[] {
  const terms = new Set<string>()
  for (const raw of query.split(/\s+/)) {
    const term = raw.replace(/^[^\p{L}\p{N}]+|[^\p{L}\p{N}]+$/gu, '')
    if (term.length >= TERM_MIN_LENGTH) terms.add(term.toLowerCase())
  }
  return [...terms].sort((a, b) => b.length - a.length)
}

const escapeRegExp = (value: string): string =>
  value.replace(/[.*+?^${}()|[\]\\]/g, String.raw`\$&`)

/**
 * Splits `text` into segments, flagging each case-insensitive occurrence of a
 * query term. With nothing to highlight the text comes back as one segment.
 */
export function highlightSegments(
  text: string,
  query: string
): SnippetSegment[] {
  if (!text) return []
  const terms = queryTerms(query)
  if (terms.length === 0) return [{ text, match: false }]

  const pattern = new RegExp(`(${terms.map(escapeRegExp).join('|')})`, 'giu')
  const segments: SnippetSegment[] = []
  // A capturing split puts every match at an odd index.
  text.split(pattern).forEach((part, index) => {
    if (part) segments.push({ text: part, match: index % 2 === 1 })
  })
  return segments
}
