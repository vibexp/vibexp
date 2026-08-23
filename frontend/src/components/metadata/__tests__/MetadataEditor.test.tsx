import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Mock } from 'vitest'

import { MetadataEditor } from '../MetadataEditor'

const lastEmitted = (fn: Mock): Record<string, unknown> => {
  const calls = fn.mock.calls
  return calls[calls.length - 1][0] as Record<string, unknown>
}

describe('MetadataEditor', () => {
  it('renders existing string pairs pre-filled and hides non-string extras', () => {
    render(
      <MetadataEditor
        value={{ author: 'ada', tags: ['x', 'y'], count: 3 }}
        onChange={vi.fn()}
      />
    )
    expect(screen.getByDisplayValue('author')).toBeInTheDocument()
    expect(screen.getByDisplayValue('ada')).toBeInTheDocument()
    // Non-string extras never surface as rows.
    expect(screen.queryByDisplayValue('tags')).not.toBeInTheDocument()
    expect(screen.getAllByTestId('metadata-row')).toHaveLength(1)
  })

  it('adds a new empty pair', async () => {
    const user = userEvent.setup()
    render(<MetadataEditor value={{}} onChange={vi.fn()} />)
    expect(screen.queryAllByTestId('metadata-row')).toHaveLength(0)
    await user.click(screen.getByTestId('metadata-add-pair'))
    expect(screen.getAllByTestId('metadata-row')).toHaveLength(1)
  })

  it('emits the recombined map with extras preserved when editing a value', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <MetadataEditor
        value={{ author: 'ada', tags: ['x'] }}
        onChange={onChange}
      />
    )
    await user.clear(screen.getByTestId('metadata-value-0'))
    await user.type(screen.getByTestId('metadata-value-0'), 'grace')
    const emitted = lastEmitted(onChange)
    expect(emitted).toEqual({ author: 'grace', tags: ['x'] })
  })

  it('deletes a pair', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<MetadataEditor value={{ a: '1', b: '2' }} onChange={onChange} />)
    await user.click(screen.getByTestId('metadata-delete-0'))
    expect(screen.getAllByTestId('metadata-row')).toHaveLength(1)
    expect(lastEmitted(onChange)).toEqual({ b: '2' })
  })

  it('shows an inline error and reports invalidity for a blank value', async () => {
    const user = userEvent.setup()
    const onValidityChange = vi.fn()
    render(
      <MetadataEditor
        value={{ a: '1' }}
        onChange={vi.fn()}
        onValidityChange={onValidityChange}
      />
    )
    await user.clear(screen.getByTestId('metadata-value-0'))
    expect(screen.getByTestId('metadata-error-0')).toHaveTextContent(
      'Value is required'
    )
    await waitFor(() => {
      expect(onValidityChange).toHaveBeenLastCalledWith(false)
    })
  })

  it('flags a duplicate key', async () => {
    const user = userEvent.setup()
    render(<MetadataEditor value={{ dup: '1' }} onChange={vi.fn()} />)
    await user.click(screen.getByTestId('metadata-add-pair'))
    await user.type(screen.getByTestId('metadata-key-1'), 'dup')
    await user.type(screen.getByTestId('metadata-value-1'), '2')
    expect(screen.getByTestId('metadata-error-1')).toHaveTextContent(
      'Duplicate key'
    )
  })

  it('flags a reserved key', async () => {
    const user = userEvent.setup()
    render(
      <MetadataEditor value={{}} onChange={vi.fn()} reservedKeys={['tags']} />
    )
    await user.click(screen.getByTestId('metadata-add-pair'))
    await user.type(screen.getByTestId('metadata-key-0'), 'tags')
    await user.type(screen.getByTestId('metadata-value-0'), 'v')
    expect(screen.getByTestId('metadata-error-0')).toHaveTextContent('reserved')
  })

  it('makes a required-key row read-only and undeletable', () => {
    render(
      <MetadataEditor
        value={{ model: 'opus' }}
        onChange={vi.fn()}
        requiredKeys={['model']}
      />
    )
    expect(screen.getByTestId('metadata-key-0')).toHaveAttribute('readonly')
    expect(screen.getByTestId('metadata-delete-0')).toBeDisabled()
  })

  it('reports validity true for well-formed metadata on mount', async () => {
    const onValidityChange = vi.fn()
    render(
      <MetadataEditor
        value={{ a: '1' }}
        onChange={vi.fn()}
        onValidityChange={onValidityChange}
      />
    )
    await waitFor(() => {
      expect(onValidityChange).toHaveBeenLastCalledWith(true)
    })
  })

  it('re-seeds rows when the host drives a new value (form reset)', () => {
    const { rerender } = render(
      <MetadataEditor value={{ a: '1' }} onChange={vi.fn()} />
    )
    expect(screen.getByDisplayValue('a')).toBeInTheDocument()
    rerender(<MetadataEditor value={{ b: '2' }} onChange={vi.fn()} />)
    expect(screen.getByDisplayValue('b')).toBeInTheDocument()
    expect(screen.queryByDisplayValue('a')).not.toBeInTheDocument()
  })

  // Rows carry two generations of id: the positional ones the initial state
  // hands out, and the counter's. This is the sequence where both are live at
  // once — a counter seeded anywhere at or below the last positional index
  // re-issues an id belonging to an existing row, which gives React duplicate
  // keys and cross-wires the two rows' inputs.
  it('keeps row ids distinct when a pair is added to initially seeded rows', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <MetadataEditor value={{ a: '1', b: '2', c: '3' }} onChange={onChange} />
    )

    await user.click(screen.getByTestId('metadata-add-pair'))
    expect(screen.getAllByTestId('metadata-row')).toHaveLength(4)

    // Typing into the appended row must not leak into any seeded row.
    await user.type(screen.getByTestId('metadata-key-3'), 'd')
    await user.type(screen.getByTestId('metadata-value-3'), '4')

    expect(screen.getByTestId('metadata-key-0')).toHaveValue('a')
    expect(screen.getByTestId('metadata-key-1')).toHaveValue('b')
    expect(screen.getByTestId('metadata-key-2')).toHaveValue('c')
    expect(lastEmitted(onChange)).toEqual({
      a: '1',
      b: '2',
      c: '3',
      d: '4',
    })
  })

  it('preserves extras that arrive with an externally driven value', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const { rerender } = render(
      <MetadataEditor value={{ a: '1', tags: ['x'] }} onChange={onChange} />
    )
    // A different extras payload arrives with the new value; editing afterwards
    // has to recombine against the NEW extras, not the mount-time ones.
    rerender(
      <MetadataEditor value={{ b: '2', tags: ['y'] }} onChange={onChange} />
    )

    await user.clear(screen.getByTestId('metadata-value-0'))
    await user.type(screen.getByTestId('metadata-value-0'), '9')

    expect(lastEmitted(onChange)).toEqual({ b: '9', tags: ['y'] })
  })
})
