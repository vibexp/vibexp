import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import { storage } from '@/utils/storage'

import { ResourceBody } from '../ResourceBody'
import type { BodyViewMode } from '../types'

// marked/DOMPurify are heavy in jsdom; the body only needs to prove which text
// reaches the renderer.
vi.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown-renderer">{content}</div>
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

  it('falls back to `content` in raw mode when no rawContent is given', async () => {
    const user = userEvent.setup()
    render(<ResourceBody content={RENDERED} />)

    await user.click(screen.getByRole('tab', { name: 'Raw' }))

    expect(screen.getByTestId('resource-body-raw')).toHaveTextContent(RENDERED)
  })

  it('mounts exactly one tabpanel, so the body is never in the DOM twice', async () => {
    const user = userEvent.setup()
    render(<ResourceBody content={RENDERED} rawContent={RAW} />)

    expect(screen.getAllByRole('tabpanel')).toHaveLength(1)
    await user.click(screen.getByRole('tab', { name: 'Raw' }))
    expect(screen.getAllByRole('tabpanel')).toHaveLength(1)
  })

  it('persists the chosen mode and rehydrates it on the next mount', async () => {
    const user = userEvent.setup()
    const { unmount } = render(<ResourceBody content={RENDERED} />)

    await user.click(screen.getByRole('tab', { name: 'Raw' }))
    expect(storage.get(STORAGE_KEYS.BODY_VIEW_RAW)).toBe('true')

    unmount()
    // A different resource entirely — the preference is shared, not per-body.
    render(<ResourceBody content="Another resource" />)

    expect(screen.getByTestId('resource-body-raw')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Raw' })).toHaveAttribute(
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

    it('leaves persistence to the owner rather than writing the key twice', async () => {
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
      expect(storage.get(STORAGE_KEYS.BODY_VIEW_RAW)).toBeNull()
    })
  })
})
