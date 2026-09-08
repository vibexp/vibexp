import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { PromptSharedFilter } from '@/pages/prompts/PromptSharedFilter'

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

const onChange = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
})

describe('PromptSharedFilter', () => {
  it('shows the committed tri-state value', () => {
    render(<PromptSharedFilter value="not_shared" onChange={onChange} />)
    expect(screen.getByLabelText('Filter by shared')).toHaveTextContent(
      'Not shared'
    )
  })

  it('emits the raw URL value the page stores', async () => {
    const user = userEvent.setup()
    render(<PromptSharedFilter value="all" onChange={onChange} />)
    await user.click(screen.getByLabelText('Filter by shared'))
    await user.click(await screen.findByRole('option', { name: 'Shared' }))
    expect(onChange).toHaveBeenCalledWith('shared')
  })

  it('sits at the same width as the generated controls', () => {
    render(<PromptSharedFilter value="all" onChange={onChange} />)
    expect(screen.getByTestId('prompt-shared-filter')).toHaveClass('w-[150px]')
  })
})
