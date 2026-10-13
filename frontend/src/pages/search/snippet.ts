/**
 * Search excerpts are raw slices of a resource's markdown body, so they arrive
 * with their syntax attached — and, being slices, often with only half of a
 * pair (an opening fence, a lone `**`). These helpers turn one into plain text
 * for display and mark where the query matched.
 */

import { forwardFinder } from '@/utils/linearText'

/** A run of display text; `match` is true where a query term matched. */
export interface SnippetSegment {
  text: string
  match: boolean
}

/**
 * Replaces every link or image with its text: `[text](target)` when `open` is
 * `(`, the reference form `[text][ref]` when it is `[`. The text runs to the
 * first `]`, the target or reference to the first closing character after it.
 *
 * A scan rather than `!?\[([^\]]*)\]\([^)]*\)`: that pattern rescans to the
 * next `]` from every `[`, which is quadratic on a long run of brackets.
 */
function unwrapLinks(text: string, open: '(' | '[', close: ')' | ']'): string {
  const labelEnd = forwardFinder(text, ']')
  const targetEnd = forwardFinder(text, close)
  let out = ''
  let copiedTo = 0
  let from = 0
  for (;;) {
    const bracket = text.indexOf('[', from)
    if (bracket === -1) break
    const label = labelEnd(bracket + 1)
    // No `]` from here on, so nothing further can be a link.
    if (label === -1) break
    const end = text[label + 1] === open ? targetEnd(label + 2) : -1
    if (end === -1) {
      from = bracket + 1
      continue
    }
    // An image's `!` goes with it.
    const isImage = bracket > copiedTo && text[bracket - 1] === '!'
    const start = isImage ? bracket - 1 : bracket
    out += text.slice(copiedTo, start) + text.slice(bracket + 1, label)
    copiedTo = end + 1
    from = end + 1
  }
  return out + text.slice(copiedTo)
}

/** A table's divider row: only pipes, colons, dashes and blanks, with `---`. */
const DIVIDER_ROW = /^[ \t|:-]+$/
const LINE_BREAK = /([\n\r\u2028\u2029])/

/** Blanks the divider row under a table header, line by line. */
function blankDividerRows(text: string): string {
  return text
    .split(LINE_BREAK)
    .map(line => (line.includes('---') && DIVIDER_ROW.test(line) ? ' ' : line))
    .join('')
}

const WORD_CHARACTER = /[\p{L}\p{N}]/u

/** `raw` without the non-letter, non-digit characters at either end. */
function trimToWordCharacters(raw: string): string {
  const characters = Array.from(raw)
  let start = 0
  let end = characters.length
  while (start < end && !WORD_CHARACTER.test(characters[start])) start += 1
  while (end > start && !WORD_CHARACTER.test(characters[end - 1])) end -= 1
  return characters.slice(start, end).join('')
}

/**
 * Strips markdown syntax from an excerpt, keeping the words: heading and list
 * markers, emphasis, code fences and backticks, and link syntax (the link text
 * stays, the target goes). Whitespace collapses to single spaces, since an
 * excerpt is shown as one paragraph.
 */
export function stripMarkdown(markdown: string): string {
  // Fence lines, with or without a language: the code itself stays.
  const unfenced = markdown.replace(/^[ \t]*(?:```|~~~)[^\n]*$/gm, ' ')
  // Images and links keep their text.
  const unlinked = unwrapLinks(unwrapLinks(unfenced, '(', ')'), '[', ']')
  const unmarked = unlinked
    // Line-leading markers: headings, quotes, bullets, numbered items, rules.
    .replace(/^[ \t]*#{1,6}[ \t]+/gm, '')
    .replace(/^[ \t]*(?:>[ \t]?)+/gm, '')
    .replace(/^[ \t]*(?:[-*+]|\d+[.)])[ \t]+(?:\[[ xX]\][ \t]+)?/gm, '')
    .replace(/^[ \t]*[-*_][-*_ \t]{2,}$/gm, ' ')
    // Emphasis. Underscores only count at word edges, so snake_case survives.
    .replace(/(\*\*|__)(?=\S)(.+?)(?<=\S)\1/g, '$2')
    .replace(/~~(?=\S)(.+?)(?<=\S)~~/g, '$1')
    .replace(/\*(?=\S)([^*\n]+?)(?<=\S)\*/g, '$1')
    .replace(/(?<!\w)_(?=\S)([^_\n]+?)(?<=\S)_(?!\w)/g, '$1')
    // Whatever a slice left unpaired.
    .replace(/\*\*|~~|`+/g, '')
  // Table pipes and the divider row under a table header.
  return blankDividerRows(unmarked)
    .replace(/[ \t]*\|[ \t]*/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
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
    const term = trimToWordCharacters(raw)
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
