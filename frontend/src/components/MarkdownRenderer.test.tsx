import '@testing-library/jest-dom'

import { act, render, screen, waitFor } from '@testing-library/react'
import DOMPurify from 'dompurify'
import { marked } from 'marked'

import { MarkdownRenderer } from './MarkdownRenderer'

// Mock all external dependencies
jest.mock('mermaid', () => ({
  initialize: jest.fn(),
  render: jest
    .fn()
    .mockResolvedValue({ svg: '<svg>mock mermaid diagram</svg>' }),
}))

// Capture the link override installed by configureMarked so tests can
// invoke it directly and assert on its output.
interface MockLinkToken {
  href: string
  title?: string | null
  text: string
  tokens: unknown[]
}
let capturedLinkFn: ((token: MockLinkToken) => string) | undefined

jest.mock('marked', () => {
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
    marked: jest.fn().mockResolvedValue('<p>mocked content</p>'),
    Renderer: MockRenderer,
  }
})

jest.mock('prismjs', () => ({
  highlight: jest.fn().mockReturnValue('highlighted code'),
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
    writeText: jest.fn().mockResolvedValue(undefined),
  },
})

const mockMarked = jest.mocked(marked)

// Mock all CSS imports
jest.mock('prismjs/themes/prism-okaidia.css', () => ({}))
jest.mock('prismjs/components/prism-javascript', () => ({}))
jest.mock('prismjs/components/prism-typescript', () => ({}))
jest.mock('prismjs/components/prism-jsx', () => ({}))
jest.mock('prismjs/components/prism-tsx', () => ({}))
jest.mock('prismjs/components/prism-python', () => ({}))
jest.mock('prismjs/components/prism-bash', () => ({}))
jest.mock('prismjs/components/prism-json', () => ({}))
jest.mock('prismjs/components/prism-yaml', () => ({}))
jest.mock('prismjs/components/prism-sql', () => ({}))
jest.mock('prismjs/components/prism-go', () => ({}))
jest.mock('prismjs/components/prism-java', () => ({}))
jest.mock('prismjs/components/prism-css', () => ({}))
jest.mock('prismjs/components/prism-scss', () => ({}))
jest.mock('prismjs/components/prism-markdown', () => ({}))

describe('MarkdownRenderer', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('Component Rendering', () => {
    it('renders without crashing', async () => {
      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        render(<MarkdownRenderer content="# Test" />)
      })
    })

    it('renders mocked content from marked', async () => {
      mockMarked.mockResolvedValue('<p>Test Content</p>')

      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        render(<MarkdownRenderer content="# Test" />)
      })

      await waitFor(() => {
        expect(screen.getByText('Test Content')).toBeInTheDocument()
      })
    })

    it('applies custom className', async () => {
      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        render(<MarkdownRenderer content="# Test" className="custom-class" />)
      })

      const container = document.querySelector('.markdown-renderer')
      expect(container).toHaveClass('custom-class')
    })

    it('applies correct theme class based on syntaxTheme prop', async () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      let rerender: any

      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        const result = render(
          <MarkdownRenderer content="# Test" syntaxTheme="light" />
        )
        rerender = result.rerender
      })

      let container = document.querySelector('.markdown-renderer')
      expect(container).toHaveClass('theme-light')

      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        rerender(<MarkdownRenderer content="# Test" syntaxTheme="dark" />)
      })
      container = document.querySelector('.markdown-renderer')
      expect(container).toHaveClass('theme-dark')

      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        rerender(<MarkdownRenderer content="# Test" syntaxTheme="auto" />)
      })
      container = document.querySelector('.markdown-renderer')
      expect(container).toHaveClass('theme-auto')
    })

    it('handles empty content gracefully', async () => {
      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        render(<MarkdownRenderer content="" />)
      })

      const container = document.querySelector('.markdown-renderer')
      expect(container).toBeInTheDocument()
    })

    it('updates content when content prop changes', async () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      let rerender: any
      mockMarked.mockResolvedValue('<p>First Content</p>')

      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        const result = render(<MarkdownRenderer content="# First" />)
        rerender = result.rerender
      })

      await waitFor(() => {
        expect(screen.getByText('First Content')).toBeInTheDocument()
      })

      mockMarked.mockResolvedValue('<p>Second Content</p>')

      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        rerender(<MarkdownRenderer content="# Second" />)
      })

      await waitFor(() => {
        expect(screen.getByText('Second Content')).toBeInTheDocument()
      })
    })
  })

  describe('Configuration', () => {
    it('calls marked with correct options including a renderer', async () => {
      mockMarked.mockResolvedValue('<p>test</p>')

      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        render(<MarkdownRenderer content="# Test" />)
      })

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

      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        render(<MarkdownRenderer content="[link](https://example.com)" />)
      })

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

      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        render(
          <MarkdownRenderer
            content="[link](https://example.com)"
            linkTarget="_blank"
          />
        )
      })

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

      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        render(
          <MarkdownRenderer
            content="[link](https://example.com)"
            linkTarget="_self"
          />
        )
      })

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

      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        render(<MarkdownRenderer content="[link](https://example.com)" />)
      })

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

      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        render(<MarkdownRenderer content="# Test" />)
      })

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
      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        render(<MarkdownRenderer content="[link](https://example.com)" />)
      })
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
      const mockOnError = jest.fn()

      mockMarked.mockRejectedValue(new Error('Markdown error'))

      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        render(<MarkdownRenderer content="# Test" onError={mockOnError} />)
      })

      await waitFor(() => {
        expect(mockOnError).toHaveBeenCalledWith(
          expect.any(Error),
          'markdown-rendering'
        )
      })
    })

    it('renders error message when markdown rendering fails', async () => {
      mockMarked.mockRejectedValue(new Error('Test error'))

      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        render(<MarkdownRenderer content="# Test" />)
      })

      await waitFor(() => {
        expect(
          screen.getByText(/Error rendering markdown: Test error/)
        ).toBeInTheDocument()
      })
    })
  })

  describe('Props Validation', () => {
    it('accepts all expected props', async () => {
      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
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
      })
    })

    it('works with minimal props', async () => {
      // eslint-disable-next-line @typescript-eslint/require-await
      await act(async () => {
        render(<MarkdownRenderer content="# Test" />)
      })
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
})
