import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { TaxonomyInput } from '../TaxonomyInput'

/**
 * The blur commit, pinned on its own because the failure is silent: the draft
 * lives in the control's local state, and clicking Save blurs the input on the
 * way to submitting the form — so without this a typed-but-not-Entered entry
 * simply never reaches the payload.
 */
describe('TaxonomyInput — committing the draft', () => {
  it('commits a typed entry on blur, not only on Enter', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <>
        <TaxonomyInput value={[]} onChange={onChange} aria-label="Labels" />
        <button type="button">Save</button>
      </>
    )

    await user.type(screen.getByLabelText('Labels'), 'onboarding')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(onChange).toHaveBeenCalledWith(['onboarding'])
  })

  it('commits nothing for a blank or duplicate draft', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <>
        <TaxonomyInput
          value={['onboarding']}
          onChange={onChange}
          aria-label="Labels"
        />
        <button type="button">Save</button>
      </>
    )

    await user.type(screen.getByLabelText('Labels'), '   ')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await user.type(screen.getByLabelText('Labels'), 'onboarding')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(onChange).not.toHaveBeenCalled()
  })
})
