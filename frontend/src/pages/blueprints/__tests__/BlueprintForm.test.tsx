import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'

import type { Blueprint } from '@/services/blueprintService'
import type { Project } from '@/services/projectService'

import { BlueprintForm, type BlueprintFormHandle } from '../BlueprintForm'

// Radix Select relies on layout APIs jsdom doesn't implement.
beforeAll(() => {
  Element.prototype.scrollIntoView = jest.fn()
  Element.prototype.hasPointerCapture = jest.fn()
  Element.prototype.releasePointerCapture = jest.fn()
})

const projects: Project[] = [{ id: 'p1', name: 'Project One' } as Project]

const baseBlueprint: Blueprint = {
  id: 'bp-1',
  project_id: 'p1',
  slug: 'my-blueprint',
  user_id: 'user-1',
  content: 'Some blueprint content',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
  status: 'active',
  title: 'My Blueprint',
  description: 'desc',
  type: 'general',
  metadata: {},
}

function renderForm(overrides?: {
  blueprint?: Blueprint
  onSubmit?: jest.Mock
}) {
  const onSubmit = overrides?.onSubmit ?? jest.fn().mockResolvedValue(undefined)
  const ref = createRef<BlueprintFormHandle>()
  render(
    <BlueprintForm
      ref={ref}
      blueprint={overrides?.blueprint}
      projects={projects}
      onSubmit={onSubmit}
    />
  )
  return { onSubmit, ref }
}

describe('BlueprintForm — metadata editing', () => {
  it('renders the metadata editor', () => {
    renderForm()
    expect(screen.getByTestId('metadata-editor')).toBeInTheDocument()
  })

  it('pre-fills existing string metadata from the blueprint', () => {
    renderForm({
      blueprint: { ...baseBlueprint, metadata: { author: 'ada' } },
    })
    expect(screen.getByDisplayValue('author')).toBeInTheDocument()
    expect(screen.getByDisplayValue('ada')).toBeInTheDocument()
  })

  it('includes edited metadata in the update payload, preserving non-string values', async () => {
    const user = userEvent.setup()
    const { onSubmit, ref } = renderForm({
      blueprint: {
        ...baseBlueprint,
        metadata: { author: 'ada', tags: ['x'] },
      },
    })

    await user.click(screen.getByTestId('metadata-add-pair'))
    await user.type(screen.getByTestId('metadata-key-1'), 'env')
    await user.type(screen.getByTestId('metadata-value-1'), 'prod')

    await act(async () => {
      ref.current?.submit()
      await Promise.resolve()
    })

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1)
    })
    expect(onSubmit.mock.calls[0][0]).toMatchObject({
      metadata: { author: 'ada', tags: ['x'], env: 'prod' },
    })
  })

  it('includes metadata in the create payload', async () => {
    const user = userEvent.setup()
    const { onSubmit, ref } = renderForm()

    await user.type(screen.getByLabelText('Title'), 'New BP')
    await user.type(screen.getByLabelText('Slug'), 'new-bp')
    await user.type(
      screen.getByRole('textbox', { name: /content/i }),
      'content body'
    )

    // Pick a project via the Radix Select.
    await user.click(screen.getByTestId('blueprint-project-select'))
    await user.click(screen.getByRole('option', { name: 'Project One' }))

    await user.click(screen.getByTestId('metadata-add-pair'))
    await user.type(screen.getByTestId('metadata-key-0'), 'author')
    await user.type(screen.getByTestId('metadata-value-0'), 'grace')

    await act(async () => {
      ref.current?.submit()
      await Promise.resolve()
    })

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1)
    })
    expect(onSubmit.mock.calls[0][0]).toMatchObject({
      slug: 'new-bp',
      metadata: { author: 'grace' },
    })
  })

  it('omits metadata from the payload when empty', async () => {
    const { onSubmit, ref } = renderForm({
      blueprint: { ...baseBlueprint, metadata: {} },
    })
    await act(async () => {
      ref.current?.submit()
      await Promise.resolve()
    })
    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1)
    })
    expect(onSubmit.mock.calls[0][0].metadata).toBeUndefined()
  })

  it('locks the required model key for sub-agents blueprints', () => {
    renderForm({
      blueprint: {
        ...baseBlueprint,
        subtype: 'sub-agents',
        metadata: { model: 'opus' },
      },
    })
    expect(screen.getByTestId('metadata-key-0')).toHaveAttribute('readonly')
    expect(screen.getByTestId('metadata-delete-0')).toBeDisabled()
  })

  it('blocks submit when metadata is invalid', async () => {
    const user = userEvent.setup()
    const { onSubmit, ref } = renderForm({
      blueprint: { ...baseBlueprint, metadata: { author: 'ada' } },
    })

    // Blank the value → editor reports invalid.
    await user.clear(screen.getByTestId('metadata-value-0'))
    await waitFor(() => {
      expect(screen.getByTestId('metadata-error-0')).toBeInTheDocument()
    })

    await act(async () => {
      ref.current?.submit()
      await Promise.resolve()
    })

    // Give any pending microtasks a chance, then assert no submit happened.
    await Promise.resolve()
    expect(onSubmit).not.toHaveBeenCalled()
  })
})
