import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'

import { storage } from '@/utils/storage'

import { RAW_BODY_CLASS, ResourceBody } from '../ResourceBody'
import type { BodyViewMode } from '../types'

// marked/DOMPurify are heavy in jsdom; the body only needs to prove which text
// reaches the renderer.
vi.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({
    content,
    hideLeadingTitle,
  }: {
    content: string
    hideLeadingTitle?: string
  }) => (
    <div data-testid="markdown-renderer" data-hide-title={hideLeadingTitle}>
      {content}
    </div>
  ),
}))

const RENDERED = 'Hello Ada — rendered'
const RAW = 'Hello {{name}} — raw source'

describe('ResourceBody', () => {
  beforeEach(() => {
    storage.clear()
  })

  it('renders the markdown body in rendered mode by default', () => {
    render(<ResourceBody content={RENDERED} rawContent={RAW} />)

    expect(screen.getByTestId('markdown-renderer')).toHaveTextContent(RENDERED)
    expect(screen.queryByTestId('resource-body-raw')).not.toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Rendered' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  })

  it('renders no card chrome — the article is bare (#899 decision A)', () => {
    const { container } = render(<ResourceBody content={RENDERED} />)

    // The Card primitive is the thing this replaced; its border/shadow classes
    // must not reappear around a body.
    expect(container.querySelector('.rounded-lg.border.shadow-sm')).toBeNull()
  })

  it('switches to the raw source, not the rendered output', async () => {
    const user = userEvent.setup()
    render(<ResourceBody content={RENDERED} rawContent={RAW} />)

    await user.click(screen.getByRole('tab', { name: 'Raw' }))

    expect(screen.getByTestId('resource-body-raw')).toHaveTextContent(RAW)
    expect(screen.queryByTestId('markdown-renderer')).not.toBeInTheDocument()
  })

  it('sets the raw source at text-sm on the surface the editor shares (#1178)', async () => {
    const user = userEvent.setup()
    render(<ResourceBody content={RENDERED} rawContent={RAW} />)

    await user.click(screen.getByRole('tab', { name: 'Raw' }))

    const raw = screen.getByTestId('resource-body-raw')
    expect(raw).toHaveClass(...RAW_BODY_CLASS.split(' '))
    expect(raw).toHaveClass('text-sm')
    expect(raw).not.toHaveClass('text-xs')
  })

  it('falls back to `content` in raw mode when no rawContent is given', async () => {
    const user = userEvent.setup()
    render(<ResourceBody content={RENDERED} />)

    await user.click(screen.getByRole('tab', { name: 'Raw' }))

    expect(screen.getByTestId('resource-body-raw')).toHaveTextContent(RENDERED)
  })

  it('tells the renderer the page title, so a repeated heading is left out, and never touches Raw', async () => {
    const user = userEvent.setup()
    render(<ResourceBody content={'# Guide\n\nBody'} title="Guide" />)

    expect(screen.getByTestId('markdown-renderer')).toHaveAttribute(
      'data-hide-title',
      'Guide'
    )

    await user.click(screen.getByRole('tab', { name: 'Raw' }))
    // The source is shown exactly as stored, heading included.
    expect(screen.getByTestId('resource-body-raw').textContent).toBe(
      '# Guide\n\nBody'
    )
  })

  it('mounts exactly one tabpanel, so the body is never in the DOM twice', async () => {
    const user = userEvent.setup()
    render(<ResourceBody content={RENDERED} rawContent={RAW} />)

    expect(screen.getAllByRole('tabpanel')).toHaveLength(1)
    await user.click(screen.getByRole('tab', { name: 'Raw' }))
    expect(screen.getAllByRole('tabpanel')).toHaveLength(1)
  })

  it('opens the next resource on Rendered after Raw was chosen on this one', async () => {
    const user = userEvent.setup()
    const { unmount } = render(<ResourceBody content={RENDERED} />)

    await user.click(screen.getByRole('tab', { name: 'Raw' }))
    expect(screen.getByTestId('resource-body-raw')).toBeInTheDocument()
    // Raw is for this page view only: nothing is written anywhere.
    expect(globalThis.localStorage).toHaveLength(0)

    unmount()
    render(<ResourceBody content="Another resource" />)

    expect(screen.getByTestId('markdown-renderer')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Rendered' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  })

  it('renders renderedExtra only in rendered mode', async () => {
    const user = userEvent.setup()
    render(
      <ResourceBody
        content={RENDERED}
        rawContent={RAW}
        renderedExtra={<div data-testid="extra">placeholder inputs</div>}
      />
    )

    expect(screen.getByTestId('extra')).toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: 'Raw' }))
    expect(screen.queryByTestId('extra')).not.toBeInTheDocument()
  })

  it('shows a spinner instead of the body while it is loading', () => {
    render(
      <ResourceBody
        content={RENDERED}
        isLoading
        renderedExtra={<div data-testid="extra">inputs stay put</div>}
      />
    )

    expect(screen.getByText('Rendering…')).toBeInTheDocument()
    expect(screen.queryByTestId('markdown-renderer')).not.toBeInTheDocument()
    // The placeholder inputs must survive the render round-trip, or typing in
    // one would unmount the field mid-keystroke.
    expect(screen.getByTestId('extra')).toBeInTheDocument()
  })

  it('does not show the spinner in raw mode — the source is always available', async () => {
    const user = userEvent.setup()
    render(<ResourceBody content={RENDERED} rawContent={RAW} isLoading />)

    await user.click(screen.getByRole('tab', { name: 'Raw' }))

    expect(screen.queryByText('Rendering…')).not.toBeInTheDocument()
    expect(screen.getByTestId('resource-body-raw')).toHaveTextContent(RAW)
  })

  describe('controlled mode', () => {
    function Controlled() {
      const [mode, setMode] = useState<BodyViewMode>('raw')
      return (
        <>
          <span data-testid="owner-mode">{mode}</span>
          <ResourceBody
            content={RENDERED}
            rawContent={RAW}
            mode={mode}
            onModeChange={setMode}
          />
        </>
      )
    }

    it('honours the owner’s mode and reports changes back to it', async () => {
      const user = userEvent.setup()
      render(<Controlled />)

      // Owner said raw, so raw it is — regardless of the stored preference.
      expect(screen.getByTestId('resource-body-raw')).toBeInTheDocument()

      await user.click(screen.getByRole('tab', { name: 'Rendered' }))

      expect(screen.getByTestId('owner-mode')).toHaveTextContent('rendered')
      expect(screen.getByTestId('markdown-renderer')).toBeInTheDocument()
    })

    it('reports the change to the owner and stores nothing', async () => {
      const user = userEvent.setup()
      const onModeChange = vi.fn()
      render(
        <ResourceBody
          content={RENDERED}
          mode="rendered"
          onModeChange={onModeChange}
        />
      )

      await user.click(screen.getByRole('tab', { name: 'Raw' }))

      expect(onModeChange).toHaveBeenCalledWith('raw')
      expect(globalThis.localStorage).toHaveLength(0)
    })
  })
})
