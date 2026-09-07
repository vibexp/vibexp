/**
 * Memories are the one resource with no `title` field — `text` is both the name
 * and the body (#900). Until the API grows a real title (epic #899 decision D,
 * backend half), derive a readable one client-side so a memory is identifiable
 * in a tab, a heading and a search result instead of reading "Memory #<uuid>".
 */

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

/** Opens or closes a fenced code block (``` or ~~~), possibly with an info string. */
const CODE_FENCE = /^ {0,3}(`{3,}|~{3,})/

/**
 * The body's lines with fenced code blocks removed, so a shell comment like
 * `# install deps` inside a fence cannot masquerade as the memory's heading.
 */
function contentLines(text: string): string[] {
  const lines: string[] = []
  let openFence: { char: string; length: number } | null = null

  // Every line terminator CommonMark recognises, not just LF. The backend
  // stores a memory's text verbatim, so a Windows or classic-Mac MCP/CLI
  // writer lands `\r\n` or a lone `\r`; leaving one in keeps ATX_HEADING and
  // CODE_FENCE from matching (JS `.` excludes terminators, `^` is unanchored
  // without the `m` flag) — the whole body arrives as one line.
  for (const line of text.split(/\r\n|[\n\r\u2028\u2029]/)) {
    const fence = CODE_FENCE.exec(line)?.[1]
    if (fence !== undefined) {
      // CommonMark: only a run of the SAME character and at least as long as
      // the opening one closes the block. A different or shorter run inside an
      // open block is just content we are already skipping.
      if (openFence === null) {
        openFence = { char: fence.charAt(0), length: fence.length }
      } else if (
        fence.startsWith(openFence.char) &&
        fence.length >= openFence.length
      ) {
        openFence = null
      }
      continue
    }
    if (openFence === null) lines.push(line)
  }

  return lines
}

/** The first ATX heading's text, or `undefined` when the body has none. */
function firstHeading(lines: readonly string[]): string | undefined {
  for (const line of lines) {
    const heading = ATX_HEADING.exec(line)?.[1]
    if (heading !== undefined) return stripClosingHashes(heading).trim()
  }
  return undefined
}

/** The body as one whitespace-collapsed line, with markdown syntax stripped. */
function toPlainText(lines: readonly string[]): string {
  return lines
    .map(line =>
      line
        .replace(/^ {0,3}#{1,6}\s*/, '') // heading markers (an empty heading)
        .replace(/^ {0,3}(?:[*+-]|\d+[.)])\s+/, '') // list markers
        .replace(/^ {0,3}>\s?/, '') // block quotes
        // Emphasis / inline-code delimiters. `_` is deliberately absent: it is
        // far more often a snake_case identifier or a URL than markdown stress.
        .replace(/[*`~]/g, '')
    )
    .join(' ')
    .replace(/\s+/g, ' ')
    .trim()
}

/** Caps `value` at `MAX_EXCERPT_LENGTH`, breaking on the last whole word. */
function excerpt(value: string): string {
  if (value.length <= MAX_EXCERPT_LENGTH) return value
  const head = value.slice(0, MAX_EXCERPT_LENGTH)
  const lastSpace = head.lastIndexOf(' ')
  const cut = lastSpace > 0 ? head.slice(0, lastSpace) : head
  return `${cut.trimEnd()}…`
}

/**
 * A display title for a memory: its first markdown heading when it has one,
 * otherwise a word-boundary excerpt of the plain-text body, otherwise
 * `'Untitled memory'`.
 */
export function deriveMemoryTitle(text: string): string {
  const lines = contentLines(text)
  const heading = firstHeading(lines)
  if (heading !== undefined && heading !== '') return heading

  const plain = toPlainText(lines)
  return plain === '' ? FALLBACK_TITLE : excerpt(plain)
}
