import type { SearchSummarySource } from '@/services/searchService'
import { ApiError } from '@/types/errors'

import {
  classifySummaryError,
  renderCompactSummaryHtml,
  renderSummaryHtml,
  sourceUrl,
  summaryKey,
} from '../aiSummary'

function makeSource(
  overrides: Partial<SearchSummarySource>
): SearchSummarySource {
  return {
    index: 1,
    type: 'memory',
    id: 'mem-1',
    title: 'A memory',
    slug: '',
    project_id: 'proj-1',
    project_name: 'Project One',
    updated_at: '2026-01-01T00:00:00Z',
    truncated: false,
    ...overrides,
  }
}

function apiError(code: string): ApiError {
  return new ApiError({
    type: `https://api.vibexp.io/errors/${code}`,
    title: 'Error',
    status: 502,
    detail: 'upstream detail',
    code,
    request_id: 'req-1',
    timestamp: '2026-01-01T00:00:00Z',
  })
}

describe('summaryKey', () => {
  it('normalizes the query so equivalent searches share one summary', () => {
    expect(summaryKey('t', '  Retry Config ', undefined, undefined)).toBe(
      summaryKey('t', 'retry config', undefined, undefined)
    )
  })

  it('changes with the team, query, type and project', () => {
    const base = summaryKey('t', 'q', undefined, undefined)
    expect(summaryKey('t2', 'q', undefined, undefined)).not.toBe(base)
    expect(summaryKey('t', 'q2', undefined, undefined)).not.toBe(base)
    expect(summaryKey('t', 'q', 'memories', undefined)).not.toBe(base)
    expect(summaryKey('t', 'q', undefined, 'proj-1')).not.toBe(base)
  })
})

describe('classifySummaryError', () => {
  const codes = [
    'AI_SUMMARY_DISABLED',
    'AI_SUMMARY_NO_PROVIDER',
    'AI_SUMMARY_NO_RESULTS',
    'AI_SUMMARY_PROVIDER_UNREACHABLE',
    'AI_SUMMARY_UNAUTHORIZED',
    'AI_SUMMARY_MODEL_ERROR',
    'AI_SUMMARY_TIMEOUT',
  ]

  it('gives every AI_SUMMARY_* code its own message', () => {
    const messages = codes.map(code => {
      const result = classifySummaryError(apiError(code))
      expect(result.code).toBe(code)
      return result.message
    })
    expect(new Set(messages).size).toBe(codes.length)
  })

  it('classifies transport failures from unwrap', () => {
    expect(
      classifySummaryError(
        new Error('Network error: Unable to connect to server')
      ).code
    ).toBe('NETWORK_ERROR')
    expect(
      classifySummaryError(
        new Error('Request timeout: the server took too long to respond')
      ).code
    ).toBe('CLIENT_TIMEOUT')
  })

  it('falls back to a generic message for an unknown code', () => {
    const unknown = classifySummaryError(apiError('SOMETHING_NEW'))
    expect(unknown.code).toBe('SOMETHING_NEW')
    expect(unknown.message).toBe('Something went wrong generating the summary.')
    expect(classifySummaryError('boom').code).toBe('UNKNOWN')
  })
})

describe('sourceUrl', () => {
  it('builds in-app links through buildResourceUrl', () => {
    expect(sourceUrl(makeSource({ type: 'memory', id: 'm-9' }))).toBe(
      '/memories/m-9'
    )
    expect(
      sourceUrl(
        makeSource({ type: 'artifact', slug: 'a-slug', project_id: 'p-1' })
      )
    ).toBe('/artifacts/p-1/a-slug')
    expect(sourceUrl(makeSource({ type: 'prompt', slug: '' }))).toBeNull()
  })
})

// `marked` is aliased to tests/mocks/marked.js, which wraps the input in <p>,
// so these assert on the citation rewrite + DOMPurify output, not on parsing.
function toDom(html: string): HTMLElement {
  const el = document.createElement('div')
  el.innerHTML = html
  return el
}

describe('renderSummaryHtml', () => {
  const sources = [
    makeSource({ index: 1, id: 'mem-1' }),
    makeSource({
      index: 2,
      type: 'prompt',
      id: 'pr-2',
      slug: 'retry-guide',
    }),
  ]

  it('turns known [n] citations into links to their sources', () => {
    const dom = toDom(
      renderSummaryHtml('Use retries [1] and backoff [2].', sources)
    )
    const links = dom.querySelectorAll('a[data-citation]')
    expect(links).toHaveLength(2)
    expect(links[0]).toHaveAttribute('href', '/memories/mem-1')
    expect(links[0]).toHaveAttribute('data-resource-id', 'mem-1')
    expect(links[0]).toHaveTextContent('[1]')
    expect(links[1]).toHaveAttribute('href', '/prompts/retry-guide')
    expect(dom).toHaveTextContent('Use retries [1] and backoff [2].')
  })

  it('leaves an unknown citation number as literal text', () => {
    const dom = toDom(renderSummaryHtml('See [7] and [1].', sources))
    expect(dom.querySelectorAll('a[data-citation]')).toHaveLength(1)
    expect(dom).toHaveTextContent('See [7] and [1].')
  })

  it('does not rewrite citations inside code', () => {
    const dom = toDom(renderSummaryHtml('<code>arr[1]</code>', sources))
    expect(dom.querySelectorAll('a[data-citation]')).toHaveLength(0)
    expect(dom.querySelector('code')).toHaveTextContent('arr[1]')
  })

  it('sanitizes scripted payloads out of the summary', () => {
    const html = renderSummaryHtml(
      'Hi <script>window.pwned = true</script><img src="x" onerror="window.pwned = true"> <a href="javascript:alert(1)">x</a> [1]',
      sources
    )
    const dom = toDom(html)
    expect(dom.querySelector('script')).toBeNull()
    expect(html).not.toContain('onerror')
    expect(html).not.toContain('javascript:')
    // The citation rewrite survives sanitization.
    expect(dom.querySelector('a[data-citation="1"]')).not.toBeNull()
  })
})

describe('renderCompactSummaryHtml', () => {
  const compactSources = [makeSource({ index: 1 })]

  it('renders known citations as plain superscripts, not links', () => {
    const dom = toDom(
      renderCompactSummaryHtml('See [1] and [9].', compactSources)
    )
    expect(dom.querySelector('sup[data-citation="1"]')).toHaveTextContent('1')
    expect(dom.querySelectorAll('a')).toHaveLength(0)
    // An unknown number stays literal.
    expect(dom).toHaveTextContent('and [9].')
  })

  it('sanitizes scripted payloads out of the summary', () => {
    const html = renderCompactSummaryHtml(
      'Hi <script>window.pwned = true</script><img src="x" onerror="window.pwned = true"> [1]',
      compactSources
    )
    const dom = toDom(html)
    expect(dom.querySelector('script')).toBeNull()
    expect(html).not.toContain('onerror')
    expect(dom.querySelector('sup[data-citation="1"]')).not.toBeNull()
  })
})
