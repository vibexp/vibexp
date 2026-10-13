/**
 * Small, pure transforms the markdown renderer applies around `marked`: one on
 * the SOURCE before parsing, one on the HTML after. Kept apart from the
 * component so they can be tested without the parser (which the unit suite
 * stubs).
 */

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
