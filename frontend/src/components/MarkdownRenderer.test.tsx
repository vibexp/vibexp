import '@testing-library/jest-dom'

import { render, screen, waitFor } from '@testing-library/react'
import DOMPurify from 'dompurify'
import { marked } from 'marked'

import { MarkdownRenderer } from './MarkdownRenderer'

// Mock all external dependencies
// `MarkdownRenderer` does `import mermaid from 'mermaid'`, so the mock must
// expose a DEFAULT export. Without one, Vitest throws on first property access
// ('No "default" export is defined on the "mermaid" mock') — which stayed
// invisible while no test ever mounted a diagram.
vi.mock('mermaid', () => {
  const mermaid = {
    initialize: vi.fn(),
    render: vi
      .fn()
      .mockResolvedValue({ svg: '<svg>mock mermaid diagram</svg>' }),
  }
  return { ...mermaid, default: mermaid }
})

// Capture the link override installed by configureMarked so tests can
// invoke it directly and assert on its output.
interface MockLinkToken {
  href: string
  title?: string | null
  text: string
  tokens: unknown[]
}
let capturedLinkFn: ((token: MockLinkToken) => string) | undefined

vi.mock('marked', () => {
  // A minimal Renderer stand-in. The component sets `renderer.link` after
  // construction, so we intercept that assignment via a property descriptor.
  class MockRenderer {
    // Intercept the link property assignment to capture the override for tests
    get link(): ((token: MockLinkToken) => string) | undefined {
      return capturedLinkFn
    }

    set link(fn: ((token: MockLinkToken) => string) | undefined) {
      capturedLinkFn = fn
    }
  }

  return {
    marked: vi.fn().mockResolvedValue('<p>mocked content</p>'),
    Renderer: MockRenderer,
  }
})

vi.mock('prismjs', () => ({
  default: {
    highlight: vi.fn().mockReturnValue('highlighted code'),
    languages: {},
  },
  highlight: vi.fn().mockReturnValue('highlighted code'),
  languages: {
    javascript: {},
    typescript: {},
    python: {},
    bash: {},
    json: {},
    yaml: {},
    sql: {},
    go: {},
    java: {},
    css: {},
    scss: {},
    markdown: {},
  },
}))

// Mock clipboard API
Object.assign(navigator, {
  clipboard: {
    writeText: vi.fn().mockResolvedValue(undefined),
  },
})

const mockMarked = vi.mocked(marked)

// Mock all CSS imports
vi.mock('prismjs/themes/prism-okaidia.css', () => ({}))
vi.mock('prismjs/components/prism-javascript', () => ({}))
vi.mock('prismjs/components/prism-typescript', () => ({}))
vi.mock('prismjs/components/prism-jsx', () => ({}))
vi.mock('prismjs/components/prism-tsx', () => ({}))
vi.mock('prismjs/components/prism-python', () => ({}))
vi.mock('prismjs/components/prism-bash', () => ({}))
vi.mock('prismjs/components/prism-json', () => ({}))
vi.mock('prismjs/components/prism-yaml', () => ({}))
vi.mock('prismjs/components/prism-sql', () => ({}))
vi.mock('prismjs/components/prism-go', () => ({}))
vi.mock('prismjs/components/prism-java', () => ({}))
vi.mock('prismjs/components/prism-css', () => ({}))
vi.mock('prismjs/components/prism-scss', () => ({}))
vi.mock('prismjs/components/prism-markdown', () => ({}))

describe('MarkdownRenderer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('Component Rendering', () => {
    it('renders without crashing', async () => {
      render(<MarkdownRenderer content="# Test" />)

      await waitFor(() => {
        expect(document.querySelector('.markdown-renderer')).toBeInTheDocument()
      })
    })

    it('renders mocked content from marked', async () => {
      mockMarked.mockResolvedValue('<p>Test Content</p>')

      render(<MarkdownRenderer content="# Test" />)

      await waitFor(() => {
        expect(screen.getByText('Test Content')).toBeInTheDocument()
      })
    })

    it('applies custom className', async () => {
      render(<MarkdownRenderer content="# Test" className="custom-class" />)

      const container = document.querySelector('.markdown-renderer')
      await waitFor(() => {
        expect(container).toHaveClass('custom-class')
      })
    })

    it('applies correct theme class based on syntaxTheme prop', async () => {
      const { rerender } = render(
        <MarkdownRenderer content="# Test" syntaxTheme="light" />
      )

      const container = document.querySelector('.markdown-renderer')
      await waitFor(() => {
        expect(container).toHaveClass('theme-light')
      })

      rerender(<MarkdownRenderer content="# Test" syntaxTheme="dark" />)
      await waitFor(() => {
        expect(container).toHaveClass('theme-dark')
      })

      rerender(<MarkdownRenderer content="# Test" syntaxTheme="auto" />)
      await waitFor(() => {
        expect(container).toHaveClass('theme-auto')
      })
    })

    it('handles empty content gracefully', async () => {
      render(<MarkdownRenderer content="" />)

      const container = document.querySelector('.markdown-renderer')
      await waitFor(() => {
        expect(container).toBeInTheDocument()
      })
    })

    it('updates content when content prop changes', async () => {
      mockMarked.mockResolvedValue('<p>First Content</p>')

      const { rerender } = render(<MarkdownRenderer content="# First" />)

      await waitFor(() => {
        expect(screen.getByText('First Content')).toBeInTheDocument()
      })

      mockMarked.mockResolvedValue('<p>Second Content</p>')

      rerender(<MarkdownRenderer content="# Second" />)

      await waitFor(() => {
        expect(screen.getByText('Second Content')).toBeInTheDocument()
      })
    })
  })

  describe('Configuration', () => {
    it('calls marked with correct options including a renderer', async () => {
      mockMarked.mockResolvedValue('<p>test</p>')

      render(<MarkdownRenderer content="# Test" />)

      await waitFor(() => {
        expect(marked).toHaveBeenCalledWith(
          '# Test',
          expect.objectContaining({
            breaks: true,
            gfm: true,
            renderer: expect.any(Object),
          })
        )
      })
    })
  })

  describe('Link Target', () => {
    it('renders links with target="_blank" and rel="noopener noreferrer" by default', async () => {
      mockMarked.mockResolvedValue('<p>test</p>')

      render(<MarkdownRenderer content="[link](https://example.com)" />)

      await waitFor(() => {
        expect(marked).toHaveBeenCalled()
      })

      // The renderer's link method should produce target="_blank" and rel="noopener noreferrer"
      expect(capturedLinkFn).toBeDefined()
      const result = capturedLinkFn!({
        href: 'https://example.com',
        title: null,
        text: 'link',
        tokens: [],
      })
      expect(result).toContain('target="_blank"')
      expect(result).toContain('rel="noopener noreferrer"')
      expect(result).toContain('href="https://example.com"')
    })

    it('renders links with target="_blank" when linkTarget="_blank" is explicitly set', async () => {
      mockMarked.mockResolvedValue('<p>test</p>')

      render(
        <MarkdownRenderer
          content="[link](https://example.com)"
          linkTarget="_blank"
        />
      )

      await waitFor(() => {
        expect(marked).toHaveBeenCalled()
      })

      expect(capturedLinkFn).toBeDefined()
      const result = capturedLinkFn!({
        href: 'https://example.com',
        title: null,
        text: 'link',
        tokens: [],
      })
      expect(result).toContain('target="_blank"')
      expect(result).toContain('rel="noopener noreferrer"')
    })

    it('renders links without target="_blank" but with rel="noreferrer" when linkTarget="_self"', async () => {
      mockMarked.mockResolvedValue('<p>test</p>')

      render(
        <MarkdownRenderer
          content="[link](https://example.com)"
          linkTarget="_self"
        />
      )

      await waitFor(() => {
        expect(marked).toHaveBeenCalled()
      })

      expect(capturedLinkFn).toBeDefined()
      const result = capturedLinkFn!({
        href: 'https://example.com',
        title: null,
        text: 'link',
        tokens: [],
      })
      expect(result).not.toContain('target="_blank"')
      expect(result).not.toContain('rel="noopener noreferrer"')
      // _self links must still carry noreferrer to avoid leaking referrer info
      expect(result).toContain('rel="noreferrer"')
      expect(result).toContain('href="https://example.com"')
    })

    it('includes title attribute in rendered link when title is provided', async () => {
      mockMarked.mockResolvedValue('<p>test</p>')

      render(<MarkdownRenderer content="[link](https://example.com)" />)

      await waitFor(() => {
        expect(marked).toHaveBeenCalled()
      })

      expect(capturedLinkFn).toBeDefined()
      const result = capturedLinkFn!({
        href: 'https://example.com',
        title: 'My Title',
        text: 'link',
        tokens: [],
      })
      expect(result).toContain('title="My Title"')
      expect(result).toContain('target="_blank"')
      expect(result).toContain('rel="noopener noreferrer"')
    })

    it('passes a renderer instance with a link override to marked options', async () => {
      capturedLinkFn = undefined
      mockMarked.mockResolvedValue('<p>test</p>')

      render(<MarkdownRenderer content="# Test" />)

      await waitFor(() => {
        // Verify the renderer's link property was overridden
        expect(capturedLinkFn).toBeDefined()
        expect(typeof capturedLinkFn).toBe('function')
      })
    })
  })

  describe('Link HTML escaping (C1, C2, H1)', () => {
    // These tests invoke capturedLinkFn directly with special characters to
    // verify that the link renderer applies HTML escaping before interpolation.
    // They complement the mocked-marked tests above by validating the exact
    // HTML output for security-relevant inputs.

    beforeEach(async () => {
      mockMarked.mockResolvedValue('<p>test</p>')
      render(<MarkdownRenderer content="[link](https://example.com)" />)
      await waitFor(() => {
        expect(marked).toHaveBeenCalled()
      })
      expect(capturedLinkFn).toBeDefined()
    })

    it('escapes & in link text so it renders as &amp; in HTML output (C2)', () => {
      const result = capturedLinkFn!({
        href: 'https://example.com',
        title: null,
        text: 'Save & Continue',
        tokens: [],
      })
      // Raw & must be escaped to prevent incorrect HTML rendering
      expect(result).toContain('&amp;')
      expect(result).not.toMatch(/>Save & Continue</)
      expect(result).toContain('>Save &amp; Continue<')
    })

    it('escapes < and > in link text (C2)', () => {
      const result = capturedLinkFn!({
        href: 'https://example.com',
        title: null,
        text: '<script>alert(1)</script>',
        tokens: [],
      })
      expect(result).toContain('&lt;script&gt;')
      expect(result).not.toContain('<script>')
    })

    it('escapes " in title attribute so it does not break the HTML attribute (C1)', () => {
      const result = capturedLinkFn!({
        href: 'https://example.com',
        title: 'Say "hello"',
        text: 'link',
        tokens: [],
      })
      // A raw " would break the attribute boundary; must be &quot;
      expect(result).toContain('title="Say &quot;hello&quot;"')
      // Ensure the raw double-quote is not present inside the attribute value
      expect(result).not.toMatch(/title="Say "hello"/)
    })

    it('escapes & in title attribute (C1)', () => {
      const result = capturedLinkFn!({
        href: 'https://example.com',
        title: 'Terms & Conditions',
        text: 'link',
        tokens: [],
      })
      expect(result).toContain('title="Terms &amp; Conditions"')
    })

    it('escapes & in href (H1)', () => {
      const result = capturedLinkFn!({
        href: 'https://example.com/search?a=1&b=2',
        title: null,
        text: 'search',
        tokens: [],
      })
      expect(result).toContain('href="https://example.com/search?a=1&amp;b=2"')
    })

    it('escapes " in href to prevent attribute breakage (H1)', () => {
      const result = capturedLinkFn!({
        href: 'https://example.com/path"with"quotes',
        title: null,
        text: 'link',
        tokens: [],
      })
      expect(result).toContain(
        'href="https://example.com/path&quot;with&quot;quotes"'
      )
    })

    it('still includes target="_blank" and rel="noopener noreferrer" with escaped content', () => {
      const result = capturedLinkFn!({
        href: 'https://example.com',
        title: 'My & Title',
        text: 'link & text',
        tokens: [],
      })
      expect(result).toContain('target="_blank"')
      expect(result).toContain('rel="noopener noreferrer"')
      expect(result).toContain('title="My &amp; Title"')
      expect(result).toContain('>link &amp; text<')
    })
  })

  describe('Error Handling', () => {
    it('calls onError callback when markdown rendering fails', async () => {
      const mockOnError = vi.fn()

      mockMarked.mockRejectedValue(new Error('Markdown error'))

      render(<MarkdownRenderer content="# Test" onError={mockOnError} />)

      await waitFor(() => {
        expect(mockOnError).toHaveBeenCalledWith(
          expect.any(Error),
          'markdown-rendering'
        )
      })
    })

    it('renders error message when markdown rendering fails', async () => {
      mockMarked.mockRejectedValue(new Error('Test error'))

      render(<MarkdownRenderer content="# Test" />)

      await waitFor(() => {
        expect(
          screen.getByText(/Error rendering markdown: Test error/)
        ).toBeInTheDocument()
      })
    })
  })

  describe('Props Validation', () => {
    it('accepts all expected props', async () => {
      render(
        <MarkdownRenderer
          content="# Test"
          className="test-class"
          syntaxTheme="dark"
          enableMermaid={true}
          enableCodeCopy={true}
          linkTarget="_blank"
          onError={() => {}}
        />
      )

      const renderer = document.querySelector('.markdown-renderer')
      await waitFor(() => {
        expect(renderer).toBeInTheDocument()
      })
      expect(renderer).toHaveClass('test-class', 'theme-dark')
    })

    it('works with minimal props', async () => {
      render(<MarkdownRenderer content="# Test" />)

      await waitFor(() => {
        expect(document.querySelector('.markdown-renderer')).toBeInTheDocument()
      })
    })
  })

  // Regression guard for the extraction ORDER (#744). Mermaid blocks must be
  // pulled out of the RAW markdown, before marked runs: marked escapes a fence
  // body (`-->` -> `--&gt;`) and wraps it in `<pre><code class="language-
  // mermaid">`, so a fence regex applied to marked's output matches nothing and
  // every diagram silently degrades to a code block. The mermaid module itself
  // is stubbed here, so this asserts what CAN be asserted at this layer — the
  // handoff — while real rendering is covered by
  // `e2e/features/artifacts/mermaid-diagram.spec.ts`.
  describe('mermaid extraction runs before marked', () => {
    const FENCE = '```mermaid\nflowchart TD\n  A[Start] --> B[Stop]\n```'

    it('hands marked a placeholder instead of the fence', async () => {
      const markedMock = vi.mocked(marked)
      markedMock.mockClear()

      render(<MarkdownRenderer content={`# Title\n\n${FENCE}\n`} />)

      await waitFor(() => {
        expect(markedMock).toHaveBeenCalled()
      })

      const source = markedMock.mock.calls[0]?.[0]
      expect(source).toMatch(/<div data-mermaid-id="mermaid-[^"]+"><\/div>/)
      // The fence itself must be gone, or marked would render it as code.
      expect(source).not.toContain('```mermaid')
      expect(source).toContain('# Title')
    })

    // The diagram container used to be rendered ONLY in the success branch, so
    // `elementRef` was null on the render effect's first (and only) run. The
    // effect bails out on a null ref before the `finally` that clears
    // `isLoading` — so the ref could never become non-null and the component
    // spun on "Rendering diagram…" forever. Feeding the mocked `marked` back
    // its own input puts the real placeholder into the DOM, which is what lets
    // the mermaid root actually mount here.
    it('mounts the diagram container instead of spinning forever', async () => {
      const markedMock = vi.mocked(marked)
      markedMock.mockClear()
      markedMock.mockImplementationOnce((src: string) => Promise.resolve(src))

      const { container } = render(<MarkdownRenderer content={FENCE} />)

      // Positive gate FIRST: pre-fix the container is never mounted, so this
      // waitFor is what actually fails. Asserting only that the spinner is
      // absent would pass vacuously — before the mermaid root mounts, neither
      // the spinner nor the container exists.
      await waitFor(
        () => {
          expect(
            container.querySelector('.mermaid-container')
          ).toBeInTheDocument()
        },
        { timeout: 3000 }
      )
      await waitFor(
        () => {
          expect(
            screen.queryByText('Rendering diagram...')
          ).not.toBeInTheDocument()
        },
        { timeout: 3000 }
      )
    })

    it('leaves the fence alone when mermaid is disabled', async () => {
      const markedMock = vi.mocked(marked)
      markedMock.mockClear()

      render(<MarkdownRenderer content={FENCE} enableMermaid={false} />)

      await waitFor(() => {
        expect(markedMock).toHaveBeenCalled()
      })

      const source = markedMock.mock.calls[0]?.[0]
      expect(source).toContain('```mermaid')
      expect(source).not.toContain('data-mermaid-id')
    })
  })

  // Regression guard for the mermaid securityLevel coupling: the component
  // sanitizes mermaid output with an SVG-only DOMPurify profile. That profile
  // PRESERVES the raw `<svg>` returned in 'strict' mode but STRIPS the
  // `<iframe src="data:…">` wrapper that 'sandbox' mode returns — which would
  // blank every diagram. These assertions fail loudly if the sanitize config
  // and the configured securityLevel ever drift back out of sync.
  describe('mermaid SVG sanitization contract', () => {
    const sanitizeConfig = { USE_PROFILES: { svg: true, svgFilters: true } }

    it('preserves a raw <svg> (strict-mode mermaid output)', () => {
      const out = DOMPurify.sanitize(
        '<svg viewBox="0 0 10 10"><g><rect width="10" height="10"></rect></g></svg>',
        sanitizeConfig
      )
      expect(out).not.toBe('')
      expect(out).toContain('<svg')
      expect(out).toContain('rect')
    })

    it('strips a sandbox-mode <iframe> wrapper (would blank the diagram)', () => {
      const out = DOMPurify.sanitize(
        '<iframe src="data:text/html;base64,PHN2Zz48L3N2Zz4=" sandbox=""></iframe>',
        sanitizeConfig
      )
      expect(out).toBe('')
    })

    it('removes script and event-handler XSS vectors from SVG', () => {
      const out = DOMPurify.sanitize(
        '<svg><script>alert(1)</script><rect onclick="alert(1)"></rect></svg>',
        sanitizeConfig
      )
      expect(out).not.toContain('<script')
      expect(out).not.toContain('onclick')
    })
  })

  // The guard (VIBEXP-FRONTEND-JS-3) aborts the render effect after more than
  // five runs inside a 200ms window. Its anchor is seeded lazily on the first
  // evaluation rather than read during render, so these pin the threshold that
  // change had to preserve. `shouldAdvanceTime: false` overrides the repo-wide
  // default in vitest.config.ts to freeze the clock outright — against a
  // self-advancing one, "within 200ms" would be a race.
  describe('rapid re-render guard', () => {
    const NOW = new Date('2026-06-20T12:00:00.000Z').getTime()

    beforeEach(() => {
      vi.useFakeTimers({ shouldAdvanceTime: false })
      vi.setSystemTime(NOW)
    })

    afterEach(() => {
      vi.useRealTimers()
    })

    const LOOP_MESSAGE =
      'MarkdownRenderer: detected rapid re-render loop, aborting render'

    it('aborts only after the sixth run inside the window', () => {
      const consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => undefined)

      const { rerender } = render(<MarkdownRenderer content="run-1" />)
      // Runs 2-5 stay under the limit; changing `content` is what re-runs the
      // render effect.
      for (const n of [2, 3, 4, 5]) {
        rerender(<MarkdownRenderer content={`run-${String(n)}`} />)
      }
      expect(consoleError).not.toHaveBeenCalledWith(LOOP_MESSAGE)

      rerender(<MarkdownRenderer content="run-6" />)
      expect(consoleError).toHaveBeenCalledWith(LOOP_MESSAGE)

      consoleError.mockRestore()
    })

    it('does not trip on re-renders spread beyond the window', () => {
      const consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => undefined)

      const { rerender } = render(<MarkdownRenderer content="slow-1" />)
      for (const n of [2, 3, 4, 5, 6, 7, 8]) {
        // Each run lands outside the previous window, which re-anchors it and
        // resets the counter.
        vi.setSystemTime(NOW + n * 250)
        rerender(<MarkdownRenderer content={`slow-${String(n)}`} />)
      }

      expect(consoleError).not.toHaveBeenCalledWith(LOOP_MESSAGE)
      consoleError.mockRestore()
    })

    // The window slides: it is re-anchored by every run that lands outside it.
    // Were the anchor left at its seed, `now - anchor` would only grow, the
    // guard would take the reset branch forever, and a loop starting any time
    // after mount would go undetected.
    it('re-anchors the window, so a burst long after mount still trips', () => {
      const consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => undefined)

      const { rerender } = render(<MarkdownRenderer content="calm-1" />)
      vi.setSystemTime(NOW + 60_000)
      rerender(<MarkdownRenderer content="calm-2" />)
      expect(consoleError).not.toHaveBeenCalledWith(LOOP_MESSAGE)

      // Clock frozen at the new anchor: a burst from here must be caught.
      for (const n of [1, 2, 3, 4, 5, 6]) {
        rerender(<MarkdownRenderer content={`burst-${String(n)}`} />)
      }

      expect(consoleError).toHaveBeenCalledWith(LOOP_MESSAGE)
      consoleError.mockRestore()
    })
  })

  // Regression guard for #884. A wide markdown table used to escape its card
  // and paint over the resource detail sidebar: `.v2-root .prose :where(table)`
  // sets `width: 100%`, which does NOT cap an auto-layout table — it still
  // grows to its min-content width. The fix wraps every rendered table in a
  // `.markdown-table-wrapper` scroll container during the same post-processing
  // pass that rewrites code blocks. `marked` is stubbed in this file, so these
  // assert the post-processing pass itself (the part that broke) by feeding it
  // the markup marked really emits.
  describe('table scroll wrapper (#884)', () => {
    const TABLE =
      '<table><thead><tr><th>a</th><th>b</th></tr></thead>' +
      '<tbody><tr><td>1</td><td>2</td></tr></tbody></table>'

    it('wraps a rendered table in a scroll container that survives sanitization', async () => {
      mockMarked.mockResolvedValue(TABLE)

      const { container } = render(<MarkdownRenderer content="| a | b |" />)

      await waitFor(() => {
        expect(container.querySelector('table')).toBeInTheDocument()
      })

      const table = container.querySelector('table')
      // DOMPurify keeps `div` and `class` by default, so no ADD_ATTR change is
      // needed — asserted here rather than assumed.
      expect(table?.parentElement).toHaveClass('markdown-table-wrapper')
      expect(
        container.querySelectorAll('.markdown-table-wrapper')
      ).toHaveLength(1)
    })

    it('wraps a table that carries attributes', async () => {
      // marked emits a bare `<table>` today. A literal string replacement would
      // silently no-op — reinstating the bug with a green suite — if a future
      // marked ever emitted attributes, so the opening tag is matched by regex.
      mockMarked.mockResolvedValue(
        '<table class="x" data-y="z"><tbody><tr><td>1</td></tr></tbody></table>'
      )

      const { container } = render(<MarkdownRenderer content="| a |" />)

      await waitFor(() => {
        expect(container.querySelector('table')).toBeInTheDocument()
      })

      const table = container.querySelector('table')
      expect(table?.parentElement).toHaveClass('markdown-table-wrapper')
      // The original attributes must survive the rewrite.
      expect(table).toHaveClass('x')
      expect(table?.getAttribute('data-y')).toBe('z')
    })

    it('gives each of several tables its own wrapper', async () => {
      mockMarked.mockResolvedValue(`${TABLE}<p>between</p>${TABLE}`)

      const { container } = render(<MarkdownRenderer content="two tables" />)

      await waitFor(() => {
        expect(container.querySelectorAll('table')).toHaveLength(2)
      })

      expect(
        container.querySelectorAll('.markdown-table-wrapper')
      ).toHaveLength(2)
      container.querySelectorAll('table').forEach(table => {
        expect(table.parentElement).toHaveClass('markdown-table-wrapper')
      })
    })

    it('adds no wrapper to a document without a table', async () => {
      mockMarked.mockResolvedValue('<p>plain paragraph</p>')

      const { container } = render(<MarkdownRenderer content="plain" />)

      await waitFor(() => {
        expect(screen.getByText('plain paragraph')).toBeInTheDocument()
      })

      expect(
        container.querySelector('.markdown-table-wrapper')
      ).not.toBeInTheDocument()
    })

    it('does not wrap table markup shown inside a fenced code block', async () => {
      // marked escapes a fence body, so `<table>` inside one arrives as
      // `&lt;table&gt;` and must stay literal text.
      mockMarked.mockResolvedValue(
        '<pre><code class="language-text">&lt;table&gt;&lt;/table&gt;</code></pre>'
      )

      const { container } = render(
        <MarkdownRenderer content="fenced code block" />
      )

      await waitFor(() => {
        expect(container.querySelector('pre')).toBeInTheDocument()
      })

      expect(
        container.querySelector('.markdown-table-wrapper')
      ).not.toBeInTheDocument()
      expect(container.querySelector('table')).not.toBeInTheDocument()
    })
  })
})
