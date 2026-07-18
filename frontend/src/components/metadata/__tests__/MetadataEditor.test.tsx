import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { MetadataEditor } from '../MetadataEditor'

const lastEmitted = (fn: jest.Mock): Record<string, unknown> => {
  const calls = fn.mock.calls
  return calls[calls.length - 1][0] as Record<string, unknown>
}

describe('MetadataEditor', () => {
  it('renders existing string pairs pre-filled and hides non-string extras', () => {
    render(
      <MetadataEditor
        value={{ author: 'ada', tags: ['x', 'y'], count: 3 }}
        onChange={jest.fn()}
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
    render(<MetadataEditor value={{}} onChange={jest.fn()} />)
    expect(screen.queryAllByTestId('metadata-row')).toHaveLength(0)
    await user.click(screen.getByTestId('metadata-add-pair'))
    expect(screen.getAllByTestId('metadata-row')).toHaveLength(1)
  })

  it('emits the recombined map with extras preserved when editing a value', async () => {
    const user = userEvent.setup()
    const onChange = jest.fn()
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
    const onChange = jest.fn()
    render(<MetadataEditor value={{ a: '1', b: '2' }} onChange={onChange} />)
    await user.click(screen.getByTestId('metadata-delete-0'))
    expect(screen.getAllByTestId('metadata-row')).toHaveLength(1)
    expect(lastEmitted(onChange)).toEqual({ b: '2' })
  })

  it('shows an inline error and reports invalidity for a blank value', async () => {
    const user = userEvent.setup()
    const onValidityChange = jest.fn()
    render(
      <MetadataEditor
        value={{ a: '1' }}
        onChange={jest.fn()}
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
    render(<MetadataEditor value={{ dup: '1' }} onChange={jest.fn()} />)
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
      <MetadataEditor value={{}} onChange={jest.fn()} reservedKeys={['tags']} />
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
        onChange={jest.fn()}
        requiredKeys={['model']}
      />
    )
    expect(screen.getByTestId('metadata-key-0')).toHaveAttribute('readonly')
    expect(screen.getByTestId('metadata-delete-0')).toBeDisabled()
  })

  it('reports validity true for well-formed metadata on mount', async () => {
    const onValidityChange = jest.fn()
    render(
      <MetadataEditor
        value={{ a: '1' }}
        onChange={jest.fn()}
        onValidityChange={onValidityChange}
      />
    )
    await waitFor(() => {
      expect(onValidityChange).toHaveBeenLastCalledWith(true)
    })
  })

  it('re-seeds rows when the host drives a new value (form reset)', () => {
    const { rerender } = render(
      <MetadataEditor value={{ a: '1' }} onChange={jest.fn()} />
    )
    expect(screen.getByDisplayValue('a')).toBeInTheDocument()
    rerender(<MetadataEditor value={{ b: '2' }} onChange={jest.fn()} />)
    expect(screen.getByDisplayValue('b')).toBeInTheDocument()
    expect(screen.queryByDisplayValue('a')).not.toBeInTheDocument()
  })
})
