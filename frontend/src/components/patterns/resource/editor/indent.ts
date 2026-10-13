/**
 * Line indentation for the Raw body editor: Tab and Shift+Tab move the
 * current line, or every line the selection touches, by one step.
 */

/** One indentation step. Matches the editor's `tab-size`. */
export const INDENT = '  '

/** A textarea's text and selection, before or after an edit. */
export interface TextSelection {
  value: string
  start: number
  end: number
}

/** The leading whitespace one outdent removes: a tab, or up to one step of spaces. */
function outdentWidth(line: string): number {
  if (line.startsWith('\t')) return 1
  let width = 0
  while (width < INDENT.length && line[width] === ' ') width += 1
  return width
}

/**
 * Indents (or, with `outdent`, outdents) the lines a selection touches.
 *
 * A collapsed caret counts as its own line. A selection that ends at the very
 * start of a line does not include that line — that is where the caret lands
 * after selecting whole lines, and moving the line after them would be a
 * surprise. Blank lines inside a multi-line selection are left blank rather
 * than given trailing whitespace.
 *
 * The selection is carried along with the text, so repeating the key keeps
 * working on the same lines.
 */
export function indentSelection(
  { value, start, end }: TextSelection,
  outdent = false
): TextSelection {
  const blockStart = value.lastIndexOf('\n', start - 1) + 1
  const lastChar = end > start && value[end - 1] === '\n' ? end - 1 : end
  const nextBreak = value.indexOf('\n', lastChar)
  const blockEnd = nextBreak === -1 ? value.length : nextBreak

  const lines = value.slice(blockStart, blockEnd).split('\n')
  const multiLine = lines.length > 1

  let firstDelta = 0
  let totalDelta = 0
  const changed = lines.map((line, index) => {
    let next = line
    if (outdent) {
      next = line.slice(outdentWidth(line))
    } else if (line !== '' || !multiLine) {
      next = INDENT + line
    }
    const delta = next.length - line.length
    if (index === 0) firstDelta = delta
    totalDelta += delta
    return next
  })

  if (totalDelta === 0) return { value, start, end }

  return {
    value:
      value.slice(0, blockStart) + changed.join('\n') + value.slice(blockEnd),
    // Never pull the caret back past the start of its own line.
    start: Math.max(blockStart, start + firstDelta),
    end: Math.max(blockStart, end + totalDelta),
  }
}
