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
 * A forward search for `needle` in `text` that is asked for ever later
 * positions: `(from) => index of the first occurrence at or after from`, or
 * -1. The last answer is kept, so a caller scanning left to right pays for
 * each stretch of the text once however many times it asks.
 */
export function forwardFinder(
  text: string,
  needle: string
): (from: number) => number {
  let found = -2
  return from => {
    if (found === -1) return -1
    if (found < from) found = text.indexOf(needle, from)
    return found
  }
}
