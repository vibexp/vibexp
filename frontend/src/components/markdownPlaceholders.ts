import type { MarkedExtension, Tokens } from 'marked'

import { forwardFinder } from '@/utils/linearText'

/**
 * Mirrors the backend's `placeholderRegex` (`internal/services/prompt.go`):
 * anything between `{{` and `}}` is a placeholder, and its key is the trimmed
 * inner text.
 */
const OPEN = '{{'

interface PlaceholderMatch {
  /** The whole placeholder as written, braces included. */
  raw: string
  /** What is between the braces, untrimmed. */
  inner: string
}

/**
 * The placeholder that begins exactly at `start`, if one does: `{{`, one or
 * more characters that are not `}`, then `}}`. `nextBrace(from)` is the first
 * `}` at or after `from`.
 *
 * A scan rather than `\{\{([^}]+)\}\}`, which rescans to the next `}` from
 * every `{{` in a body and so slows quadratically on a long run of braces.
 */
function placeholderAt(
  text: string,
  start: number,
  nextBrace: (from: number) => number
): PlaceholderMatch | null {
  if (!text.startsWith(OPEN, start)) return null
  const innerStart = start + OPEN.length
  const close = nextBrace(innerStart)
  if (close <= innerStart || text[close + 1] !== '}') return null
  return {
    raw: text.slice(start, close + 2),
    inner: text.slice(innerStart, close),
  }
}

const TOKEN_TYPE = 'promptPlaceholder'

interface PlaceholderToken extends Tokens.Generic {
  type: typeof TOKEN_TYPE
  raw: string
  key: string
}

const escHtml = (s: string): string =>
  s
    .replaceAll('&', '&amp;')
    .replaceAll('"', '&quot;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')

const valueOf = (values: Record<string, string>, key: string): string =>
  Object.hasOwn(values, key) ? values[key] : ''

/**
 * Substitutes the placeholders that HAVE a value and leaves the rest as
 * written. This is the treatment for text that is shown verbatim — code blocks,
 * inline code, link targets — where a chip cannot be drawn.
 */
export function substituteFilledPlaceholders(
  text: string,
  values: Record<string, string>
): string {
  const nextBrace = forwardFinder(text, '}')
  let out = ''
  let copiedTo = 0
  let from = 0
  for (;;) {
    const start = text.indexOf(OPEN, from)
    if (start === -1) break
    const found = placeholderAt(text, start, nextBrace)
    if (found === null) {
      from = start + 1
      continue
    }
    const end = start + found.raw.length
    out +=
      text.slice(copiedTo, start) +
      (valueOf(values, found.inner.trim()) || found.raw)
    copiedTo = end
    from = end
  }
  return out + text.slice(copiedTo)
}

/**
 * A marked extension that keeps prompt `{{placeholders}}` visible in rendered
 * markdown: one without a value renders as a monospace chip naming the
 * variable, one with a value renders as that value, lightly marked so
 * substituted text stays distinguishable from the prose around it.
 *
 * It is an INLINE extension, so it applies wherever marked parses inline
 * content (paragraphs, emphasis, list items, table cells, headings) and never
 * inside code, which marked does not parse inline. Code and link targets get
 * the plain {@link substituteFilledPlaceholders} treatment instead.
 *
 * A value is inserted as text, never parsed as markdown or HTML.
 */
export function promptPlaceholderExtension(
  values: Record<string, string>
): MarkedExtension {
  return {
    extensions: [
      {
        name: TOKEN_TYPE,
        level: 'inline',
        start: (src: string) => {
          const index = src.indexOf('{{')
          return index === -1 ? undefined : index
        },
        tokenizer: (src: string): PlaceholderToken | undefined => {
          const found = placeholderAt(src, 0, from => src.indexOf('}', from))
          if (!found) return undefined
          return { type: TOKEN_TYPE, raw: found.raw, key: found.inner.trim() }
        },
        renderer: token => {
          const { key } = token as PlaceholderToken
          const name = escHtml(key)
          const value = valueOf(values, key)
          if (value === '') {
            return `<span class="prompt-placeholder" data-placeholder="${name}" title="Placeholder without a value">{{${name}}}</span>`
          }
          return `<span class="prompt-placeholder prompt-placeholder-filled" data-placeholder="${name}" title="Value of {{${name}}}">${escHtml(value)}</span>`
        },
      },
    ],
    walkTokens: token => {
      if (token.type === 'code' || token.type === 'codespan') {
        const code = token as Tokens.Code | Tokens.Codespan
        code.text = substituteFilledPlaceholders(code.text, values)
      } else if (token.type === 'link') {
        const link = token as Tokens.Link
        link.href = substituteFilledPlaceholders(link.href, values)
        link.text = substituteFilledPlaceholders(link.text, values)
      }
    },
  }
}
