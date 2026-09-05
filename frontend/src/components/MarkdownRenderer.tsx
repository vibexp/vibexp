import DOMPurify from 'dompurify'
import { AlertTriangle } from 'lucide-react'
import { marked, Renderer } from 'marked'
import mermaid from 'mermaid'
import { useCallback, useEffect, useRef, useState } from 'react'

import { cn } from '@/lib/utils'
import Prism from '@/utils/prism-config'

/**
 * Escapes HTML special characters in a string to prevent attribute breakage
 * and XSS in rendered HTML. Applied to link href, title, and text values
 * before interpolation into raw HTML strings.
 */
const escHtml = (s: string): string =>
  s
    .replaceAll('&', '&amp;')
    .replaceAll('"', '&quot;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')

/**
 * Builds the copy-to-clipboard button markup injected into highlighted code
 * blocks. The code text is stored URI-encoded in a data attribute so a single
 * delegated click listener can recover it without DOM mutation.
 */
function buildCopyButtonHtml(codeContent: string, copyId: string): string {
  const encodedCode = encodeURIComponent(codeContent.replace(/<[^<>]*>/g, ''))
  return `<button class="copy-button absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity p-1 bg-muted-foreground/80 text-background rounded text-xs hover:bg-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring" title="Copy code" data-copy-id="${copyId}" data-code="${encodedCode}"><svg class="h-3 w-3" fill="currentColor" viewBox="0 0 20 20"><path d="M8 3a1 1 0 011-1h2a1 1 0 110 2H9a1 1 0 01-1-1z"></path><path d="M6 3a2 2 0 00-2 2v11a2 2 0 002 2h8a2 2 0 002-2V5a2 2 0 00-2-2h-1v1a1 1 0 01-1 1H7a1 1 0 01-1-1V3H6z"></path></svg></button>`
}

/**
 * Copies code text to the clipboard and toggles the button's "copied" state
 * for two seconds via the provided state setter.
 */
async function copyCodeToClipboard(
  codeText: string,
  copyId: string,
  setCopiedIds: React.Dispatch<React.SetStateAction<Set<string>>>
): Promise<void> {
  try {
    await navigator.clipboard.writeText(codeText)

    // Mark as copied using React state - no DOM mutation needed
    setCopiedIds(prev => {
      const next = new Set(prev)
      next.add(copyId)
      return next
    })

    setTimeout(() => {
      setCopiedIds(prev => {
        const next = new Set(prev)
        next.delete(copyId)
        return next
      })
    }, 2000)
  } catch (error) {
    console.error('Failed to copy code:', error)
  }
}

export interface MarkdownRendererProps {
  content: string
  className?: string
  syntaxTheme?: 'light' | 'dark' | 'auto'
  enableMermaid?: boolean
  enableCodeCopy?: boolean
  /**
   * Controls the `target` attribute on rendered links. Only `_blank` and
   * `_self` are supported — these are the two commonly-used targets in this
   * context. Other standard values (`_parent`, `_top`, named frames) are
   * intentionally out of scope.
   */
  linkTarget?: '_blank' | '_self'
  onError?: (error: Error, context: string) => void
}

// Removed standalone CopyButton component since we're implementing copy functionality inline

interface MermaidDiagramProps {
  code: string
  onError?: (error: Error) => void
}

const MermaidDiagram: React.FC<MermaidDiagramProps> = ({ code, onError }) => {
  const elementRef = useRef<HTMLDivElement>(null)
  const [error, setError] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    const renderDiagram = async () => {
      const element = elementRef.current
      if (!element) return

      try {
        setIsLoading(true)
        setError(null)

        // Initialize mermaid.
        //
        // securityLevel MUST stay 'strict' (not 'sandbox'): in 'sandbox' mode
        // mermaid.render() returns an `<iframe src="data:text/html;base64,…">`
        // wrapper rather than a raw `<svg>`. The SVG-only DOMPurify profile
        // applied below would strip that iframe entirely, blanking every
        // diagram. In 'strict' mode mermaid sanitizes internally and returns a
        // raw `<svg>` that our sanitize step preserves while still removing any
        // script / event-handler XSS vectors. Diagram interactivity is unused.
        //
        // For the same reason, diagram labels are kept in the SVG namespace
        // DELIBERATELY (`htmlLabels: false`, see below): the sanitize profile
        // below stays SVG-only and needs no widening.
        mermaid.initialize({
          startOnLoad: false,
          theme: 'default',
          securityLevel: 'strict',
          fontFamily: 'inherit',
          // Emit labels as native SVG `<text>` instead of HTML inside a
          // `<foreignObject>`, whose contents the SVG-only sanitize profile
          // below strips — which blanked every flowchart, class and state
          // diagram label (#744). This MUST be the top-level key: measured
          // against mermaid 11.16.1, the narrower `flowchart: { htmlLabels:
          // false }` fixes edge labels only and leaves node labels blank, and
          // does nothing for class/state diagrams. Cost is that `<br/>` and
          // inline markup in a label no longer render. Do not "clean this up".
          htmlLabels: false,
        })

        // Generate unique ID
        const randomStr = Math.random().toString(36).substring(2, 11)
        const id = `mermaid-${String(Date.now())}-${randomStr}`

        // Render diagram
        const { svg } = await mermaid.render(id, code)

        element.innerHTML = DOMPurify.sanitize(svg, {
          USE_PROFILES: { svg: true, svgFilters: true },
        })
      } catch (err) {
        const error = err as Error
        console.error('Mermaid rendering error:', error)
        setError(error.message)
        onError?.(error)
      } finally {
        setIsLoading(false)
      }
    }

    void renderDiagram()
  }, [code, onError])

  // The container MUST stay mounted in every state. It used to be returned only
  // in the success branch, which deadlocked the component: the render effect
  // resolves `elementRef` on its first run and bails out when it is null —
  // before the `finally` that clears `isLoading` — so the ref could never
  // become non-null and the diagram spun on "Rendering diagram…" forever
  // (#744). The loading and error views now render *alongside* it.
  return (
    <>
      {error !== null && (
        <div className="flex items-center p-4 bg-destructive/10 border border-destructive/30 rounded-lg">
          <AlertTriangle className="h-5 w-5 text-destructive mr-2 flex-shrink-0" />
          <div className="text-sm text-destructive">
            <strong>Mermaid Error:</strong> {error}
          </div>
        </div>
      )}

      {isLoading && (
        <div className="flex items-center justify-center p-8 bg-muted border border-border rounded-lg">
          <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-primary"></div>
          <span className="ml-2 text-sm text-muted-foreground">
            Rendering diagram...
          </span>
        </div>
      )}

      <div ref={elementRef} className="mermaid-container" />
    </>
  )
}

function syntaxThemeClass(theme: 'light' | 'dark' | 'auto'): string {
  if (theme === 'dark') return 'theme-dark'
  if (theme === 'light') return 'theme-light'
  return 'theme-auto'
}

export const MarkdownRenderer: React.FC<MarkdownRendererProps> = ({
  content,
  className = '',
  syntaxTheme = 'auto',
  enableMermaid = true,
  enableCodeCopy = true,
  linkTarget = '_blank',
  onError,
}) => {
  const [renderedContent, setRenderedContent] = useState('')
  const [mermaidDiagrams, setMermaidDiagrams] = useState<
    { id: string; code: string }[]
  >([])
  // Track which copy buttons are in "copied" state
  const [copiedIds, setCopiedIds] = useState<Set<string>>(new Set())
  const containerRef = useRef<HTMLDivElement>(null)

  // Use a stable ref for onError to prevent it from being a dependency that
  // causes re-render loops when callers pass inline arrow functions.
  const onErrorRef = useRef(onError)
  useEffect(() => {
    onErrorRef.current = onError
  }, [onError])

  // Guard against rapid re-render loops (VIBEXP-FRONTEND-JS-3)
  const renderCountRef = useRef(0)
  // Anchors the 200ms window below. Seeded lazily on the guard's first
  // evaluation rather than from `useRef(Date.now())`, which read the clock
  // during render (react-hooks/purity) on every render only to discard every
  // reading after the first. `null` means "no window open yet".
  const renderTimestampRef = useRef<number | null>(null)

  // Configure marked renderer
  const configureMarked = useCallback(() => {
    const renderer = new Renderer()
    renderer.link = ({ href, title, text }) => {
      const targetAttr = linkTarget === '_blank' ? ' target="_blank"' : ''
      const relAttr =
        linkTarget === '_blank'
          ? ' rel="noopener noreferrer"'
          : ' rel="noreferrer"'
      const titleAttr = title ? ` title="${escHtml(title)}"` : ''
      return `<a href="${escHtml(href)}"${titleAttr}${targetAttr}${relAttr}>${escHtml(text)}</a>`
    }
    return {
      breaks: true,
      gfm: true,
      renderer,
    }
  }, [linkTarget])

  // Render markdown content
  useEffect(() => {
    // Infinite loop guard: if more than 5 renders occur within 200ms, abort.
    const now = Date.now()
    // First evaluation opens the window here. The old render-phase
    // `useRef(Date.now())` opened it one commit earlier, so the threshold is
    // identical whenever the mount commit lands inside the 200ms window — which
    // it does in practice; only a mount that took longer than the window itself
    // would differ, and then by a single run.
    renderTimestampRef.current ??= now
    if (now - renderTimestampRef.current < 200) {
      renderCountRef.current += 1
      if (renderCountRef.current > 5) {
        console.error(
          'MarkdownRenderer: detected rapid re-render loop, aborting render'
        )
        return
      }
    } else {
      renderCountRef.current = 0
      renderTimestampRef.current = now
    }

    const renderMarkdown = async () => {
      try {
        setMermaidDiagrams([]) // Reset mermaid diagrams
        const options = configureMarked()

        // Extract mermaid diagrams from the RAW markdown, before marked runs.
        //
        // This must happen first: marked turns a ```mermaid fence into
        // `<pre><code class="language-mermaid">` with its body HTML-escaped
        // (`-->` becomes `--&gt;`), so a fence-matching regex applied to
        // marked's OUTPUT can never match. Running it there meant no diagram
        // ever rendered — every one silently degraded to a code block (#744).
        let source = content
        if (enableMermaid) {
          const mermaidRegex = /```(?:mermaid|mmd)\r?\n([\s\S]*?)```/g
          const diagrams: { id: string; code: string }[] = []

          source = source.replace(
            mermaidRegex,
            (_match: string, code: string) => {
              const randomStr = Math.random().toString(36).substring(2, 11)
              const id = `mermaid-${String(Date.now())}-${randomStr}`
              diagrams.push({ id, code: code.trim() })
              // Surrounding blank lines keep this a top-level HTML block, so
              // marked emits it verbatim instead of wrapping it in a <p>.
              return `\n\n<div data-mermaid-id="${id}"></div>\n\n`
            }
          )

          setMermaidDiagrams(diagrams)
        }

        let html = await marked(source, options)

        // Apply syntax highlighting and add copy buttons to code blocks in the HTML string
        let copyButtonCounter = 0
        html = html.replace(
          /<pre><code class="([^"]*)">([\s\S]*?)<\/code><\/pre>/g,
          (_match: string, className: string, codeContent: string) => {
            // Extract language from className (e.g., "language-javascript" -> "javascript")
            const match = /language-(\w+)/.exec(className)
            let highlightedCode = codeContent

            // Apply Prism syntax highlighting if language is supported
            if (match) {
              const lang = match[1]
              // Safely check if language grammar exists and get it
              if (Object.hasOwn(Prism.languages, lang)) {
                try {
                  // Decode HTML entities before highlighting
                  const tempDiv = document.createElement('div')
                  tempDiv.innerHTML = codeContent
                  const plainText = tempDiv.textContent ?? ''

                  // Apply Prism highlighting - lang is validated above
                  const languageGrammar =
                    Prism.languages[lang as keyof typeof Prism.languages]
                  highlightedCode = Prism.highlight(
                    plainText,
                    languageGrammar,
                    lang
                  )
                } catch (error) {
                  console.warn('Syntax highlighting failed:', error)
                  // Keep original codeContent if highlighting fails
                }
              }
            }

            // Build the final HTML with highlighting and optional copy button.
            // Use a stable data-copy-id to allow event delegation without mutating
            // the DOM (avoids the replaceChild conflict with React's virtual DOM).
            const copyButton = enableCodeCopy
              ? buildCopyButtonHtml(
                  codeContent,
                  `copy-${String(copyButtonCounter++)}`
                )
              : ''

            return `<pre class="relative group overflow-x-auto"><code class="${className}">${highlightedCode}</code>${copyButton}</pre>`
          }
        )

        // Wrap every table in a horizontally scrollable container (#884).
        // `width: 100%` on a `<table>` does not cap an auto-layout table — it
        // still grows to its min-content width, escaping the card and painting
        // over the metadata sidebar. The wrapper is added here rather than via
        // a CSS-only `display: block` on the table, which would drop table
        // layout semantics and the element's accessibility role.
        //
        // The opening tag is matched with a regex rather than a literal
        // `replaceAll('<table>')`: marked emits a bare `<table>` today, but a
        // literal match would silently no-op (leaving the bug back) if a future
        // marked ever emits attributes. GFM has no nested tables, so pairing
        // each opening tag with the next `</table>` needs no parser.
        html = html
          .replace(
            /<table(\s[^>]*)?>/g,
            (_match: string, attrs?: string) =>
              `<div class="markdown-table-wrapper"><table${attrs ?? ''}>`
          )
          .replaceAll('</table>', '</table></div>')

        setRenderedContent(
          DOMPurify.sanitize(html, { ADD_ATTR: ['target', 'rel'] })
        )
      } catch (error) {
        console.error('Error rendering markdown:', error)
        onErrorRef.current?.(error as Error, 'markdown-rendering')
        setRenderedContent(
          `<p>Error rendering markdown: ${(error as Error).message}</p>`
        )
      }
    }

    if (content) {
      void renderMarkdown()
    } else {
      setRenderedContent('')
      setMermaidDiagrams([])
    }
    // onError intentionally omitted from deps - we use onErrorRef to avoid
    // re-render loops when callers pass inline callbacks (VIBEXP-FRONTEND-JS-3).
  }, [content, configureMarked, enableMermaid, enableCodeCopy])

  // Attach copy button click listeners via event delegation on the container.
  // This avoids cloneNode/replaceChild which conflicts with React's virtual DOM
  // (VIBEXP-FRONTEND-JS-7). We use a single delegated listener instead of
  // attaching per-button listeners.
  useEffect(() => {
    if (!containerRef.current || !enableCodeCopy) return

    const container = containerRef.current

    const handleClick = (event: MouseEvent) => {
      const target = event.target as HTMLElement
      const button = target.closest<HTMLButtonElement>('.copy-button')
      if (!button) return

      const copyId = button.dataset.copyId
      const encodedCode = button.dataset.code
      if (!encodedCode || !copyId) return

      const codeText = decodeURIComponent(encodedCode)

      void copyCodeToClipboard(codeText, copyId, setCopiedIds)
    }

    container.addEventListener('click', handleClick)
    return () => {
      container.removeEventListener('click', handleClick)
    }
  }, [renderedContent, enableCodeCopy])

  // Update copy button icons based on copiedIds state (no DOM replacement).
  useEffect(() => {
    if (!containerRef.current || !enableCodeCopy) return

    const checkSvg =
      '<svg class="h-3 w-3" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"></path></svg>'
    const copySvg =
      '<svg class="h-3 w-3" fill="currentColor" viewBox="0 0 20 20"><path d="M8 3a1 1 0 011-1h2a1 1 0 110 2H9a1 1 0 01-1-1z"></path><path d="M6 3a2 2 0 00-2 2v11a2 2 0 002 2h8a2 2 0 002-2V5a2 2 0 00-2-2h-1v1a1 1 0 01-1 1H7a1 1 0 01-1-1V3H6z"></path></svg>'

    const buttons =
      containerRef.current.querySelectorAll<HTMLButtonElement>('.copy-button')
    buttons.forEach(button => {
      const copyId = button.dataset.copyId
      if (!copyId) return
      // Update innerHTML to reflect copied state - this is safe because we are
      // targeting elements inside dangerouslySetInnerHTML (not React-managed
      // component trees), and we are only mutating SVG icon content, not
      // structural nodes that React tracks.
      button.innerHTML = copiedIds.has(copyId) ? checkSvg : copySvg
    })
  }, [copiedIds, enableCodeCopy])

  // Render mermaid placeholders by appending React roots into placeholder divs.
  // We use appendChild (not replaceChild) to avoid the NotFoundError that occurs
  // when React's reconciler attempts to manage nodes we have already removed
  // (VIBEXP-FRONTEND-JS-7).
  useEffect(() => {
    if (!containerRef.current || mermaidDiagrams.length === 0) return

    // Capture diagrams and onErrorRef value for this effect run
    const diagrams = mermaidDiagrams
    const onErrorFn = onErrorRef.current
    const handleMermaidError = (error: Error) => {
      onErrorFn?.(error, 'mermaid-rendering')
    }

    void import('react-dom/client').then(({ createRoot }) => {
      if (!containerRef.current) return

      diagrams.forEach(({ id, code }) => {
        const placeholder = containerRef.current?.querySelector(
          `[data-mermaid-id="${id}"]`
        )
        if (!placeholder) return

        // Clear placeholder content and append a fresh child - no replaceChild
        placeholder.innerHTML = ''
        const mountPoint = document.createElement('div')
        placeholder.appendChild(mountPoint)

        const root = createRoot(mountPoint)
        root.render(<MermaidDiagram code={code} onError={handleMermaidError} />)
      })
    })
  }, [renderedContent, mermaidDiagrams])

  const themeClass = syntaxThemeClass(syntaxTheme)

  return (
    <div
      ref={containerRef}
      className={cn(
        'markdown-renderer prose max-w-none',
        themeClass,
        className
      )}
      dangerouslySetInnerHTML={{ __html: renderedContent }}
    />
  )
}

export default MarkdownRenderer
