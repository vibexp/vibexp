import { render, screen } from '@testing-library/react'

import { marksLastWrite } from '../lastWrite'
import { LastWriteIndicator } from '../LastWriteIndicator'

const LONG_NAME = 'An assistant name of thirty ch'

describe('LastWriteIndicator', () => {
  describe('inline, for a meta line', () => {
    it('names MCP and the assistant that wrote', () => {
      render(
        <LastWriteIndicator
          variant="inline"
          source="mcp"
          assistantName="Claude Code"
        />
      )
      expect(screen.getByTestId('last-write-indicator')).toHaveTextContent(
        /^via MCP · Claude Code$/
      )
    })

    it('names MCP alone when the client gave no name', () => {
      render(
        <LastWriteIndicator
          variant="inline"
          source="mcp"
          assistantName={null}
        />
      )
      expect(screen.getByTestId('last-write-indicator')).toHaveTextContent(
        /^via MCP$/
      )
    })

    it.each([
      ['cli', 'via CLI'],
      ['api', 'via API'],
    ] as const)('names a %s write', (source, text) => {
      render(<LastWriteIndicator variant="inline" source={source} />)
      expect(screen.getByTestId('last-write-indicator').textContent).toBe(text)
    })

    it('renders nothing for a web write', () => {
      // A person editing in this app is the unmarked case: only what an AI
      // tool or a script wrote is worth a mark on every row.
      const { container } = render(
        <LastWriteIndicator variant="inline" source="web" />
      )
      expect(container).toBeEmptyDOMElement()
    })

    it.each([[null], [undefined]])(
      'renders nothing when no write was recorded (%s)',
      source => {
        const { container } = render(
          <LastWriteIndicator
            variant="inline"
            source={source}
            assistantName="Claude Code"
          />
        )
        expect(container).toBeEmptyDOMElement()
      }
    )

    it('keeps a long assistant name whole in its title', () => {
      render(
        <LastWriteIndicator
          variant="inline"
          source="mcp"
          assistantName={LONG_NAME}
        />
      )
      expect(screen.getByText(LONG_NAME)).toHaveAttribute('title', LONG_NAME)
    })
  })

  describe('full, for a details row', () => {
    it('names MCP and the assistant that wrote', () => {
      render(
        <LastWriteIndicator
          variant="full"
          source="mcp"
          assistantName="Claude Code"
        />
      )
      expect(screen.getByTestId('last-write-indicator')).toHaveTextContent(
        /^MCP·Claude Code$/
      )
      expect(screen.getByText('Claude Code')).toHaveAttribute(
        'title',
        'Claude Code'
      )
    })

    it.each([
      ['mcp', 'MCP'],
      ['cli', 'CLI'],
      ['api', 'API'],
      ['web', 'Web'],
    ] as const)('names a %s write by its channel label', (source, label) => {
      render(<LastWriteIndicator variant="full" source={source} />)
      expect(screen.getByTestId('last-write-indicator').textContent).toBe(label)
    })

    it('says so when no write was recorded, and names no channel', () => {
      render(
        <LastWriteIndicator
          variant="full"
          source={null}
          assistantName="Claude Code"
        />
      )
      expect(screen.getByTestId('last-write-indicator')).toHaveTextContent(
        /^Not recorded$/
      )
    })
  })

  it('says the channel in words: the colour dot is hidden from assistive tech', () => {
    const { container } = render(
      <LastWriteIndicator variant="full" source="cli" />
    )
    const hidden = container.querySelectorAll('[aria-hidden="true"]')
    expect(hidden).toHaveLength(1)
    expect(hidden[0]).toBeEmptyDOMElement()
    expect(screen.getByText('CLI')).toBeInTheDocument()
  })
})

describe('marksLastWrite', () => {
  it.each([
    ['mcp', true],
    ['cli', true],
    ['api', true],
    ['web', false],
    [null, false],
    [undefined, false],
  ] as const)('%s -> %s', (source, marked) => {
    expect(marksLastWrite(source)).toBe(marked)
  })
})
