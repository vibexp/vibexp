import '@testing-library/jest-dom'

import { render, waitFor } from '@testing-library/react'

import { MarkdownRenderer } from './MarkdownRenderer'

// The real `marked` on purpose: the placeholder treatment is a marked
// extension, so it can only be observed through a real parse. The suite-wide
// alias in `vitest.config.ts` points `marked` at a stub, so the real module is
// loaded by file path (a non-literal specifier, which the alias cannot touch).
vi.mock('marked', () => {
  const real = '../../node_modules/marked/lib/marked.esm.js'
  return import(/* @vite-ignore */ real) as Promise<typeof import('marked')>
})

vi.mock('mermaid', () => {
  const mermaid = {
    initialize: vi.fn(),
    render: vi.fn().mockResolvedValue({ svg: '<svg></svg>' }),
  }
  return { ...mermaid, default: mermaid }
})

const BODY = [
  '# Review for {{repository}}',
  '',
  'You are reviewing a pull request for **{{repository}}** in *{{ language }}*.',
  '',
  '- Language: {{language}}',
  '- Risk tolerance: {{risk_level}}',
  '',
  '| Field | Value |',
  '| --- | --- |',
  '| Repo | {{repository}} |',
  '',
  'Inline `{{language}}` stays code.',
  '',
  '```text',
  'repo={{repository}} risk={{risk_level}}',
  '```',
].join('\n')

async function renderBody(
  placeholders?: Record<string, string>,
  content = BODY
) {
  const { container } = render(
    <MarkdownRenderer content={content} placeholders={placeholders} />
  )
  await waitFor(() => {
    expect(container.querySelector('.markdown-renderer > *')).not.toBeNull()
  })
  return container
}

const chips = (root: Element, selector = '') =>
  [...root.querySelectorAll(`${selector} .prompt-placeholder`)].map(
    el => el.textContent
  )

describe('MarkdownRenderer prompt placeholders', () => {
  it('shows every unfilled placeholder as a chip naming the variable', async () => {
    const container = await renderBody({
      repository: '',
      language: '',
      risk_level: '',
    })

    expect(container).not.toHaveTextContent('****')
    expect(chips(container, 'h1')).toEqual(['{{repository}}'])
    expect(chips(container, 'strong')).toEqual(['{{repository}}'])
    // The key is trimmed, as the backend trims it.
    expect(chips(container, 'em')).toEqual(['{{language}}'])
    expect(chips(container, 'li')).toEqual(['{{language}}', '{{risk_level}}'])
    expect(chips(container, 'td')).toEqual(['{{repository}}'])
    expect(
      container.querySelector('.prompt-placeholder-filled')
    ).not.toBeInTheDocument()
  })

  it('treats a placeholder with no entry at all as unfilled', async () => {
    const container = await renderBody({})

    expect(chips(container, 'li')).toEqual(['{{language}}', '{{risk_level}}'])
  })

  it('replaces a filled placeholder with its value, marked as substituted', async () => {
    const container = await renderBody({
      repository: 'vibexp/vibexp',
      language: '',
      risk_level: 'low',
    })

    const strong = container.querySelector('strong .prompt-placeholder')
    expect(strong).toHaveTextContent('vibexp/vibexp')
    expect(strong).toHaveClass('prompt-placeholder-filled')
    expect(strong).toHaveAttribute('data-placeholder', 'repository')

    // Filled and unfilled side by side in the list.
    const [language, risk] = container.querySelectorAll(
      'li .prompt-placeholder'
    )
    expect(language).toHaveTextContent('{{language}}')
    expect(language).not.toHaveClass('prompt-placeholder-filled')
    expect(risk).toHaveTextContent('low')
    expect(risk).toHaveClass('prompt-placeholder-filled')
  })

  it('never draws a chip inside code, and substitutes only filled values there', async () => {
    const container = await renderBody({
      repository: 'vibexp/vibexp',
      language: '',
      risk_level: '',
    })

    expect(
      container.querySelector('code .prompt-placeholder')
    ).not.toBeInTheDocument()
    expect(container.querySelector('p > code')).toHaveTextContent(
      '{{language}}'
    )
    expect(container.querySelector('pre code')).toHaveTextContent(
      'repo=vibexp/vibexp risk={{risk_level}}'
    )
  })

  it('inserts a value as text, not as markup or markdown', async () => {
    const container = await renderBody(
      { name: '<img src=x onerror=alert(1)> **x** {{other}}' },
      'Hello {{name}}'
    )

    const chip = container.querySelector('.prompt-placeholder-filled')
    expect(chip).toHaveTextContent(
      '<img src=x onerror=alert(1)> **x** {{other}}'
    )
    expect(container.querySelector('img')).not.toBeInTheDocument()
    expect(chip?.querySelector('strong')).not.toBeInTheDocument()
  })

  it('leaves {{…}} as ordinary text when the prop is not passed', async () => {
    const container = await renderBody(undefined)

    expect(
      container.querySelector('.prompt-placeholder')
    ).not.toBeInTheDocument()
    expect(container.querySelector('strong')).toHaveTextContent(
      '{{repository}}'
    )
  })
})
