import { render, screen } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import type { ReactNode } from 'react'

import {
  Panel,
  PanelAction,
  PanelBody,
  PanelHeader,
  PanelPresentationProvider,
  PanelRow,
  PanelTitle,
  usePanelInset,
  usePanelPresentation,
} from '../panel'

/** Renders `ui` on a flat surface — what `DetailsColumn` provides (#890). */
function flat(ui: ReactNode) {
  return render(
    <PanelPresentationProvider value="flat">{ui}</PanelPresentationProvider>
  )
}

const CARD_CHROME = ['rounded-lg', 'border', 'shadow-sm', 'bg-card'] as const

describe('panel presentation', () => {
  it('defaults to card when no surface declares one', () => {
    function Probe() {
      return (
        <>
          <span data-testid="presentation">{usePanelPresentation()}</span>
          <span data-testid="inset">{usePanelInset()}</span>
        </>
      )
    }
    render(<Probe />)
    expect(screen.getByTestId('presentation')).toHaveTextContent('card')
    expect(screen.getByTestId('inset')).toHaveTextContent('px-5')
  })

  it('hands the flat inset to widgets that pad their own sub-rows', () => {
    function Probe() {
      return <span data-testid="inset">{usePanelInset()}</span>
    }
    flat(<Probe />)
    expect(screen.getByTestId('inset')).toHaveTextContent('px-0')
  })
})

describe('Panel', () => {
  it('paints the card box by default', () => {
    render(<Panel data-testid="p" />)
    expect(screen.getByTestId('p')).toHaveClass(...CARD_CHROME)
  })

  it('paints no chrome at all when flat', () => {
    flat(<Panel data-testid="p" />)
    const panel = screen.getByTestId('p')
    for (const cls of CARD_CHROME) expect(panel).not.toHaveClass(cls)
    expect(panel.className).toBe('')
  })

  it('still takes a caller className when flat', () => {
    flat(<Panel data-testid="p" className="mt-2" />)
    expect(screen.getByTestId('p')).toHaveClass('mt-2')
  })
})

describe('PanelHeader', () => {
  it('carries the card gutter by default', () => {
    render(<PanelHeader data-testid="h" />)
    expect(screen.getByTestId('h')).toHaveClass('px-5', 'pt-5', 'pb-4')
  })

  it('drops the gutter when flat', () => {
    flat(<PanelHeader data-testid="h" />)
    const header = screen.getByTestId('h')
    expect(header).toHaveClass('pb-2.5')
    expect(header).not.toHaveClass('px-5')
  })
})

describe('PanelTitle', () => {
  it('is 16px on a card and 14px when flat', () => {
    const { unmount } = render(<PanelTitle>Comments</PanelTitle>)
    expect(screen.getByRole('heading', { name: 'Comments' })).toHaveClass(
      'text-base'
    )
    unmount()
    flat(<PanelTitle>Comments</PanelTitle>)
    expect(screen.getByRole('heading', { name: 'Comments' })).toHaveClass(
      'text-sm'
    )
  })

  it('stays a level-settable heading element', () => {
    render(<PanelTitle as="h2">Relations</PanelTitle>)
    expect(
      screen.getByRole('heading', { level: 2, name: 'Relations' })
    ).toBeInTheDocument()
  })
})

describe('PanelAction', () => {
  it('is the standard small outline button on a card', () => {
    render(<PanelAction data-testid="a">Add file</PanelAction>)
    const action = screen.getByTestId('a')
    expect(action).toHaveClass('h-9', 'border')
    expect(action).toHaveAttribute('type', 'button')
  })

  // The full-size button did not fit beside the title at the column's 320px
  // width, which is what truncated "Comments" / "Attachments" (#890).
  it('is a compact 12px chip when flat', () => {
    flat(<PanelAction data-testid="a">Add file</PanelAction>)
    const action = screen.getByTestId('a')
    expect(action).toHaveClass(
      'text-xs',
      'rounded-md',
      'border',
      'px-2',
      'py-1'
    )
    expect(action).not.toHaveClass('h-9')
  })

  it('clicks and disables in both presentations', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()
    const { unmount } = render(
      <PanelAction data-testid="a" onClick={onClick}>
        Add
      </PanelAction>
    )
    await user.click(screen.getByTestId('a'))
    unmount()

    flat(
      <PanelAction data-testid="a" onClick={onClick} disabled>
        Add
      </PanelAction>
    )
    expect(screen.getByTestId('a')).toBeDisabled()
    await user.click(screen.getByTestId('a'))
    expect(onClick).toHaveBeenCalledTimes(1)
  })
})

describe('PanelBody', () => {
  it('insets by the card gutter, or not at all when flat', () => {
    const { unmount } = render(<PanelBody data-testid="b" />)
    expect(screen.getByTestId('b')).toHaveClass('px-5')
    unmount()
    flat(<PanelBody data-testid="b" />)
    expect(screen.getByTestId('b')).toHaveClass('px-0')
  })
})

describe('PanelRow', () => {
  it('is a roomy 14px row inside a card', () => {
    render(<PanelRow data-testid="r" />)
    expect(screen.getByTestId('r')).toHaveClass('min-h-12', 'px-5', 'text-sm')
  })

  it('is a 13px hairline row when flat, with no inset of its own', () => {
    flat(<PanelRow data-testid="r" />)
    const row = screen.getByTestId('r')
    expect(row).toHaveClass('text-[13px]', 'py-2.5')
    expect(row).not.toHaveClass('px-5', 'min-h-12')
  })

  it('renders as the requested element', () => {
    render(
      <ul>
        <PanelRow as="li" data-testid="r" />
      </ul>
    )
    expect(screen.getByTestId('r').tagName).toBe('LI')
  })
})
