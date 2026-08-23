import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { MetadataFilterProps } from '../MetadataFilter'
import { MetadataFilter } from '../MetadataFilter'

// cmdk (used inside the popover) relies on browser APIs jsdom doesn't provide.
beforeAll(() => {
  global.ResizeObserver = class {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  Element.prototype.scrollIntoView = vi.fn()
})

const baseProps = (
  overrides: Partial<MetadataFilterProps> = {}
): MetadataFilterProps => ({
  value: {},
  onChange: vi.fn(),
  keys: ['env', 'team'],
  onOpenCatalog: vi.fn(),
  activeKey: null,
  onSelectKey: vi.fn(),
  values: [],
  valueQuery: '',
  onValueQueryChange: vi.fn(),
  ...overrides,
})

const openPopover = async (
  user: ReturnType<typeof userEvent.setup>
): Promise<void> => {
  await user.click(screen.getByRole('combobox', { name: 'Filter by metadata' }))
}

describe('MetadataFilter', () => {
  it('requests the key catalog when the popover opens', async () => {
    const user = userEvent.setup()
    const props = baseProps()
    render(<MetadataFilter {...props} />)

    expect(props.onOpenCatalog).not.toHaveBeenCalled()
    await openPopover(user)

    expect(props.onOpenCatalog).toHaveBeenCalledTimes(1)
    expect(await screen.findByText('env')).toBeInTheDocument()
  })

  it('selecting a key asks the host to list that key’s values', async () => {
    const user = userEvent.setup()
    const props = baseProps()
    render(<MetadataFilter {...props} />)

    await openPopover(user)
    await user.click(await screen.findByText('env'))

    expect(props.onSelectKey).toHaveBeenCalledWith('env')
  })

  it('multi-selecting values and applying emits one key with both values', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const props = baseProps({
      onChange,
      activeKey: 'env',
      values: ['prod', 'staging', 'dev'],
    })
    render(<MetadataFilter {...props} />)

    await openPopover(user)
    await user.click(await screen.findByText('prod'))
    await user.click(screen.getByText('staging'))
    await user.click(screen.getByRole('button', { name: 'Apply env filter' }))

    expect(onChange).toHaveBeenCalledWith({ env: ['prod', 'staging'] })
  })

  it('applying with nothing selected removes the key rather than emitting an empty array', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    // An empty array means "the key exists" to the backend, which is not what
    // an emptied checkbox list expresses.
    const props = baseProps({
      onChange,
      value: { env: ['prod'] },
      activeKey: 'env',
      values: ['prod'],
    })
    render(<MetadataFilter {...props} />)

    await openPopover(user)
    await user.click(await screen.findByText('prod')) // untick the seeded value
    await user.click(screen.getByRole('button', { name: 'Apply env filter' }))

    expect(onChange).toHaveBeenCalledWith({})
  })

  it('seeds the draft from the committed values so a chip can be edited', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const props = baseProps({
      onChange,
      value: { env: ['prod'] },
      activeKey: 'env',
      values: ['prod', 'staging'],
    })
    render(<MetadataFilter {...props} />)

    await openPopover(user)
    await user.click(await screen.findByText('staging'))
    await user.click(screen.getByRole('button', { name: 'Apply env filter' }))

    expect(onChange).toHaveBeenCalledWith({ env: ['prod', 'staging'] })
  })

  // The re-seed contract, pinned because both halves are easy to break while
  // "fixing" the dependency array: seeding must key off `activeKey` ALONE, yet
  // read the LATEST `value` when it fires.
  it('re-seeds the draft from the current value when activeKey changes', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const { rerender } = render(
      <MetadataFilter {...baseProps({ onChange, activeKey: null })} />
    )

    await openPopover(user)

    // The host commits a value and selects a key in the same update. The seeding
    // effect runs on the activeKey change and must see the value that arrived
    // with it, not the empty one from the previous commit.
    rerender(
      <MetadataFilter
        {...baseProps({
          onChange,
          value: { env: ['prod', 'staging'] },
          activeKey: 'env',
          values: ['prod', 'staging'],
        })}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Apply env filter' }))

    expect(onChange).toHaveBeenCalledWith({ env: ['prod', 'staging'] })
  })

  it('does not re-seed over in-progress toggles when only value changes', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const props = {
      onChange,
      value: { env: ['prod'] },
      activeKey: 'env',
      values: ['prod', 'staging'],
    }
    const { rerender } = render(<MetadataFilter {...baseProps(props)} />)

    await openPopover(user)
    await user.click(await screen.findByText('staging')) // draft: prod + staging

    // A re-render carrying a fresh `value` identity (the host rebuilding its
    // filter object) must not restart the draft from the committed values.
    rerender(
      <MetadataFilter {...baseProps({ ...props, value: { env: ['prod'] } })} />
    )

    await user.click(screen.getByRole('button', { name: 'Apply env filter' }))

    expect(onChange).toHaveBeenCalledWith({ env: ['prod', 'staging'] })
  })

  it('renders one chip per committed key', () => {
    render(
      <MetadataFilter
        {...baseProps({ value: { env: ['prod', 'staging'], team: ['core'] } })}
      />
    )

    expect(screen.getByTestId('metadata-chip-env')).toHaveTextContent(
      'env: prod, staging'
    )
    expect(screen.getByTestId('metadata-chip-team')).toHaveTextContent(
      'team: core'
    )
  })

  it('renders a key-exists filter as "any" rather than a blank chip', () => {
    // [] is the backend's "the key exists" form, which a page can restore from
    // the URL even though this component never emits it.
    render(<MetadataFilter {...baseProps({ value: { env: [] } })} />)

    expect(screen.getByTestId('metadata-chip-env')).toHaveTextContent(
      'env: any'
    )
  })

  it('removing a chip omits that key entirely', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <MetadataFilter
        {...baseProps({ onChange, value: { env: ['prod'], team: ['core'] } })}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Remove env filter' }))

    expect(onChange).toHaveBeenCalledWith({ team: ['core'] })
  })

  it('clear filters emits an empty object', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <MetadataFilter {...baseProps({ onChange, value: { env: ['prod'] } })} />
    )

    await user.click(
      screen.getByRole('button', { name: 'Clear metadata filters' })
    )

    expect(onChange).toHaveBeenCalledWith({})
  })

  it('has no clear button when nothing is filtered', () => {
    render(<MetadataFilter {...baseProps()} />)

    expect(
      screen.queryByRole('button', { name: 'Clear metadata filters' })
    ).not.toBeInTheDocument()
  })

  it('typing in the value search reports each keystroke to the host, which debounces', async () => {
    const user = userEvent.setup()
    const onValueQueryChange = vi.fn()
    const props = baseProps({
      activeKey: 'env',
      values: ['prod'],
      onValueQueryChange,
    })
    render(<MetadataFilter {...props} />)

    await openPopover(user)
    await user.type(await screen.findByLabelText('Search env values'), 'pr')

    // The component is controlled and does not debounce itself — that lives in
    // useMetadataCatalog, which has its own debounce test.
    expect(onValueQueryChange).toHaveBeenCalled()
  })

  it('surfaces the truncation affordance only when truncated', async () => {
    const user = userEvent.setup()
    const { unmount } = render(
      <MetadataFilter
        {...baseProps({
          activeKey: 'env',
          values: ['prod'],
          valuesTruncated: true,
        })}
      />
    )

    await openPopover(user)
    expect(await screen.findByText(/More values available/)).toBeInTheDocument()
    unmount()

    render(
      <MetadataFilter
        {...baseProps({
          activeKey: 'env',
          values: ['prod'],
          valuesTruncated: false,
        })}
      />
    )
    await openPopover(userEvent.setup())
    expect(screen.queryByText(/More values available/)).not.toBeInTheDocument()
  })

  it('shows the loading and error states for keys', async () => {
    const user = userEvent.setup()
    const { unmount } = render(
      <MetadataFilter {...baseProps({ keysLoading: true })} />
    )
    await openPopover(user)
    expect(await screen.findByTestId('loader2-icon')).toBeInTheDocument()
    unmount()

    render(
      <MetadataFilter
        {...baseProps({ keysError: 'Failed to load metadata keys' })}
      />
    )
    await openPopover(userEvent.setup())
    expect(
      await screen.findByText('Failed to load metadata keys')
    ).toBeInTheDocument()
  })

  it('shows the error state for values', async () => {
    const user = userEvent.setup()
    render(
      <MetadataFilter
        {...baseProps({
          activeKey: 'env',
          valuesError: 'Failed to load metadata values',
        })}
      />
    )

    await openPopover(user)

    expect(
      await screen.findByText('Failed to load metadata values')
    ).toBeInTheDocument()
  })

  it('going back returns to the key list', async () => {
    const user = userEvent.setup()
    const props = baseProps({ activeKey: 'env', values: ['prod'] })
    render(<MetadataFilter {...props} />)

    await openPopover(user)
    await user.click(
      await screen.findByRole('button', { name: 'Back to keys' })
    )

    expect(props.onSelectKey).toHaveBeenCalledWith(null)
  })

  it('every interactive control has an accessible name', async () => {
    const user = userEvent.setup()
    render(
      <MetadataFilter
        {...baseProps({
          value: { env: ['prod'] },
          activeKey: 'env',
          values: ['prod'],
        })}
      />
    )

    expect(
      screen.getByRole('combobox', { name: 'Filter by metadata' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Remove env filter' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Clear metadata filters' })
    ).toBeInTheDocument()

    await openPopover(user)
    expect(
      await screen.findByLabelText('Search env values')
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Apply env filter' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Back to keys' })
    ).toBeInTheDocument()
  })

  it('respects disabled', () => {
    render(<MetadataFilter {...baseProps({ disabled: true })} />)

    expect(
      screen.getByRole('combobox', { name: 'Filter by metadata' })
    ).toBeDisabled()
  })
})
