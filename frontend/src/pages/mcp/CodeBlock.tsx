import { Check, Copy, FileCode } from 'lucide-react'
import { useState } from 'react'

import { cn } from '@/lib/utils'

interface CodeBlockProps {
  code: string
  language: string
  /** File / location label shown in the header bar (e.g. "~/.cursor/mcp.json"). */
  file?: string
  onCopy: (code: string) => void
}

function escapeHtml(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
}

/**
 * Minimal token highlighter for the two snippet shapes we render (JSON config
 * and a single shell command). It escapes first, then wraps tokens in spans
 * bound to our tone tokens, so the output is safe for dangerouslySetInnerHTML.
 */
function highlight(code: string, language: string): string {
  let html = escapeHtml(code)
  if (language === 'json') {
    // Snippets are simple, machine-generated config — no escaped quotes inside
    // strings — so a plain quoted-run match is sufficient (and ReDoS-safe). A
    // quoted run followed by a colon is an object key (info), otherwise it is a
    // string value (success).
    html = html.replace(/"[^"]*"/g, (match, offset: number, full: string) => {
      const isKey = /^\s*:/.test(full.slice(offset + match.length))
      const tone = isKey ? 'text-info' : 'text-success'
      return `<span class="${tone}">${match}</span>`
    })
    html = html.replace(
      /([{}[\],])/g,
      '<span class="text-muted-foreground">$1</span>'
    )
  } else {
    html = html.replace(
      /(https?:\/\/[^\s\\]+)/g,
      '<span class="text-success">$1</span>'
    )
    html = html.replace(
      /(\s)(--?[a-zA-Z][\w-]*)/g,
      '$1<span class="text-warning">$2</span>'
    )
    html = html.replace(/^(\w[\w-]*)/, '<span class="text-info">$1</span>')
  }
  return html
}

export function CodeBlock({
  code,
  language,
  file,
  onCopy,
}: Readonly<CodeBlockProps>) {
  const [copied, setCopied] = useState(false)

  const handleCopy = () => {
    onCopy(code)
    setCopied(true)
    setTimeout(() => {
      setCopied(false)
    }, 1500)
  }

  return (
    <div className="bg-muted overflow-hidden rounded-lg border">
      {file && (
        <div className="bg-muted/60 flex items-center justify-between gap-2.5 border-b py-2 pl-3.5 pr-3">
          <span className="text-muted-foreground flex items-center gap-1.5 font-mono text-xs">
            <FileCode className="size-3.5" />
            {file}
          </span>
          <button
            type="button"
            onClick={handleCopy}
            className={cn(
              'bg-background hover:text-foreground inline-flex h-7 items-center gap-1.5 rounded-sm border px-2.5 font-sans text-xs font-medium transition-colors',
              copied
                ? 'text-success border-success/40'
                : 'text-muted-foreground'
            )}
          >
            {copied ? (
              <Check className="size-3.5" />
            ) : (
              <Copy className="size-3.5" />
            )}
            {copied ? 'Copied' : 'Copy'}
          </button>
        </div>
      )}
      <pre className="text-foreground overflow-x-auto px-[18px] py-4 font-mono text-sm leading-[1.65]">
        <code
          // Static, self-authored snippets — escaped before highlighting above.
          dangerouslySetInnerHTML={{ __html: highlight(code, language) }}
        />
      </pre>
    </div>
  )
}
