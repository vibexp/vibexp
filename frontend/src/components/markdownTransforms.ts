/**
 * Small, pure transforms the markdown renderer applies around `marked`: one on
 * the SOURCE before parsing, the others on the HTML after. Kept apart from the
 * component so they can be tested without the parser (which the unit suite
 * stubs).
 */

import { createSlugger } from '@/components/patterns/reading-page/outline'
import { forwardFinder, removeSpans } from '@/utils/linearText'

const normalize = (text: string): string =>
  text.trim().replace(/\s+/g, ' ').toLowerCase()

/** Inline markers an author may wrap a heading's words in: `# **Title**`. */
const stripInlineMarkers = (text: string): string => text.replace(/[*_`]/g, '')

/**
 * Removes a leading `# Heading` that only repeats the resource's title.
 *
 * Agents usually open a body with `# Title`, and the page header already shows
 * the title, so the reader would see it twice in a row. Only the rendered view
 * drops it: the stored content, the Raw view and "Copy content" are untouched,
 * because this runs on the string handed to the renderer and nowhere else.
 *
 * Deliberately narrow, so no real content is ever lost: it must be the FIRST
 * block of the body, a level-1 heading (ATX `# x` or setext `x` / `===`), and
 * equal to the title ignoring case, surrounding whitespace and inline markers.
 * A different H1, or the same words further down, stays.
 */
export function stripLeadingTitleHeading(
  source: string,
  title: string | undefined
): string {
  const wanted = normalize(title ?? '')
  if (wanted === '') return source

  const lines = source.split(/\r\n|[\n\r]/)
  let first = 0
  while (first < lines.length && lines[first].trim() === '') first += 1
  if (first >= lines.length) return source

  const matches = (heading: string) =>
    normalize(stripInlineMarkers(heading)) === wanted

  // ATX: up to 3 spaces, exactly one `#`, whitespace, the text, and an
  // optional closing run of hashes.
  const line = lines[first]
  const afterIndent = line.replace(/^ {0,3}/, '')
  if (afterIndent.startsWith('# ') || afterIndent.startsWith('#\t')) {
    const text = afterIndent.slice(1).replace(/\s#+\s*$/, '')
    return matches(text) ? lines.slice(first + 1).join('\n') : source
  }

  // Setext: the text on one line, a run of `=` on the next.
  const underline = lines[first + 1] ?? ''
  if (/^ {0,3}=+\s*$/.test(underline) && matches(line)) {
    return lines.slice(first + 2).join('\n')
  }

  return source
}

/**
 * Turns GFM task-list items into something that can be styled as one.
 *
 * `marked` emits `<li><input disabled type="checkbox"> text</li>` — an item
 * that still carries its list bullet, with a native disabled checkbox whose
 * checked state is a faint grey in every browser. The list item gains a
 * `task-list-item` class (no bullet), and the checkbox becomes a `<span>` the
 * stylesheet draws from the app's tokens, so checked and unchecked are clearly
 * different in both themes. It is read-only by construction: there is no input
 * left to toggle. The state is announced through its label.
 */
export function markTaskListItems(html: string): string {
  return html.replace(
    /<li>(\s*<p>)?\s*<input\b([^>]*)>/g,
    (match: string, paragraph: string | undefined, attributes: string) => {
      if (!attributes.includes('type="checkbox"')) return match
      const checked = /\bchecked\b/.test(attributes)
      const box = `<span class="task-checkbox" role="img" data-checked="${String(checked)}" aria-label="${checked ? 'Completed' : 'Not completed'}"></span>`
      return `<li class="task-list-item">${paragraph ?? ''}${box}`
    }
  )
}

const decodeEntities = (text: string): string =>
  text
    .replaceAll('&lt;', '<')
    .replaceAll('&gt;', '>')
    .replaceAll('&quot;', '"')
    .replaceAll('&#39;', "'")
    .replaceAll('&amp;', '&')

/**
 * Gives every heading a stable `id` derived from its text, so a section can be
 * linked to (`#rollout`) and the reading page's outline has something to jump
 * to. Ids are unique within the document: a repeated heading gets `-2`, `-3`.
 * A heading that already carries an id (raw HTML in the body) keeps it.
 */
export function addHeadingIds(html: string): string {
  const slug = createSlugger()
  // A scan, not `<h([1-6])>([\s\S]*?)<\/h\1>`: that pattern reads to the end
  // of the document from every opening tag that is never closed. Here each
  // level looks for its closing tag with a search that only moves forward.
  const opening = /<h([1-6])>/g
  const closers = new Map<string, (from: number) => number>()
  const closerFor = (level: string) => {
    let find = closers.get(level)
    if (!find) {
      find = forwardFinder(html, `</h${level}>`)
      closers.set(level, find)
    }
    return find
  }

  let out = ''
  let copiedTo = 0
  for (;;) {
    const open = opening.exec(html)
    if (open === null) break
    const level = open[1]
    const innerStart = open.index + open[0].length
    const close = closerFor(level)(innerStart)
    if (close === -1) {
      // Never closed: not a heading. Look again from the next character.
      opening.lastIndex = open.index + 1
      continue
    }
    const inner = html.slice(innerStart, close)
    const text = decodeEntities(removeSpans(inner, '<', '>'))
    const end = close + `</h${level}>`.length
    out += `${html.slice(copiedTo, open.index)}<h${level} id="${slug(text)}">${inner}</h${level}>`
    copiedTo = end
    opening.lastIndex = end
  }
  return out + html.slice(copiedTo)
}

const CODE_BLOCK_OPEN = '<pre><code'
const CODE_BLOCK_CLASS = ' class="'
const CODE_BLOCK_CLOSE = '</code></pre>'

/**
 * Replaces every `<pre><code>…</code></pre>` block marked emitted with what
 * `replace` returns for it. `className` is the `class` attribute's value when
 * the opening tag carries exactly that one attribute (a fence with a language),
 * and `undefined` for a bare `<pre><code>`; `code` is the escaped body, up to
 * the first closing pair. Anything else — another attribute, a block that is
 * never closed — is left as written.
 *
 * A scan, not `<pre><code(?: class="([^"]*)")?>([\s\S]*?)<\/code><\/pre>`:
 * that pattern reads to the end of the document from every opening that is
 * never closed.
 */
export function replaceCodeBlocks(
  html: string,
  replace: (className: string | undefined, code: string) => string
): string {
  const nextQuote = forwardFinder(html, '"')
  const nextClose = forwardFinder(html, CODE_BLOCK_CLOSE)
  let out = ''
  let copiedTo = 0
  let from = 0
  for (;;) {
    const open = html.indexOf(CODE_BLOCK_OPEN, from)
    if (open === -1) break
    let cursor = open + CODE_BLOCK_OPEN.length
    let className: string | undefined
    if (html.startsWith(CODE_BLOCK_CLASS, cursor)) {
      const valueStart = cursor + CODE_BLOCK_CLASS.length
      const quote = nextQuote(valueStart)
      // No closing quote from here on: no later block can have a class.
      if (quote !== -1) {
        className = html.slice(valueStart, quote)
        cursor = quote + 1
      }
    }
    const close = html[cursor] === '>' ? nextClose(cursor + 1) : -1
    if (close === -1) {
      from = open + 1
      continue
    }
    out +=
      html.slice(copiedTo, open) +
      replace(className, html.slice(cursor + 1, close))
    copiedTo = close + CODE_BLOCK_CLOSE.length
    from = copiedTo
  }
  return out + html.slice(copiedTo)
}

const FENCE = '```'
const MERMAID_INFO_STRINGS = ['mermaid', 'mmd'] as const

/**
 * Replaces every ```` ```mermaid ```` (or ```` ```mmd ````) fence in raw
 * markdown with what `replace` returns for its body. The info string must be
 * followed directly by a line break (LF or CRLF); the body runs to the next
 * three backticks. A fence that is never closed is left as written.
 *
 * A scan, not ```` ```(?:mermaid|mmd)\r?\n([\s\S]*?)``` ````: that pattern
 * reads to the end of the document from every such opening that never closes.
 */
export function replaceMermaidFences(
  source: string,
  replace: (code: string) => string
): string {
  const nextClose = forwardFinder(source, FENCE)
  let out = ''
  let copiedTo = 0
  let from = 0
  for (;;) {
    const open = source.indexOf(FENCE, from)
    if (open === -1) break
    const infoStart = open + FENCE.length
    const info = MERMAID_INFO_STRINGS.find(name =>
      source.startsWith(name, infoStart)
    )
    let bodyStart = -1
    if (info !== undefined) {
      let cursor = infoStart + info.length
      if (source[cursor] === '\r') cursor += 1
      if (source[cursor] === '\n') bodyStart = cursor + 1
    }
    const close = bodyStart === -1 ? -1 : nextClose(bodyStart)
    if (close === -1) {
      from = open + 1
      continue
    }
    out +=
      source.slice(copiedTo, open) + replace(source.slice(bodyStart, close))
    copiedTo = close + FENCE.length
    from = copiedTo
  }
  return out + source.slice(copiedTo)
}
