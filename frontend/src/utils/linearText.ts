/**
 * Small text scans that run in time linear in the input, for places where the
 * obvious regular expression backtracks (`-+$` retries from every `-` in the
 * string; `\[[^\]]*\]` rescans to the end from every `[`). They run on text
 * people wrote, so a long pathological body must not stall the page.
 */

/** `text` without the run of `char` at its end. */
export function trimEndChar(text: string, char: string): string {
  let end = text.length
  while (end > 0 && text[end - 1] === char) end -= 1
  return end === text.length ? text : text.slice(0, end)
}

/** `text` without the runs of `char` at its start and its end. */
export function trimChar(text: string, char: string): string {
  const trimmed = trimEndChar(text, char)
  let start = 0
  while (start < trimmed.length && trimmed[start] === char) start += 1
  return start === 0 ? trimmed : trimmed.slice(start)
}

/**
 * Wraps a search — `(from) => index of the first hit at or after from`, or -1
 * — for a caller that asks for ever later positions. The last answer is kept,
 * so a left-to-right scan pays for each stretch of the text once however many
 * times it asks.
 */
export function forwardSearch(
  search: (from: number) => number
): (from: number) => number {
  let found = -2
  return from => {
    if (found === -1) return -1
    if (found < from) found = search(from)
    return found
  }
}

/** {@link forwardSearch} for the next occurrence of `needle` in `text`. */
export function forwardFinder(
  text: string,
  needle: string
): (from: number) => number {
  return forwardSearch(from => text.indexOf(needle, from))
}

/**
 * `text` without every span that runs from an `open` character to the first
 * `close` character after it, both included: `removeSpans(html, '<', '>')`
 * drops the tags. An `open` with no `close` after it is left as written.
 *
 * The scan for `<[^>]*>`, which rescans to the next `>` from every `<`.
 */
export function removeSpans(text: string, open: string, close: string): string {
  const nextClose = forwardFinder(text, close)
  let out = ''
  let copiedTo = 0
  for (;;) {
    const start = text.indexOf(open, copiedTo)
    if (start === -1) break
    const end = nextClose(start + 1)
    if (end === -1) break
    out += text.slice(copiedTo, start)
    copiedTo = end + 1
  }
  return out + text.slice(copiedTo)
}
