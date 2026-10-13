import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import { storage } from '@/utils/storage'

import { UsageDisclosure } from '../UsageDisclosure'

const Chart = () => <div data-testid="access-chart">chart</div>

describe('UsageDisclosure', () => {
  beforeEach(() => {
    storage.clear()
  })

  it('is closed by default and does not mount its content', () => {
    render(
      <UsageDisclosure>
        <Chart />
      </UsageDisclosure>
    )

    expect(screen.getByRole('button', { name: 'Usage' })).toHaveAttribute(
      'aria-expanded',
      'false'
    )
    // Not mounted: a reader who never opens it never pays for the request.
    expect(screen.queryByTestId('access-chart')).not.toBeInTheDocument()
  })

  it('opens on click and remembers the choice for the next page', async () => {
    const user = userEvent.setup()
    const first = render(
      <UsageDisclosure>
        <Chart />
      </UsageDisclosure>
    )

    await user.click(screen.getByRole('button', { name: 'Usage' }))
    expect(screen.getByTestId('access-chart')).toBeInTheDocument()
    expect(storage.get(STORAGE_KEYS.USAGE_OPEN)).toBe('true')

    first.unmount()
    render(
      <UsageDisclosure>
        <Chart />
      </UsageDisclosure>
    )
    expect(screen.getByTestId('access-chart')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Usage' }))
    expect(screen.queryByTestId('access-chart')).not.toBeInTheDocument()
    expect(storage.get(STORAGE_KEYS.USAGE_OPEN)).toBe('false')
  })
})
