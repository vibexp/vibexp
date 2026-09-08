/**
 * Memories are the one resource with no `title` field — `text` is both the name
 * and the body (#900). Until the API grows a real title (epic #899 decision D,
 * backend half), derive a readable one client-side so a memory is identifiable
 * in a tab, a heading and a search result instead of reading "Memory #<uuid>".
 *
 * The markdown stripping itself lives in `markdownExcerpt.ts` (#909), shared
 * with the memory list's Content cell; only the heading logic is here.
 */

import { contentLines, markdownToExcerpt } from '@/lib/markdownExcerpt'

/** Longest excerpt used when the body carries no heading. */
const MAX_EXCERPT_LENGTH = 60

/** Shown for an empty or whitespace-only memory. */
const FALLBACK_TITLE = 'Untitled memory'

/**
 * An ATX heading — up to 3 leading spaces, 1–6 `#`, then the text.
 *
 * Deliberately unanchored at the end. `\s` matches line terminators and `.`
 * does not, so `(.+)$` lets the engine retry every length of `\s+` against an
 * O(n) `.+` scan whenever a stray terminator survives the split — 8.5s on a
 * 200 KB body. Without `$` the greedy `.+` succeeds on its first attempt.
 */
const ATX_HEADING = /^ {0,3}#{1,6}\s+(.+)/

/**
 * Trims an ATX heading's optional closing `#` run.
 *
 * Deliberately a scan and not a regex. Both `(.+?)\s*#*\s*$` folded into
 * ATX_HEADING and a separate `/\s+#+\s*$/` are super-linear — the engine
 * re-tries every length of the whitespace/lazy run at every start position —
 * and memory bodies are unbounded, user- and MCP-writable text evaluated on
 * the render path, so a crafted memory would freeze the tab (~3.6s at 64k).
 */
function stripClosingHashes(heading: string): string {
  const trimmed = heading.trimEnd()
  let end = trimmed.length
  while (end > 0 && trimmed.charAt(end - 1) === '#') end -= 1
  if (end === trimmed.length) return trimmed // no closing run
  if (end === 0) return '' // the heading was nothing but hashes
  // CommonMark: a closing sequence only counts when whitespace precedes it,
  // so `# C#` keeps its hash while `# C #` does not.
  return /\s/.test(trimmed.charAt(end - 1))
    ? trimmed.slice(0, end).trimEnd()
    : trimmed
}

/** The first ATX heading's text, or `undefined` when the body has none. */
function firstHeading(lines: readonly string[]): string | undefined {
  for (const line of lines) {
    const heading = ATX_HEADING.exec(line)?.[1]
    if (heading !== undefined) return stripClosingHashes(heading).trim()
  }
  return undefined
}

/**
 * A display title for a memory: its first markdown heading when it has one,
 * otherwise a word-boundary excerpt of the plain-text body, otherwise
 * `'Untitled memory'`.
 */
export function deriveMemoryTitle(text: string): string {
  const heading = firstHeading(contentLines(text))
  if (heading !== undefined && heading !== '') return heading

  return markdownToExcerpt(text, MAX_EXCERPT_LENGTH) || FALLBACK_TITLE
}
