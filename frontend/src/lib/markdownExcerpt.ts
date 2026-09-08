/**
 * Plain-text excerpts of markdown bodies (#909).
 *
 * Resource bodies are markdown, but a list cell or a tab title is plain text —
 * rendering the raw source there shows `#` and `**` markers verbatim, exactly
 * on the resources that were written most carefully. This module strips the
 * syntax and caps the result on a word boundary.
 *
 * Extracted verbatim from `memoryTitle.ts` (#902), which still consumes it for
 * the derived memory title. **The comments below document deliberate,
 * non-obvious choices that a "simplification" would undo — they are load
 * bearing, not decoration:** these bodies are unbounded, user- and
 * MCP-writable text evaluated on the render path, so a regex that backtracks
 * freezes the tab.
 */

/** Opens or closes a fenced code block (``` or ~~~), possibly with an info string. */
const CODE_FENCE = /^ {0,3}(`{3,}|~{3,})/

/**
 * The body's lines with fenced code blocks removed, so a shell comment like
 * `# install deps` inside a fence cannot masquerade as the body's heading.
 */
export function contentLines(text: string): string[] {
  const lines: string[] = []
  let openFence: { char: string; length: number } | null = null

  // Every line terminator CommonMark recognises, not just LF. The backend
  // stores a memory's text verbatim, so a Windows or classic-Mac MCP/CLI
  // writer lands `\r\n` or a lone `\r`; leaving one in keeps the heading and
  // fence patterns from matching (JS `.` excludes terminators, `^` is
  // unanchored without the `m` flag) — the whole body arrives as one line.
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

/** Caps `value` at `maxLength`, breaking on the last whole word. */
function cut(value: string, maxLength: number): string {
  if (value.length <= maxLength) return value
  const head = value.slice(0, maxLength)
  const lastSpace = head.lastIndexOf(' ')
  const tail = lastSpace > 0 ? head.slice(0, lastSpace) : head
  return `${tail.trimEnd()}…`
}

/**
 * `text` as a plain-text excerpt of at most `maxLength` characters plus an
 * ellipsis, with markdown syntax and fenced code blocks removed.
 *
 * Returns `''` for an empty or whitespace-only body — never a bare `…` — so a
 * caller can render an empty cell rather than a stray character.
 */
export function markdownToExcerpt(text: string, maxLength: number): string {
  return cut(toPlainText(contentLines(text)), maxLength)
}
