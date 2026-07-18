import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'

import type { Memory } from '@/services/memoryService'
import type { Project } from '@/services/projectService'

import { MemoryForm, type MemoryFormHandle } from '../MemoryForm'

// Radix Select (project/status) uses layout APIs jsdom lacks.
beforeAll(() => {
  Element.prototype.scrollIntoView = jest.fn()
  Element.prototype.hasPointerCapture = jest.fn()
  Element.prototype.releasePointerCapture = jest.fn()
})

const projects: Project[] = [{ id: 'p1', name: 'Project One' } as Project]

const baseMemory: Memory = {
  id: 'm-1',
  project_id: 'p1',
  user_id: 'user-1',
  text: 'A memory',
  status: 'active',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
  metadata: {},
} as Memory

function renderForm(overrides?: { memory?: Memory; onSubmit?: jest.Mock }) {
  const onSubmit = overrides?.onSubmit ?? jest.fn().mockResolvedValue(undefined)
  const ref = createRef<MemoryFormHandle>()
  render(
    <MemoryForm
      ref={ref}
      memory={overrides?.memory}
      projects={projects}
      onSubmit={onSubmit}
    />
  )
  return { onSubmit, ref }
}

describe('MemoryForm — metadata editing alongside tags', () => {
  it('renders both the tags chip UI and the metadata editor', () => {
    renderForm()
    expect(
      screen.getByPlaceholderText('Add tags (comma-separated)')
    ).toBeInTheDocument()
    expect(screen.getByTestId('metadata-editor')).toBeInTheDocument()
  })

  it('shows non-tags string metadata as editor rows and never tags as a row', () => {
    renderForm({
      memory: {
        ...baseMemory,
        metadata: { tags: ['alpha'], author: 'ada' },
      },
    })
    // Non-tags key is an editable row.
    expect(screen.getByDisplayValue('author')).toBeInTheDocument()
    expect(screen.getByDisplayValue('ada')).toBeInTheDocument()
    // `tags` stays in the chip UI, never a string row.
    expect(screen.getByText('alpha')).toBeInTheDocument()
    expect(screen.queryByDisplayValue('tags')).not.toBeInTheDocument()
  })

  it('recombines chip tags + edited extras into the submit payload, preserving complex values', async () => {
    const user = userEvent.setup()
    const { onSubmit, ref } = renderForm({
      memory: {
        ...baseMemory,
        metadata: { tags: ['alpha'], author: 'ada', config: { x: 1 } },
      },
    })

    await user.click(screen.getByTestId('metadata-add-pair'))
    await user.type(screen.getByTestId('metadata-key-1'), 'env')
    await user.type(screen.getByTestId('metadata-value-1'), 'prod')

    await act(async () => {
      ref.current?.submit()
      await new Promise(resolve => setTimeout(resolve, 0))
    })

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1)
    })
    expect(onSubmit.mock.calls[0][0].metadata).toEqual({
      author: 'ada',
      config: { x: 1 },
      env: 'prod',
      tags: ['alpha'],
    })
  })

  it('rejects a string row named tags (reserved) and blocks submit', async () => {
    const user = userEvent.setup()
    const { onSubmit, ref } = renderForm()

    await user.click(screen.getByTestId('metadata-add-pair'))
    await user.type(screen.getByTestId('metadata-key-0'), 'tags')
    await user.type(screen.getByTestId('metadata-value-0'), 'oops')

    expect(screen.getByTestId('metadata-error-0')).toHaveTextContent('reserved')

    await act(async () => {
      ref.current?.submit()
      await new Promise(resolve => setTimeout(resolve, 0))
    })
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('keeps the tags chip add/remove flow working', async () => {
    const user = userEvent.setup()
    renderForm()
    const input = screen.getByPlaceholderText('Add tags (comma-separated)')

    await user.type(input, 'beta{enter}')
    expect(screen.getByText('beta')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Remove beta' }))
    expect(screen.queryByText('beta')).not.toBeInTheDocument()
  })

  it('omits metadata from the payload when there are no tags and no extras', async () => {
    const { onSubmit, ref } = renderForm({
      memory: { ...baseMemory, metadata: {} },
    })
    await act(async () => {
      ref.current?.submit()
      await new Promise(resolve => setTimeout(resolve, 0))
    })
    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1)
    })
    expect(onSubmit.mock.calls[0][0].metadata).toBeUndefined()
  })
})
