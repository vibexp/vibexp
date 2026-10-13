import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'

import { ReadingChromeContext } from '@/components/patterns/reading-page/readingChrome'

import { ResourceBodyEditor } from '../ResourceBodyEditor'

/**
 * An edit page's header offers the editor a slot for its controls, the way a
 * reading page's toolbar hosts the Rendered / Raw switch, so the body starts
 * at the same place under the header on both.
 */
function EditPage({ withSlot }: Readonly<{ withSlot: boolean }>) {
  const [slot, setSlot] = useState<HTMLElement | null>(null)
  return (
    <ReadingChromeContext.Provider
      value={{ setBodyView: vi.fn(), controlsSlot: slot }}
    >
      <header data-testid="header">
        {withSlot && <div ref={setSlot} data-testid="slot" />}
      </header>
      <div data-testid="editor">
        <ResourceBodyEditor
          value="Body"
          onChange={vi.fn()}
          extensions={{ templates: vi.fn(), render: { content: <p>Shown</p> } }}
        />
      </div>
    </ReadingChromeContext.Provider>
  )
}

describe('ResourceBodyEditor header controls', () => {
  it('draws its controls in the header slot when the page offers one', async () => {
    const user = userEvent.setup()
    render(<EditPage withSlot />)

    const slot = screen.getByTestId('slot')
    const editor = screen.getByTestId('editor')
    expect(
      within(slot).getByRole('tablist', { name: 'Body view' })
    ).toBeInTheDocument()
    expect(
      within(slot).getByRole('button', { name: /load template/i })
    ).toBeInTheDocument()
    expect(within(editor).queryByRole('tablist')).not.toBeInTheDocument()
    // The body itself stays where it was.
    expect(within(editor).getByRole('tabpanel')).toBeInTheDocument()

    // The switch still drives the editor from up there.
    await user.click(within(slot).getByRole('tab', { name: 'Resolved' }))
    expect(within(editor).getByText('Shown')).toBeInTheDocument()
  })

  it('keeps its controls above the text when the header has no slot', () => {
    render(<EditPage withSlot={false} />)

    const editor = screen.getByTestId('editor')
    expect(
      within(editor).getByRole('tablist', { name: 'Body view' })
    ).toBeInTheDocument()
    expect(
      within(screen.getByTestId('header')).queryByRole('tablist')
    ).not.toBeInTheDocument()
  })

  it('keeps its controls above the text outside a reading page', () => {
    render(
      <div data-testid="editor">
        <ResourceBodyEditor value="Body" onChange={vi.fn()} />
      </div>
    )

    expect(
      within(screen.getByTestId('editor')).getByRole('tablist', {
        name: 'Body view',
      })
    ).toBeInTheDocument()
  })
})
