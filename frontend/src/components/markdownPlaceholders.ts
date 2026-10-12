import type { MarkedExtension, Tokens } from 'marked'

/**
 * Mirrors the backend's `placeholderRegex` (`internal/services/prompt.go`):
 * anything between `{{` and `}}` is a placeholder, and its key is the trimmed
 * inner text.
 */
const PLACEHOLDER_SOURCE = String.raw`\{\{([^}]+)\}\}`

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
  return text.replace(
    new RegExp(PLACEHOLDER_SOURCE, 'g'),
    (match: string, inner: string) => valueOf(values, inner.trim()) || match
  )
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
  const leading = new RegExp(`^${PLACEHOLDER_SOURCE}`)

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
          const match = leading.exec(src)
          if (!match) return undefined
          return { type: TOKEN_TYPE, raw: match[0], key: match[1].trim() }
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
